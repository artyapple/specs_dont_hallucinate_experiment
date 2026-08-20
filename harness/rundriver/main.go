// Command rundriver preserves the original synthetic finalization CLI and adds
// the explicit production subcommand for containerized candidate execution.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const timeout = 45 * time.Minute

type options struct {
	root, runDir, runID, cellID, phase, status string
	workspaceSource, workspaceOverlay          string
	transcriptSource, patchSource              string
	startedAt, finishedAt                      string
	infrastructureCategory                     string
	infrastructureEvidence                     string
	runresultBin, evaluatorBin                 string
	repeatIndex                                int
}

type metadata struct {
	RunID              string              `json:"runId"`
	CellID             string              `json:"cellId"`
	RepeatIndex        int                 `json:"repeatIndex"`
	Phase              string              `json:"phase"`
	Status             string              `json:"status"`
	StartedAt          time.Time           `json:"startedAt"`
	FinishedAt         time.Time           `json:"finishedAt"`
	Workspace          string              `json:"workspace"`
	ProtocolViolations []protocolViolation `json:"protocolViolations"`
	Infrastructure     *infrastructureNote `json:"infrastructure,omitempty"`
}

type protocolViolation struct {
	Category                   string `json:"category"`
	Timestamp                  string `json:"timestamp"`
	Evidence                   string `json:"evidence"`
	IsolationForcedTermination bool   `json:"isolationForcedTermination"`
}

type infrastructureNote struct {
	Category string `json:"category"`
	Evidence string `json:"evidence"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "production" {
		if err := productionMain(os.Args[2:]); err != nil {
			fail(err.Error())
		}
		return
	}
	var opts options
	flag.StringVar(&opts.root, "root", ".", "experiment repository root")
	flag.StringVar(&opts.runDir, "run-dir", "", "new run artifact directory")
	flag.StringVar(&opts.runID, "run-id", "", "run identity")
	flag.StringVar(&opts.cellID, "cell-id", "", "experiment matrix cell identity")
	flag.IntVar(&opts.repeatIndex, "repeat-index", 1, "cell repeat index")
	flag.StringVar(&opts.phase, "phase", "pilot", "pilot or measured")
	flag.StringVar(&opts.status, "status", "", "submitted, timed-out, infrastructure-failure, or harness-failure")
	flag.StringVar(&opts.workspaceSource, "workspace-source", "", "canonical workspace source directory")
	flag.StringVar(&opts.workspaceOverlay, "workspace-overlay", "", "optional synthetic final-workspace overlay")
	flag.StringVar(&opts.transcriptSource, "transcript-source", "", "synthetic OpenCode-format JSONL transcript")
	flag.StringVar(&opts.patchSource, "patch-source", "", "synthetic final patch")
	flag.StringVar(&opts.startedAt, "started-at", "", "RFC3339 run start")
	flag.StringVar(&opts.finishedAt, "finished-at", "", "RFC3339 run finish")
	flag.StringVar(&opts.infrastructureCategory, "infrastructure-category", "", "failure-policy category")
	flag.StringVar(&opts.infrastructureEvidence, "infrastructure-evidence", "", "non-secret failure evidence")
	flag.StringVar(&opts.runresultBin, "runresult", "", "runresult binary (default <root>/bin/runresult)")
	flag.StringVar(&opts.evaluatorBin, "evaluator", "", "evaluator binary forwarded to runresult")
	flag.Parse()
	if flag.NArg() != 0 {
		fail("positional arguments are not accepted")
	}
	if err := run(opts); err != nil {
		fail(err.Error())
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "rundriver:", message)
	os.Exit(2)
}

func run(opts options) error {
	if err := validateOptions(&opts); err != nil {
		return err
	}
	root, err := filepath.Abs(opts.root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	runDir, err := filepath.Abs(opts.runDir)
	if err != nil {
		return fmt.Errorf("resolve run directory: %w", err)
	}
	if _, err := os.Lstat(runDir); !os.IsNotExist(err) {
		if err == nil {
			return fmt.Errorf("run directory already exists: %s", runDir)
		}
		return fmt.Errorf("inspect run directory: %w", err)
	}
	if err := os.Mkdir(runDir, 0o755); err != nil {
		return fmt.Errorf("create run directory: %w", err)
	}
	workspace := filepath.Join(runDir, "workspace")
	if err := copyTree(opts.workspaceSource, workspace, false); err != nil {
		return fmt.Errorf("copy canonical workspace: %w", err)
	}
	if opts.workspaceOverlay != "" {
		if err := copyTree(opts.workspaceOverlay, workspace, true); err != nil {
			return fmt.Errorf("apply synthetic workspace overlay: %w", err)
		}
	}
	for source, name := range map[string]string{
		opts.transcriptSource: "transcript.jsonl",
		opts.patchSource:      "final.patch",
	} {
		if err := copyFile(source, filepath.Join(runDir, name), 0o644, false); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	started, _ := time.Parse(time.RFC3339, opts.startedAt)
	finished, _ := time.Parse(time.RFC3339, opts.finishedAt)
	document := metadata{
		RunID: opts.runID, CellID: opts.cellID, RepeatIndex: opts.repeatIndex,
		Phase: opts.phase, Status: opts.status, StartedAt: started, FinishedAt: finished,
		Workspace: "workspace", ProtocolViolations: []protocolViolation{},
	}
	if opts.infrastructureCategory != "" {
		document.Infrastructure = &infrastructureNote{Category: opts.infrastructureCategory, Evidence: opts.infrastructureEvidence}
	}
	if err := writeJSONAtomic(filepath.Join(runDir, "metadata.json"), document); err != nil {
		return err
	}
	runresult := opts.runresultBin
	if runresult == "" {
		runresult = filepath.Join(root, "bin", "runresult")
	}
	args := []string{"-run-dir", runDir, "-root", root}
	if opts.evaluatorBin != "" {
		args = append(args, "-evaluator", opts.evaluatorBin)
	}
	command := exec.Command(runresult, args...)
	command.Env = withoutProviderCredentials(os.Environ())
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("runresult assembly failed: %w", err)
	}
	return nil
}

func validateOptions(opts *options) error {
	for label, value := range map[string]string{
		"run-dir": opts.runDir, "run-id": opts.runID, "cell-id": opts.cellID,
		"status": opts.status, "workspace-source": opts.workspaceSource,
		"transcript-source": opts.transcriptSource, "patch-source": opts.patchSource,
		"started-at": opts.startedAt, "finished-at": opts.finishedAt,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("-%s is required", label)
		}
	}
	if opts.repeatIndex < 1 || opts.repeatIndex > 5 {
		return errors.New("-repeat-index must be in 1..5")
	}
	if opts.phase != "pilot" && opts.phase != "measured" {
		return errors.New("-phase must be pilot or measured")
	}
	started, err := time.Parse(time.RFC3339, opts.startedAt)
	if err != nil {
		return fmt.Errorf("parse -started-at: %w", err)
	}
	finished, err := time.Parse(time.RFC3339, opts.finishedAt)
	if err != nil {
		return fmt.Errorf("parse -finished-at: %w", err)
	}
	duration := finished.Sub(started)
	if duration < 0 || duration > timeout {
		return errors.New("run timestamps must define a duration from zero through 45 minutes")
	}
	failure := opts.status == "infrastructure-failure" || opts.status == "harness-failure"
	if opts.status != "submitted" && opts.status != "timed-out" && !failure {
		return fmt.Errorf("unsupported -status %q", opts.status)
	}
	if opts.status == "timed-out" && duration != timeout {
		return errors.New("timed-out synthetic runs must finish exactly 45 minutes after start")
	}
	if failure != (opts.infrastructureCategory != "" && opts.infrastructureEvidence != "") {
		return errors.New("failure statuses require both infrastructure fields; other statuses forbid them")
	}
	return nil
}

func copyTree(source, destination string, overwrite bool) error {
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("source %s is not a directory", source)
	}
	if !overwrite {
		if err := os.Mkdir(destination, 0o755); err != nil {
			return err
		}
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == "." {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", rel)
		}
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".") {
			return fmt.Errorf("hidden directory is outside workspace-manifest coverage: %s", rel)
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if !fileInfo.Mode().IsRegular() {
			return fmt.Errorf("special file is not allowed: %s", rel)
		}
		return copyFile(path, target, fileInfo.Mode().Perm(), overwrite)
	})
}

func copyFile(source, destination string, mode fs.FileMode, overwrite bool) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	output, err := os.OpenFile(destination, flags, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return nil
}

func withoutProviderCredentials(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, item := range environment {
		if !strings.HasPrefix(item, "OPENROUTER_API_KEY=") {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
