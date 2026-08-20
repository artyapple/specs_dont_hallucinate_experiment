package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// providerCredentialVariables are removed from the environment passed to the
// evaluator process, so provider keys never enter the evaluation environment.
var providerCredentialVariables = []string{"OPENROUTER_API_KEY"}

// withoutProviderCredentials returns env with provider credentials stripped.
func withoutProviderCredentials(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		redacted := false
		for _, key := range providerCredentialVariables {
			if strings.HasPrefix(entry, key+"=") {
				redacted = true
				break
			}
		}
		if !redacted {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// evaluateCandidate runs the hidden evaluator against the preserved workspace
// and classifies the outcome. A nil failure means the evaluator produced a
// trustworthy standalone result and the returned Evaluation embeds it. A
// non-nil failure flips the run to harness-failure or infrastructure-failure
// and the raw evaluator output is still preserved as evaluation.json.
func evaluateCandidate(opts options, evaluatorBin, root, workspaceDir string, cell Cell, manifestIDs map[string]bool) (*Evaluation, []byte, *failureNote) {
	// The 16 minute assembly deadline strictly exceeds the frozen 15 minute
	// evaluation budget, so a healthy evaluator always finishes first.
	ctx, cancel := context.WithTimeout(context.Background(), 16*time.Minute)
	defer cancel()
	arguments := []string{"-task", cell.Task, "-candidate", workspaceDir, "-output", "-"}
	var command *exec.Cmd
	if strings.HasSuffix(evaluatorBin, ".sh") {
		command = exec.CommandContext(ctx, "bash", append([]string{evaluatorBin}, arguments...)...)
	} else {
		command = exec.CommandContext(ctx, evaluatorBin, arguments...)
	}
	command.Env = withoutProviderCredentials(os.Environ())
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	cleanupLabeledEvaluator()
	raw := stdout.Bytes()
	if len(raw) == 0 {
		raw = []byte("null\n")
	}
	if ctx.Err() == context.DeadlineExceeded {
		return nil, raw, &failureNote{status: statusHarnessFail, category: infraCategoryHarnessCrash, evidence: "evaluator exceeded the 16 minute assembly deadline"}
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return nil, raw, &failureNote{status: statusHarnessFail, category: infraCategoryHarnessCrash, evidence: fmt.Sprintf("start evaluator: %v", runErr)}
		}
		if exitErr.ExitCode() == 2 {
			return nil, raw, &failureNote{status: statusHarnessFail, category: infraCategoryHarnessCrash, evidence: fmt.Sprintf("evaluator rejected the invocation (exit 2): %s", trimEvidence(stderr.String()))}
		}
		if exitErr.ExitCode() != 0 && exitErr.ExitCode() != 1 {
			return nil, raw, &failureNote{status: statusHarnessFail, category: infraCategoryHarnessCrash, evidence: fmt.Sprintf("evaluator exited with unexpected code %d: %s", exitErr.ExitCode(), trimEvidence(stderr.String()))}
		}
	}
	var standalone evaluatorResult
	if err := json.Unmarshal(stdout.Bytes(), &standalone); err != nil || len(standalone.BehaviorCases) == 0 {
		return nil, raw, &failureNote{status: statusHarnessFail, category: infraCategoryHarnessCrash, evidence: fmt.Sprintf("evaluator produced no parseable result JSON: %s", trimEvidence(stderr.String()))}
	}
	// A PostgreSQL setup failure is an evaluator infrastructure failure: the
	// candidate never had a chance to run. Behavior outcomes are not fabricated.
	if !standalone.Setup.Postgres {
		evidence := standalone.Setup.Evidence
		if evidence == "" {
			evidence = "evaluator could not start the pinned PostgreSQL container"
		}
		return nil, raw, &failureNote{status: statusInfraFail, category: infraCategoryEvaluatorFailure, evidence: evidence}
	}

	if err := checkRoster(standalone.BehaviorCases, manifestIDs); err != nil {
		// Registry and evaluator disagree: this is a tooling defect, not run evidence.
		return nil, raw, &failureNote{status: statusHarnessFail, category: infraCategoryHarnessCrash, evidence: "evaluator roster mismatch: " + err.Error()}
	}

	formalOK, formalEvidence := checkFormalInputs(workspaceDir, cell, filepath.Join(root, "tasks", "propagation"))
	cases := make([]BehaviorCase, 0, len(standalone.BehaviorCases))
	for _, c := range standalone.BehaviorCases {
		out := BehaviorCase{ID: c.ID, Applicable: c.Applicable, Passed: c.Passed}
		if c.Applicable {
			evidence := c.Evidence
			out.Evidence = &evidence
		}
		cases = append(cases, out)
	}
	evaluation := &Evaluation{
		CommonGates:      deriveGates(standalone, formalOK, cell.Task),
		BehaviorCases:    cases,
		CandidateTests:   countCandidateTests(workspaceDir),
		ResidualFailures: []string{},
	}
	if !formalOK && formalEvidence != "" {
		evaluation.ResidualFailures = append(evaluation.ResidualFailures, "gate:formal-inputs: "+formalEvidence)
	}
	complete := true
	gates := evaluation.CommonGates
	if !gates.FormalInputs || !gates.Build || !gates.Migrations || !gates.ServiceStart ||
		!gates.BaselineBehavior || !gates.TaskBehavior || !gates.Regressions ||
		!gates.APIConformance || !gates.DatabaseConsistency {
		complete = false
	}
	for _, c := range standalone.BehaviorCases {
		if !c.Applicable {
			continue
		}
		if c.Passed == nil || !*c.Passed {
			complete = false
			evaluation.ResidualFailures = append(evaluation.ResidualFailures, c.ID)
		}
	}
	sort.Strings(evaluation.ResidualFailures)
	evaluation.CompleteSuccess = complete

	if cell.Treatment == "codegen" {
		health, err := measureCodegenHealth(workspaceDir, opts.codegenImage)
		if err != nil {
			return nil, raw, &failureNote{status: statusInfraFail, category: infraCategoryRuntimeFailure, evidence: "codegen health check environment failed: " + err.Error()}
		}
		evaluation.CodegenHealth = health
	}
	return evaluation, raw, nil
}

// cleanupLabeledEvaluator handles an evaluator wrapper whose Docker client was
// killed by the assembly deadline. A graceful stop lets the evaluator clean up
// its Testcontainers children before the labelled container is removed.
func cleanupLabeledEvaluator() {
	runID, instanceID := os.Getenv("EXPERIMENT_RUN_ID"), os.Getenv("EXPERIMENT_INSTANCE_ID")
	if runID == "" || instanceID == "" {
		return
	}
	filter := []string{"ps", "-aq", "--filter", "label=experiment.run-id=" + runID, "--filter", "label=experiment.instance-id=" + instanceID}
	output, err := exec.Command("docker", filter...).Output()
	if err != nil {
		return
	}
	for _, id := range strings.Fields(string(output)) {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		_ = exec.CommandContext(ctx, "docker", "stop", "--time", "20", id).Run()
		cancel()
		_ = exec.Command("docker", "rm", "-f", id).Run()
	}
}

// checkRoster proves that the evaluator emitted every frozen manifest case
// exactly once, with no duplicates, unknown IDs, or missing IDs.
func checkRoster(cases []evaluatorCase, manifestIDs map[string]bool) error {
	seen := make(map[string]bool, len(cases))
	for _, c := range cases {
		if !manifestIDs[c.ID] {
			return fmt.Errorf("unknown case id %q", c.ID)
		}
		if seen[c.ID] {
			return fmt.Errorf("duplicate case id %q", c.ID)
		}
		seen[c.ID] = true
	}
	for id := range manifestIDs {
		if !seen[id] {
			return fmt.Errorf("missing case id %q", id)
		}
	}
	return nil
}

func trimEvidence(value string) string {
	const limit = 2048
	value = string(bytes.TrimSpace([]byte(value)))
	if len(value) > limit {
		value = value[:limit] + "…"
	}
	if value == "" {
		return "no diagnostics captured"
	}
	return value
}
