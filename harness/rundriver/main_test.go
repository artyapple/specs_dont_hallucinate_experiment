package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCopyTreeIncludesIgnoredNamesAndPreservesExecutable(t *testing.T) {
	source := t.TempDir()
	generated := filepath.Join(source, "internal", "generated")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]struct {
		content string
		mode    os.FileMode
	}{
		".gitignore":                        {"generated/\n", 0o644},
		"internal/generated/ignored-output": {"preserved\n", 0o644},
		"run.sh":                            {"#!/bin/sh\n", 0o755},
	}
	for name, fixture := range files {
		path := filepath.Join(source, name)
		if err := os.WriteFile(path, []byte(fixture.content), fixture.mode); err != nil {
			t.Fatal(err)
		}
	}
	destination := filepath.Join(t.TempDir(), "workspace")
	if err := copyTree(source, destination, false); err != nil {
		t.Fatal(err)
	}
	for name, fixture := range files {
		path := filepath.Join(destination, name)
		data, err := os.ReadFile(path)
		if err != nil || string(data) != fixture.content {
			t.Fatalf("%s = %q, %v", name, data, err)
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm() != fixture.mode {
			t.Fatalf("%s mode = %o, want %o", name, info.Mode().Perm(), fixture.mode)
		}
	}
}

func TestCopyTreeRejectsSymlinkAndHiddenDirectory(t *testing.T) {
	for name, setup := range map[string]func(string) error{
		"symlink":          func(root string) error { return os.Symlink("target", filepath.Join(root, "link")) },
		"hidden directory": func(root string) error { return os.Mkdir(filepath.Join(root, ".git"), 0o755) },
	} {
		t.Run(name, func(t *testing.T) {
			source := t.TempDir()
			if err := setup(source); err != nil {
				t.Fatal(err)
			}
			if err := copyTree(source, filepath.Join(t.TempDir(), "workspace"), false); err == nil {
				t.Fatal("unsafe tree must be rejected")
			}
		})
	}
}

func TestValidateTimedOutDuration(t *testing.T) {
	start := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	opts := options{
		runDir: "run", runID: "id", cellID: "greenfield-direct", repeatIndex: 1,
		phase: "pilot", status: "timed-out", workspaceSource: "workspace",
		transcriptSource: "transcript", patchSource: "patch",
		startedAt: start.Format(time.RFC3339), finishedAt: start.Add(timeout).Format(time.RFC3339),
	}
	if err := validateOptions(&opts); err != nil {
		t.Fatal(err)
	}
	opts.finishedAt = start.Add(timeout - time.Second).Format(time.RFC3339)
	if err := validateOptions(&opts); err == nil {
		t.Fatal("short timed-out duration must fail")
	}
}

func TestWithoutProviderCredentials(t *testing.T) {
	got := withoutProviderCredentials([]string{"A=1", "OPENROUTER_API_KEY=secret", "B=2"})
	if len(got) != 2 || got[0] != "A=1" || got[1] != "B=2" {
		t.Fatalf("filtered environment = %v", got)
	}
}

func TestWithEnvironmentReplacesExactValues(t *testing.T) {
	got := withEnvironment([]string{"OPENCODE_MODEL=wrong", "A=1", "OPENCODE_CONFIG_CONTENT=wrong"}, "OPENCODE_MODEL=right", "OPENCODE_CONFIG_CONTENT={}")
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "wrong") || !strings.Contains(joined, "OPENCODE_MODEL=right") || !strings.Contains(joined, "A=1") {
		t.Fatalf("environment = %v", got)
	}
}

func TestResolveProductionAllCells(t *testing.T) {
	root := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(root, "config", "experiment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config productionConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	runs := make([]map[string]any, 0, len(config.Cells))
	for index, cell := range config.Cells {
		runs = append(runs, map[string]any{"ordinal": index + 1, "runId": "run-" + cell.ID, "cellId": cell.ID, "repeatIndex": 1})
	}
	schedule := filepath.Join(t.TempDir(), "schedule.json")
	writeTestJSON(t, schedule, map[string]any{"runs": runs})
	if len(config.Cells) != 14 {
		t.Fatalf("config has %d cells, want 14", len(config.Cells))
	}
	for _, cell := range config.Cells {
		resolved, err := resolveProduction(root, schedule, "run-"+cell.ID)
		if err != nil {
			t.Fatalf("%s: %v", cell.ID, err)
		}
		if cell.Stage == "greenfield" && resolved.Fixture != "fixtures/base1" {
			t.Errorf("%s fixture = %s", cell.ID, resolved.Fixture)
		}
		if cell.Stage == "existing-service" && resolved.Fixture != "fixtures/base2-"+cell.Treatment {
			t.Errorf("%s fixture = %s", cell.ID, resolved.Fixture)
		}
		if cell.ID == "greenfield-codegen" && resolved.WorkspaceOverlay != "treatments/codegen/workspace" {
			t.Errorf("greenfield-codegen overlay = %q", resolved.WorkspaceOverlay)
		}
		if !strings.HasPrefix(resolved.Treatment, "treatments/"+cell.Treatment+"/") || resolved.Model != config.Model.ID {
			t.Errorf("%s resolved incorrectly: %+v", cell.ID, resolved)
		}
		expectedTask := "tasks/part1.md"
		if cell.Stage == "existing-service" {
			expectedTask = "tasks/propagation/" + cell.Task + ".md"
			if cell.Mode == "full-workflow" {
				expectedTask = "tasks/full/" + cell.Task + ".md"
			}
		}
		if resolved.Task != expectedTask {
			t.Errorf("%s task = %s, want %s", cell.ID, resolved.Task, expectedTask)
		}
	}
}

func TestApplyPropagationAndVerifyHashes(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, task := range []string{"nullable-patch", "optimistic-locking", "cursor-pagination"} {
		for _, treatment := range []string{"direct", "codegen"} {
			t.Run(task+"-"+treatment, func(t *testing.T) {
				workspace := filepath.Join(t.TempDir(), "workspace")
				if err := copyTree(filepath.Join(root, "fixtures", "base2-"+treatment), workspace, false); err != nil {
					t.Fatal(err)
				}
				if err := applyPropagation(root, workspace, task); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestGreenfieldCodegenWorkspaceOverlay(t *testing.T) {
	root := filepath.Join("..", "..")
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := copyTree(filepath.Join(root, "fixtures", "base1"), workspace, false); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(filepath.Join(root, "treatments", "codegen", "workspace"), workspace, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"oapi-codegen.yaml", "sqlc.yaml", "scripts/generate.sh", "scripts/verify-generate.sh"} {
		if _, err := os.Stat(filepath.Join(workspace, path)); err != nil {
			t.Errorf("overlay lacks %s: %v", path, err)
		}
	}
	makefile, err := os.ReadFile(filepath.Join(workspace, "Makefile"))
	if err != nil || !strings.Contains(string(makefile), "generate:") || !strings.Contains(string(makefile), "verify-generate:") {
		t.Fatalf("overlay Makefile lacks generation targets: %v", err)
	}
}

func TestSafeRelativePath(t *testing.T) {
	for _, path := range []string{"../escape", "/absolute", "a/../../escape", `a\b`} {
		if safeRelativePath(path) {
			t.Errorf("accepted unsafe path %q", path)
		}
	}
	if !safeRelativePath("db/migrations/000002_safe.sql") {
		t.Fatal("rejected safe path")
	}
}

func TestGeneratePatchIncludesIgnoredBinaryAndModesWithoutChangingContent(t *testing.T) {
	runDir := t.TempDir()
	starting := filepath.Join(runDir, "starting-workspace")
	workspace := filepath.Join(runDir, "workspace")
	for _, directory := range []string{starting, workspace} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(starting, "run.sh"), []byte("starting-workspace/old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "run.sh"), []byte("starting-workspace/new\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "ignored.bin"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	patch := filepath.Join(runDir, "final.patch")
	if err := generatePatch(runDir, starting, workspace, patch); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(patch)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{"diff --git a/run.sh b/run.sh", "old mode 100644", "new mode 100755", "+starting-workspace/new", "ignored.bin"} {
		if !strings.Contains(text, expected) {
			t.Errorf("patch lacks %q:\n%s", expected, text)
		}
	}
}

func TestProductionCandidateOutcomesAndFinalization(t *testing.T) {
	for _, test := range []struct {
		name, exit, status string
		wantError          bool
	}{
		{"submitted", "0", "submitted", false},
		{"timeout", "124", "timed-out", false},
		{"coordinator", "70", "harness-failure", true},
		{"tool", "71", "infrastructure-failure", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, schedule, runner, assembler := productionTestRoot(t)
			t.Setenv("OPENROUTER_API_KEY", "candidate-only-secret")
			t.Setenv("CANDIDATE_TEST_EXIT", test.exit)
			runDir := filepath.Join(t.TempDir(), "run")
			opts := productionOptions{root: root, phase: "pilot", schedule: schedule, runDir: runDir, runID: "run-1", candidateRunner: runner, runresultBin: assembler, evaluatorWrapper: filepath.Join(root, "fake-evaluator.sh")}
			err := runProduction(context.Background(), opts)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v", err)
			}
			var metadata productionMetadata
			readTestJSON(t, filepath.Join(runDir, "metadata.json"), &metadata)
			if metadata.Status != test.status || metadata.Orchestration.CandidateExit != mustInt(test.exit) {
				t.Fatalf("metadata = %+v", metadata)
			}
			calls, _ := os.ReadFile(filepath.Join(root, "assembler.calls"))
			if string(calls) != "1\n" {
				t.Fatalf("assembler calls = %q", calls)
			}
			prompt, _ := os.ReadFile(filepath.Join(runDir, "prompt.md"))
			if string(prompt) != "TREATMENT\n\nTASK" {
				t.Fatalf("prompt = %q", prompt)
			}
			if _, err := os.Stat(filepath.Join(runDir, ".finalization-started")); err != nil {
				t.Fatal("finalization marker missing")
			}
			if test.status == "timed-out" && metadata.FinishedAt.Sub(metadata.StartedAt) != 45*time.Minute {
				t.Fatal("timeout metadata is not exactly 2700 seconds")
			}
			evaluationCalls, evaluationErr := os.ReadFile(filepath.Join(root, "evaluator.calls"))
			shouldEvaluate := test.status == "submitted" || test.status == "timed-out"
			if shouldEvaluate && (evaluationErr != nil || string(evaluationCalls) != "1\n") {
				t.Fatalf("evaluator calls = %q, %v", evaluationCalls, evaluationErr)
			}
			if !shouldEvaluate && !os.IsNotExist(evaluationErr) {
				t.Fatalf("hidden evaluator reached runner failure: %q, %v", evaluationCalls, evaluationErr)
			}
			if test.name == "submitted" {
				if err := runProduction(context.Background(), opts); err == nil {
					t.Fatal("preexisting finalized run was accepted")
				}
				calls, _ = os.ReadFile(filepath.Join(root, "assembler.calls"))
				if string(calls) != "1\n" {
					t.Fatalf("finalized run invoked assembler again: %q", calls)
				}
			}
		})
	}
}

func TestProductionCancellationStillFinalizesWithoutEvaluation(t *testing.T) {
	root, schedule, runner, assembler := productionTestRoot(t)
	if err := os.WriteFile(runner, []byte("#!/bin/sh\ntrap 'exit 130' INT TERM\nwhile :; do sleep 1; done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	runDir := filepath.Join(t.TempDir(), "run")
	err := runProduction(ctx, productionOptions{root: root, phase: "pilot", schedule: schedule, runDir: runDir, runID: "run-1", candidateRunner: runner, runresultBin: assembler, evaluatorWrapper: filepath.Join(root, "fake-evaluator.sh")})
	if err == nil {
		t.Fatal("cancelled production run returned success")
	}
	var metadata productionMetadata
	readTestJSON(t, filepath.Join(runDir, "metadata.json"), &metadata)
	if metadata.Status != "harness-failure" || metadata.Orchestration.CandidateSignal == "" {
		t.Fatalf("cancellation metadata = %+v", metadata)
	}
	if _, err := os.Stat(filepath.Join(runDir, ".finalization-started")); err != nil {
		t.Fatal("cancelled run was not finalized")
	}
	if _, err := os.Stat(filepath.Join(root, "evaluator.calls")); !os.IsNotExist(err) {
		t.Fatal("cancelled run reached hidden evaluation")
	}
}

func TestProductionDefaultRunnerCleanup(t *testing.T) {
	for _, test := range []struct {
		name, cleanupExit, status string
		wantError                 bool
	}{
		{name: "success", cleanupExit: "0", status: "submitted"},
		{name: "failure", cleanupExit: "1", status: "infrastructure-failure", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, schedule, runner, assembler := productionTestRoot(t)
			defaultRunner := filepath.Join(root, "harness", "run-candidate.sh")
			script := `#!/bin/sh
set -eu
if test "${1:-}" = --cleanup; then
  printf 'cleanup\n' >>"$(dirname "$0")/../cleanup.calls"
  exit "$CLEANUP_TEST_EXIT"
fi
printf '{}\n' >"$CANDIDATE_TRANSCRIPT"
exit 0
`
			if err := os.WriteFile(defaultRunner, []byte(script), 0o644); err != nil {
				t.Fatal(err)
			}
			_ = runner
			t.Setenv("CLEANUP_TEST_EXIT", test.cleanupExit)
			runDir := filepath.Join(t.TempDir(), "run")
			err := runProduction(context.Background(), productionOptions{
				root: root, phase: "pilot", schedule: schedule, runDir: runDir, runID: "run-1",
				runresultBin: assembler, evaluatorWrapper: filepath.Join(root, "fake-evaluator.sh"),
			})
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v", err)
			}
			calls, err := os.ReadFile(filepath.Join(root, "cleanup.calls"))
			if err != nil || string(calls) != "cleanup\n" {
				t.Fatalf("cleanup calls = %q, %v", calls, err)
			}
			var metadata productionMetadata
			readTestJSON(t, filepath.Join(runDir, "metadata.json"), &metadata)
			if metadata.Status != test.status {
				t.Fatalf("status = %s, want %s", metadata.Status, test.status)
			}
			if test.wantError {
				if _, err := os.Stat(filepath.Join(root, "evaluator.calls")); !os.IsNotExist(err) {
					t.Fatal("cleanup failure reached hidden evaluator")
				}
			}
		})
	}
}

func TestProductionShellContracts(t *testing.T) {
	candidate, err := os.ReadFile(filepath.Join("..", "run-candidate.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(candidate)
	for _, required := range []string{"--init", "--user 10001:10001", "OPENROUTER_API_KEY", "--add-host \"openrouter.ai:$EGRESS_PROXY_IP\"", "experiment.run-id", "experiment.instance-id", "CANDIDATE_TIMEOUT_SECONDS", "DATABASE_URL=postgres://", "--tmpfs /var/lib/postgresql:rw,nosuid,nodev", "--tmpfs /home/candidate/.cache/go-build:rw,exec,nosuid,nodev", "$MODULE_CACHE:/go/pkg/mod:ro", "pg_isready"} {
		if !strings.Contains(text, required) {
			t.Errorf("candidate runner lacks %q", required)
		}
	}
	toolBlock := text[strings.Index(text, "docker run -d"):strings.Index(text, "healthy=false")]
	if strings.Contains(toolBlock, "OPENROUTER_API_KEY") || strings.Contains(toolBlock, "openrouter.ai") {
		t.Fatal("tool command contains provider credential or host mapping")
	}
	evaluator, err := os.ReadFile(filepath.Join("..", "run-evaluator-container.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"uname -s", "Darwin) socket_gid=0", "Linux) socket_gid=\"$(stat -c '%g'"} {
		if !strings.Contains(string(evaluator), required) {
			t.Errorf("evaluator runner lacks %q", required)
		}
	}
}

func TestRunCandidateCancellation(t *testing.T) {
	script := filepath.Join(t.TempDir(), "candidate.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrap 'exit 130' INT TERM\nwhile :; do sleep 1; done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	command := scriptCommand(script)
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	exit, _, err := runCandidate(ctx, command)
	if err == nil || exit == 0 {
		t.Fatalf("cancelled candidate exit=%d err=%v", exit, err)
	}
}

func TestConcurrencySlotsAndUniqueInstances(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "locks")
	release1, err := acquireSlot(context.Background(), directory, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer release1()
	release2, err := acquireSlot(context.Background(), directory, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer release2()
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	if _, err := acquireSlot(ctx, directory, 2); err == nil {
		t.Fatal("third concurrent slot was admitted")
	}
	one, _ := randomInstanceID()
	two, _ := randomInstanceID()
	if one == "" || one == two {
		t.Fatalf("instance IDs are not unique: %q %q", one, two)
	}
}

func productionTestRoot(t *testing.T) (string, string, string, string) {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{"config", "fixtures/base1", "treatments/direct", "tasks", "harness"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	config := map[string]any{
		"model": map[string]any{"id": "openrouter/test-model"}, "agent": map[string]any{"version": "1.18.18"},
		"frozenInputs": map[string]any{"environmentImages": map[string]any{"coordinator": digest, "toolDirect": digest, "toolCodegen": digest, "evaluator": digest}},
		"execution":    map[string]any{"timeoutSeconds": 2700, "maxConcurrency": 2},
		"cells":        []map[string]any{{"id": "greenfield-direct", "stage": "greenfield", "task": "baseline-service", "mode": "greenfield", "treatment": "direct"}},
	}
	writeTestJSON(t, filepath.Join(root, "config", "experiment.json"), config)
	writeTestJSON(t, filepath.Join(root, "config", "versions.json"), map[string]any{
		"frozen": map[string]any{"postgresImage": "postgres:18.6@" + digest},
	})
	writeTestJSON(t, filepath.Join(root, "schedule.json"), map[string]any{"runs": []map[string]any{{"ordinal": 1, "runId": "run-1", "cellId": "greenfield-direct", "repeatIndex": 1}}})
	for path, content := range map[string]string{
		"fixtures/base1/input.txt": "initial\n", "treatments/direct/overlay.md": "TREATMENT", "tasks/part1.md": "TASK",
		"harness/opencode-run.json": `{"model":"{env:OPENCODE_MODEL}"}`, "fake-evaluator.sh": "#!/bin/sh\nexit 99\n",
	} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runner := filepath.Join(root, "candidate.sh")
	if err := os.WriteFile(runner, []byte("#!/bin/sh\nset -eu\ntest -n \"${OPENROUTER_API_KEY:-}\"\ntest \"$CANDIDATE_TIMEOUT_SECONDS\" = 2700\ntest \"$OPENCODE_MODEL\" = openrouter/test-model\ntest -n \"$OPENCODE_CONFIG_CONTENT\"\nprintf '{}\\n' >\"$CANDIDATE_TRANSCRIPT\"\nprintf changed >\"$CANDIDATE_WORKSPACE/output.txt\"\nexit \"$CANDIDATE_TEST_EXIT\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	assembler := filepath.Join(root, "runresult")
	if err := os.WriteFile(assembler, []byte("#!/bin/sh\nset -eu\ntest -z \"${OPENROUTER_API_KEY+x}\"\nprintf '1\\n' >>\"$EXPERIMENT_ROOT/assembler.calls\"\nif grep -Eq '\"status\": \"(submitted|timed-out)\"' \"$2/metadata.json\"; then printf '1\\n' >>\"$EXPERIMENT_ROOT/evaluator.calls\"; fi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, filepath.Join(root, "schedule.json"), runner, assembler
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
}

func mustInt(value string) int {
	result := 0
	for _, digit := range value {
		result = result*10 + int(digit-'0')
	}
	return result
}
