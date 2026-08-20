package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	productionTimeout     = 2700
	productionConcurrency = 2
)

type productionOptions struct {
	root, phase, schedule, runDir, runID string
	candidateRunner, runresultBin        string
	evaluatorWrapper                     string
}

type productionConfig struct {
	Model struct {
		ID string `json:"id"`
	} `json:"model"`
	Agent struct {
		Version string `json:"version"`
	} `json:"agent"`
	FrozenInputs struct {
		EnvironmentImages struct {
			Coordinator string `json:"coordinator"`
			ToolDirect  string `json:"toolDirect"`
			ToolCodegen string `json:"toolCodegen"`
			Evaluator   string `json:"evaluator"`
		} `json:"environmentImages"`
	} `json:"frozenInputs"`
	Execution struct {
		TimeoutSeconds int `json:"timeoutSeconds"`
		MaxConcurrency int `json:"maxConcurrency"`
	} `json:"execution"`
	Cells []productionCell `json:"cells"`
}

type productionVersions struct {
	Frozen struct {
		PostgresImage string `json:"postgresImage"`
	} `json:"frozen"`
}

type productionCell struct {
	ID, Stage, Task, Mode, Treatment string
}

type productionSchedule struct {
	Runs []struct {
		Ordinal, RepeatIndex int
		RunID, CellID        string
	} `json:"runs"`
}

type resolvedRun struct {
	Cell             productionCell
	RepeatIndex      int
	ScheduleOrdinal  int
	Fixture          string
	Treatment        string
	WorkspaceOverlay string
	Task             string
	Model            string
	AgentVersion     string
	CoordinatorImage string
	ToolImage        string
	CodegenImage     string
	EvaluatorImage   string
	PostgresImage    string
	TimeoutSeconds   int
	MaxConcurrency   int
	Blocker          string
}

type productionMetadata struct {
	RunID              string                  `json:"runId"`
	CellID             string                  `json:"cellId"`
	RepeatIndex        int                     `json:"repeatIndex"`
	Phase              string                  `json:"phase"`
	Status             string                  `json:"status"`
	StartedAt          time.Time               `json:"startedAt"`
	FinishedAt         time.Time               `json:"finishedAt"`
	Workspace          string                  `json:"workspace"`
	ProtocolViolations []protocolViolation     `json:"protocolViolations"`
	Infrastructure     *infrastructureNote     `json:"infrastructure,omitempty"`
	Orchestration      productionOrchestration `json:"orchestration"`
}

type productionOrchestration struct {
	SchedulePath    string            `json:"schedulePath"`
	ScheduleOrdinal int               `json:"scheduleOrdinal"`
	ScheduleRunID   string            `json:"scheduleRunId"`
	Model           string            `json:"model"`
	AgentVersion    string            `json:"agentVersion"`
	Sources         map[string]string `json:"resolvedSources"`
	Images          map[string]string `json:"images"`
	TimeoutSeconds  int               `json:"timeoutSeconds"`
	ResourceLabels  map[string]string `json:"resourceLabels"`
	CandidateExit   int               `json:"candidateExitCode"`
	CandidateSignal string            `json:"candidateSignal,omitempty"`
}

type targetManifest struct {
	FormalPatchSHA256 string `json:"formalPatchSha256"`
	Files             []struct {
		Path, SHA256 string
	} `json:"files"`
}

func productionMain(arguments []string) error {
	var opts productionOptions
	flags := flag.NewFlagSet("production", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.root, "root", ".", "experiment repository root")
	flags.StringVar(&opts.phase, "phase", "", "pilot or measured")
	flags.StringVar(&opts.schedule, "schedule", "", "schedule manifest")
	flags.StringVar(&opts.runDir, "run-dir", "", "new run artifact directory")
	flags.StringVar(&opts.runID, "run-id", "", "schedule run identity")
	flags.StringVar(&opts.candidateRunner, "candidate-runner", "", "candidate runner override")
	flags.StringVar(&opts.runresultBin, "runresult", "", "runresult override")
	flags.StringVar(&opts.evaluatorWrapper, "evaluator-wrapper", "", "evaluator wrapper override")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("production does not accept positional arguments")
	}
	for name, value := range map[string]string{"phase": opts.phase, "schedule": opts.schedule, "run-dir": opts.runDir, "run-id": opts.runID} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("-%s is required", name)
		}
	}
	if opts.phase != "pilot" && opts.phase != "measured" {
		return errors.New("-phase must be pilot or measured")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runProduction(ctx, opts)
}

func runProduction(ctx context.Context, opts productionOptions) error {
	root, err := filepath.Abs(opts.root)
	if err != nil {
		return err
	}
	runDir, err := filepath.Abs(opts.runDir)
	if err != nil {
		return err
	}
	schedulePath, err := filepath.Abs(opts.schedule)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(runDir); !os.IsNotExist(err) {
		if err == nil {
			return fmt.Errorf("run directory already exists: %s", runDir)
		}
		return err
	}
	resolved, err := resolveProduction(root, schedulePath, opts.runID)
	if err != nil {
		return err
	}
	if resolved.Blocker != "" {
		return errors.New(resolved.Blocker)
	}
	release, err := acquireSlot(ctx, filepath.Join(root, ".cache", "production-rundriver-locks"), resolved.MaxConcurrency)
	if err != nil {
		return err
	}
	defer release()
	if err := os.Mkdir(runDir, 0o755); err != nil {
		return fmt.Errorf("create run directory: %w", err)
	}
	workspace := filepath.Join(runDir, "workspace")
	if err := copyTree(filepath.Join(root, resolved.Fixture), workspace, false); err != nil {
		return fmt.Errorf("copy production fixture: %w", err)
	}
	if resolved.Cell.Mode == "propagation-only" {
		if err := applyPropagation(root, workspace, resolved.Cell.Task); err != nil {
			return err
		}
	}
	if resolved.WorkspaceOverlay != "" {
		if err := copyTree(filepath.Join(root, resolved.WorkspaceOverlay), workspace, true); err != nil {
			return fmt.Errorf("apply treatment workspace overlay: %w", err)
		}
	}
	if err := makeWorkspaceWritable(workspace); err != nil {
		return fmt.Errorf("prepare UID 10001 workspace: %w", err)
	}
	starting := filepath.Join(runDir, "starting-workspace")
	if err := copyTree(workspace, starting, false); err != nil {
		return fmt.Errorf("snapshot starting workspace: %w", err)
	}
	treatmentText, err := os.ReadFile(filepath.Join(root, resolved.Treatment))
	if err != nil {
		return fmt.Errorf("read treatment: %w", err)
	}
	taskText, err := os.ReadFile(filepath.Join(root, resolved.Task))
	if err != nil {
		return fmt.Errorf("read task: %w", err)
	}
	prompt := append(append(append([]byte(nil), treatmentText...), '\n', '\n'), taskText...)
	if err := os.WriteFile(filepath.Join(runDir, "prompt.md"), prompt, 0o644); err != nil {
		return err
	}
	for _, name := range []string{"transcript.jsonl", "candidate-stderr.log"} {
		if err := os.WriteFile(filepath.Join(runDir, name), nil, 0o644); err != nil {
			return err
		}
	}
	instance, err := randomInstanceID()
	if err != nil {
		return err
	}
	labels := map[string]string{"experiment.run-id": opts.runID, "experiment.instance-id": instance}
	runner := opts.candidateRunner
	defaultRunner := runner == ""
	if runner == "" {
		runner = filepath.Join(root, "harness", "run-candidate.sh")
	}
	runConfig, err := os.ReadFile(filepath.Join(root, "harness", "opencode-run.json"))
	if err != nil {
		return err
	}
	compactConfig := &bytes.Buffer{}
	if err := json.Compact(compactConfig, runConfig); err != nil {
		return err
	}
	started := time.Now().UTC()
	command := scriptCommand(runner)
	command.Env = withEnvironment(os.Environ(),
		"EXPERIMENT_ROOT="+root, "EXPERIMENT_RUN_ID="+opts.runID, "EXPERIMENT_INSTANCE_ID="+instance,
		"CANDIDATE_WORKSPACE="+workspace, "CANDIDATE_PROMPT_FILE="+filepath.Join(runDir, "prompt.md"),
		"CANDIDATE_TRANSCRIPT="+filepath.Join(runDir, "transcript.jsonl"), "CANDIDATE_STDERR="+filepath.Join(runDir, "candidate-stderr.log"),
		"COORDINATOR_IMAGE="+resolved.CoordinatorImage, "TOOL_IMAGE="+resolved.ToolImage,
		"EVALUATOR_IMAGE="+resolved.EvaluatorImage, "POSTGRES_IMAGE="+resolved.PostgresImage,
		"OPENCODE_MODEL="+resolved.Model, "OPENCODE_CONFIG_CONTENT="+compactConfig.String(),
		fmt.Sprintf("CANDIDATE_TIMEOUT_SECONDS=%d", resolved.TimeoutSeconds))
	candidateContext, cancelCandidate := context.WithTimeout(ctx, time.Duration(resolved.TimeoutSeconds)*time.Second)
	exitCode, candidateSignal, runErr := runCandidate(candidateContext, command)
	deadlineExceeded := candidateContext.Err() == context.DeadlineExceeded
	cancelCandidate()
	if deadlineExceeded {
		exitCode, candidateSignal = 124, ""
	}
	if defaultRunner {
		cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		cleanup := exec.CommandContext(cleanupContext, "bash", runner, "--cleanup")
		cleanup.Env = withEnvironment(os.Environ(), "EXPERIMENT_RUN_ID="+opts.runID, "EXPERIMENT_INSTANCE_ID="+instance)
		stderr, openErr := os.OpenFile(filepath.Join(runDir, "candidate-stderr.log"), os.O_WRONLY|os.O_APPEND, 0o644)
		if openErr == nil {
			cleanup.Stdout, cleanup.Stderr = stderr, stderr
		}
		cleanupErr := cleanup.Run()
		if stderr != nil {
			_ = stderr.Close()
		}
		cancelCleanup()
		if openErr != nil || cleanupErr != nil {
			exitCode = 71
			runErr = fmt.Errorf("post-candidate label cleanup failed: %v %v", openErr, cleanupErr)
		}
	}
	finished := time.Now().UTC()
	status := "submitted"
	var infrastructure *infrastructureNote
	switch exitCode {
	case 0:
	case 124:
		status = "timed-out"
		finished = started.Add(time.Duration(resolved.TimeoutSeconds) * time.Second)
	case 70:
		status = "harness-failure"
		infrastructure = &infrastructureNote{Category: "harness-process-crash", Evidence: "coordinator failed; see candidate-stderr.log"}
	case 71:
		status = "infrastructure-failure"
		infrastructure = &infrastructureNote{Category: "host-container-runtime-failure", Evidence: "tool or container runtime failed; see candidate-stderr.log"}
	default:
		status = "harness-failure"
		infrastructure = &infrastructureNote{Category: "harness-process-crash", Evidence: fmt.Sprintf("candidate runner terminated with exit %d signal %s", exitCode, candidateSignal)}
	}
	unsafeWorkspace := validateWorkspaceTree(workspace)
	if unsafeWorkspace != nil {
		status = "harness-failure"
		infrastructure = &infrastructureNote{Category: "harness-process-crash", Evidence: "candidate workspace rejected: " + unsafeWorkspace.Error()}
		if err := os.WriteFile(filepath.Join(runDir, "final.patch"), nil, 0o644); err != nil {
			return err
		}
	} else if err := generatePatch(runDir, starting, workspace, filepath.Join(runDir, "final.patch")); err != nil {
		return err
	}
	if err := os.RemoveAll(starting); err != nil {
		return fmt.Errorf("remove starting workspace snapshot: %w", err)
	}
	document := productionMetadata{
		RunID: opts.runID, CellID: resolved.Cell.ID, RepeatIndex: resolved.RepeatIndex, Phase: opts.phase,
		Status: status, StartedAt: started, FinishedAt: finished, Workspace: "workspace",
		ProtocolViolations: []protocolViolation{}, Infrastructure: infrastructure,
		Orchestration: productionOrchestration{
			SchedulePath: schedulePath, ScheduleOrdinal: resolved.ScheduleOrdinal, ScheduleRunID: opts.runID,
			Model: resolved.Model, AgentVersion: resolved.AgentVersion,
			Sources:        map[string]string{"fixture": resolved.Fixture, "treatment": resolved.Treatment, "workspaceOverlay": resolved.WorkspaceOverlay, "task": resolved.Task},
			Images:         map[string]string{"coordinator": resolved.CoordinatorImage, "tool": resolved.ToolImage, "toolCodegen": resolved.CodegenImage, "evaluator": resolved.EvaluatorImage, "postgres": resolved.PostgresImage},
			TimeoutSeconds: resolved.TimeoutSeconds, ResourceLabels: labels, CandidateExit: exitCode, CandidateSignal: candidateSignal,
		},
	}
	if err := writeJSONAtomic(filepath.Join(runDir, "metadata.json"), document); err != nil {
		return err
	}
	marker, err := os.OpenFile(filepath.Join(runDir, ".finalization-started"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o444)
	if err != nil {
		return fmt.Errorf("refuse ambiguous finalization: %w", err)
	}
	_ = marker.Close()
	runresult := opts.runresultBin
	if runresult == "" {
		runresult = filepath.Join(root, "bin", "runresult")
	}
	evaluator := opts.evaluatorWrapper
	if evaluator == "" {
		evaluator = filepath.Join(root, "harness", "run-evaluator-container.sh")
	}
	assemblyArgs := []string{"-run-dir", runDir, "-root", root, "-evaluator", evaluator, "-codegen-image", resolved.CodegenImage}
	var assembly *exec.Cmd
	if strings.HasSuffix(runresult, ".sh") {
		assembly = exec.Command("bash", append([]string{runresult}, assemblyArgs...)...)
	} else {
		assembly = exec.Command(runresult, assemblyArgs...)
	}
	assembly.Env = withEnvironment(withoutProviderCredentials(os.Environ()),
		"EXPERIMENT_ROOT="+root, "EXPERIMENT_RUN_ID="+opts.runID, "EXPERIMENT_INSTANCE_ID="+instance,
		"EVALUATOR_IMAGE="+resolved.EvaluatorImage)
	assembly.Stdout, assembly.Stderr = os.Stdout, os.Stderr
	if err := assembly.Run(); err != nil {
		return fmt.Errorf("runresult assembly failed: %w", err)
	}
	if exitCode != 0 && exitCode != 124 {
		if runErr != nil {
			return fmt.Errorf("candidate runner failed: %w", runErr)
		}
		return fmt.Errorf("candidate phase failed with exit %d", exitCode)
	}
	if unsafeWorkspace != nil {
		return unsafeWorkspace
	}
	return nil
}

func resolveProduction(root, schedulePath, runID string) (resolvedRun, error) {
	var config productionConfig
	if err := decodeStrict(filepath.Join(root, "config", "experiment.json"), &config); err != nil {
		return resolvedRun{}, err
	}
	if config.Execution.TimeoutSeconds != productionTimeout || config.Execution.MaxConcurrency != productionConcurrency {
		return resolvedRun{}, fmt.Errorf("production requires timeoutSeconds=%d and maxConcurrency=%d", productionTimeout, productionConcurrency)
	}
	var schedule productionSchedule
	if err := decodeStrict(schedulePath, &schedule); err != nil {
		return resolvedRun{}, err
	}
	var versions productionVersions
	if err := decodeStrict(filepath.Join(root, "config", "versions.json"), &versions); err != nil {
		return resolvedRun{}, err
	}
	var entry *struct {
		Ordinal, RepeatIndex int
		RunID, CellID        string
	}
	for i := range schedule.Runs {
		if schedule.Runs[i].RunID == runID {
			if entry != nil {
				return resolvedRun{}, fmt.Errorf("schedule has duplicate run-id %q", runID)
			}
			entry = &schedule.Runs[i]
		}
	}
	if entry == nil {
		return resolvedRun{}, fmt.Errorf("run-id %q is absent from schedule", runID)
	}
	if entry.Ordinal < 1 || entry.RepeatIndex < 1 || entry.RepeatIndex > 5 {
		return resolvedRun{}, fmt.Errorf("schedule entry %q has invalid ordinal or repeatIndex", runID)
	}
	var cell *productionCell
	for i := range config.Cells {
		if config.Cells[i].ID == entry.CellID {
			cell = &config.Cells[i]
			break
		}
	}
	if cell == nil {
		return resolvedRun{}, fmt.Errorf("schedule cell %q is absent from config", entry.CellID)
	}
	resolved := resolvedRun{Cell: *cell, RepeatIndex: entry.RepeatIndex, ScheduleOrdinal: entry.Ordinal,
		Model: config.Model.ID, AgentVersion: config.Agent.Version, CoordinatorImage: config.FrozenInputs.EnvironmentImages.Coordinator,
		CodegenImage: config.FrozenInputs.EnvironmentImages.ToolCodegen, EvaluatorImage: config.FrozenInputs.EnvironmentImages.Evaluator,
		PostgresImage:  versions.Frozen.PostgresImage,
		TimeoutSeconds: config.Execution.TimeoutSeconds, MaxConcurrency: config.Execution.MaxConcurrency}
	if cell.Stage == "greenfield" {
		resolved.Fixture, resolved.Task = "fixtures/base1", "tasks/part1.md"
		if cell.Treatment == "codegen" {
			resolved.WorkspaceOverlay = "treatments/codegen/workspace"
		}
	} else {
		resolved.Fixture = "fixtures/base2-" + cell.Treatment
		if cell.Mode == "full-workflow" {
			resolved.Task = "tasks/full/" + cell.Task + ".md"
		} else {
			resolved.Task = "tasks/propagation/" + cell.Task + ".md"
		}
	}
	resolved.Treatment = "treatments/" + cell.Treatment + "/overlay.md"
	if cell.Treatment == "direct" {
		resolved.ToolImage = config.FrozenInputs.EnvironmentImages.ToolDirect
	} else {
		resolved.ToolImage = config.FrozenInputs.EnvironmentImages.ToolCodegen
	}
	for name, value := range map[string]string{"model": resolved.Model, "agent version": resolved.AgentVersion, "coordinator image": resolved.CoordinatorImage, "tool image": resolved.ToolImage, "evaluator image": resolved.EvaluatorImage, "PostgreSQL image": resolved.PostgresImage} {
		if value == "" {
			return resolvedRun{}, fmt.Errorf("config has empty %s", name)
		}
	}
	for name, value := range map[string]string{"coordinator": resolved.CoordinatorImage, "tool": resolved.ToolImage, "codegen": resolved.CodegenImage, "evaluator": resolved.EvaluatorImage} {
		if !validDigest(value) {
			return resolvedRun{}, fmt.Errorf("%s image is not an immutable sha256 digest", name)
		}
	}
	if !validNamedDigest(resolved.PostgresImage) {
		return resolvedRun{}, errors.New("PostgreSQL image is not an immutable named sha256 digest")
	}
	return resolved, nil
}

func scriptCommand(path string) *exec.Cmd {
	if strings.HasSuffix(path, ".sh") {
		return exec.Command("bash", path)
	}
	return exec.Command(path)
}

func withEnvironment(environment []string, overrides ...string) []string {
	keys := make(map[string]bool, len(overrides))
	for _, value := range overrides {
		keys[strings.SplitN(value, "=", 2)[0]] = true
	}
	result := make([]string, 0, len(environment)+len(overrides))
	for _, value := range environment {
		if !keys[strings.SplitN(value, "=", 2)[0]] {
			result = append(result, value)
		}
	}
	return append(result, overrides...)
}

func decodeStrict(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	// Config and schedule contain fields not needed by this driver; their schemas own strictness.
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validNamedDigest(value string) bool {
	parts := strings.Split(value, "@")
	return len(parts) == 2 && parts[0] != "" && validDigest(parts[1])
}

func randomInstanceID() (string, error) {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func acquireSlot(ctx context.Context, directory string, count int) (func(), error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, err
	}
	for {
		for slot := 1; slot <= count; slot++ {
			path := filepath.Join(directory, fmt.Sprintf("slot-%d.lock", slot))
			file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
			if err != nil {
				return nil, err
			}
			if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
				return func() {
					_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
					_ = file.Close()
				}, nil
			}
			_ = file.Close()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func makeWorkspaceWritable(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm()
		if entry.IsDir() {
			mode |= 0o777
		} else {
			mode |= 0o222
		}
		return os.Chmod(path, mode)
	})
}

func validateWorkspaceTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", rel)
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") {
				return fmt.Errorf("hidden directory is outside workspace-manifest coverage: %s", rel)
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special file is not allowed: %s", rel)
		}
		return nil
	})
}

func applyPropagation(root, workspace, task string) error {
	directory := filepath.Join(root, "tasks", "propagation", task)
	var manifest targetManifest
	if err := decodeStrict(filepath.Join(directory, "target-manifest.json"), &manifest); err != nil {
		return err
	}
	patchPath := filepath.Join(directory, "formal.patch")
	patchData, err := os.ReadFile(patchPath)
	if err != nil {
		return err
	}
	if sha256Hex(patchData) != manifest.FormalPatchSHA256 {
		return errors.New("formal.patch SHA-256 does not match target manifest")
	}
	for _, line := range strings.Split(string(patchData), "\n") {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return errors.New("formal patch contains a malformed path header")
			}
			path := fields[1]
			if path != "/dev/null" {
				path = strings.TrimPrefix(strings.TrimPrefix(path, "a/"), "b/")
				if !safeRelativePath(path) {
					return fmt.Errorf("unsafe patch path %q", path)
				}
			}
		}
	}
	command := exec.Command("patch", "--silent", "-p1", "-d", workspace)
	command.Stdin = bytes.NewReader(patchData)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("apply formal patch: %w: %s", err, bytes.TrimSpace(output))
	}
	seen := map[string]bool{}
	for _, file := range manifest.Files {
		if seen[file.Path] || !safeRelativePath(file.Path) {
			return fmt.Errorf("unsafe or duplicate target path %q", file.Path)
		}
		seen[file.Path] = true
		data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(file.Path)))
		if err != nil {
			return err
		}
		if sha256Hex(data) != file.SHA256 {
			return fmt.Errorf("propagated target %s hash mismatch", file.Path)
		}
	}
	return nil
}

func safeRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "\\") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	return clean == path && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func sha256Hex(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func runCandidate(ctx context.Context, command *exec.Cmd) (int, string, error) {
	if err := command.Start(); err != nil {
		return 71, "", err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	var err error
	cancelled := false
	select {
	case err = <-done:
	case <-ctx.Done():
		cancelled = true
		_ = command.Process.Signal(os.Interrupt)
		select {
		case err = <-done:
		case <-time.After(5 * time.Second):
			_ = command.Process.Kill()
			err = <-done
		}
	}
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exit = exitErr.ExitCode()
		} else {
			return 71, "", err
		}
	}
	signalName := ""
	if state := command.ProcessState; state != nil {
		if wait, ok := state.Sys().(syscall.WaitStatus); ok && wait.Signaled() {
			signalName = wait.Signal().String()
		}
	}
	if cancelled && signalName == "" {
		signalName = "context-cancelled"
	}
	return exit, signalName, err
}

func generatePatch(runDir, starting, workspace, output string) error {
	command := exec.Command("git", "diff", "--no-index", "--binary", "--no-prefix", "--", filepath.Base(starting), filepath.Base(workspace))
	command.Dir = runDir
	data, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return fmt.Errorf("generate final patch: %w", err)
		}
	}
	oldPrefix, newPrefix := []byte(filepath.Base(starting)+"/"), []byte(filepath.Base(workspace)+"/")
	lines := bytes.SplitAfter(data, []byte("\n"))
	for index, line := range lines {
		if bytes.HasPrefix(line, []byte("diff --git ")) || bytes.HasPrefix(line, []byte("--- ")) ||
			bytes.HasPrefix(line, []byte("+++ ")) || bytes.HasPrefix(line, []byte("Binary files ")) ||
			bytes.HasPrefix(line, []byte("rename from ")) || bytes.HasPrefix(line, []byte("rename to ")) {
			line = bytes.ReplaceAll(line, oldPrefix, []byte("a/"))
			line = bytes.ReplaceAll(line, newPrefix, []byte("b/"))
			lines[index] = line
		}
	}
	data = bytes.Join(lines, nil)
	return os.WriteFile(output, data, 0o644)
}
