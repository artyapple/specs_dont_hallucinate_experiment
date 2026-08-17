package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// commandEvent mirrors schemas/command-event.schema.json. The assembler derives
// these events deterministically from the OpenCode JSON transcript and stores
// captured tool output under the run directory commands/ subdirectory.
type commandEvent struct {
	Index            int64  `json:"index"`
	StartedAt        string `json:"startedAt"`
	FinishedAt       string `json:"finishedAt"`
	Command          string `json:"command"`
	WorkingDirectory string `json:"workingDirectory,omitempty"`
	ExitCode         *int64 `json:"exitCode"`
	TimedOut         bool   `json:"timedOut"`
	Category         string `json:"category"`
	StdoutPath       string `json:"stdoutPath"`
	StderrPath       string `json:"stderrPath"`

	stdout string
	stderr string
}

type transcriptLine struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	Part      *struct {
		Type  string `json:"type"`
		Tool  string `json:"tool"`
		State *struct {
			Status   string          `json:"status"`
			Input    json.RawMessage `json:"input"`
			Output   string          `json:"output"`
			Error    string          `json:"error"`
			Metadata json.RawMessage `json:"metadata"`
			Time     *struct {
				Start int64 `json:"start"`
				End   int64 `json:"end"`
			} `json:"time"`
		} `json:"state"`
		Tokens *struct {
			Input  *int64 `json:"input"`
			Output *int64 `json:"output"`
		} `json:"tokens"`
	} `json:"part"`
}

// parseTranscript reads the OpenCode --format json transcript and derives
// usage counters, command events, and process metrics. The extraction rules
// are draft until the analysis freeze and are documented in harness/README.md.
func parseTranscript(runDir, name, treatment string) (Usage, []commandEvent, Process, error) {
	usage := Usage{}
	process := Process{CompilerEvents: []CompilerEvent{}}
	events := []commandEvent{}
	touched := map[string]bool{}

	file, err := os.Open(filepath.Join(runDir, name))
	if err != nil {
		return usage, nil, process, err
	}
	defer file.Close()

	var inputSum, outputSum int64
	tokenLines := int64(0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 16<<20)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var line transcriptLine
		if err := json.Unmarshal([]byte(text), &line); err != nil {
			return usage, nil, process, fmt.Errorf("transcript line %d: %w", lineNumber, err)
		}
		if line.Part == nil {
			continue
		}
		switch {
		case line.Type == "step_finish":
			usage.Turns++
			if line.Part.Tokens != nil {
				if line.Part.Tokens.Input != nil {
					inputSum += *line.Part.Tokens.Input
					tokenLines++
				}
				if line.Part.Tokens.Output != nil {
					outputSum += *line.Part.Tokens.Output
				}
			}
		case line.Type == "tool_use" && line.Part.State != nil:
			event := buildCommandEvent(int64(len(events)), line)
			events = append(events, event)
			for _, path := range eventTouchedFiles(line) {
				touched[path] = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return usage, nil, process, fmt.Errorf("read transcript: %w", err)
	}
	if tokenLines > 0 {
		usage.InputTokens = &inputSum
		usage.OutputTokens = &outputSum
	}
	usage.ToolCalls = int64(len(events))

	for i := range events {
		failed := events[i].ExitCode == nil || *events[i].ExitCode != 0
		verification := events[i].Category == "build" || events[i].Category == "test"
		if failed && verification {
			process.RepairIterations++
			if compiler, ok := extractCompilerEvent(events[i], treatment); ok {
				process.CompilerEvents = append(process.CompilerEvents, compiler)
			}
		}
	}
	process.FilesTouched = int64(len(touched))
	return usage, events, process, nil
}

func buildCommandEvent(index int64, line transcriptLine) commandEvent {
	part := line.Part
	state := part.State
	event := commandEvent{Index: index, Category: "other"}
	start, end := line.Timestamp, line.Timestamp
	if state.Time != nil {
		start, end = state.Time.Start, state.Time.End
	}
	event.StartedAt = time.UnixMilli(start).UTC().Format(time.RFC3339Nano)
	event.FinishedAt = time.UnixMilli(end).UTC().Format(time.RFC3339Nano)
	event.stdout = state.Output
	event.stderr = state.Error

	completed := state.Status == "completed"
	switch part.Tool {
	case "bash":
		var input struct {
			Command string `json:"command"`
			Workdir string `json:"workdir"`
		}
		_ = json.Unmarshal(state.Input, &input)
		event.Command = input.Command
		event.WorkingDirectory = input.Workdir
		event.Category = classifyBash(input.Command)
		var metadata struct {
			Exit *int64 `json:"exit"`
		}
		_ = json.Unmarshal(state.Metadata, &metadata)
		if completed && metadata.Exit != nil {
			event.ExitCode = metadata.Exit
		}
		event.TimedOut = !completed && indicatesTimeout(state.Output+" "+state.Error)
	case "read", "edit", "write":
		var input struct {
			FilePath string `json:"filePath"`
		}
		_ = json.Unmarshal(state.Input, &input)
		event.Command = firstNonEmpty(input.FilePath, part.Tool)
		event.Category = part.Tool
		if part.Tool != "read" {
			event.Category = "edit"
		}
		if completed {
			zero := int64(0)
			event.ExitCode = &zero
		}
	case "apply_patch":
		event.Command = "apply_patch"
		event.Category = "edit"
		if completed {
			zero := int64(0)
			event.ExitCode = &zero
		}
	default:
		event.Command = firstNonEmpty(part.Tool, "unknown")
		if completed {
			zero := int64(0)
			event.ExitCode = &zero
		}
	}
	if event.Command == "" {
		event.Command = part.Tool
	}
	return event
}

// classifyBash assigns the draft command category. The network-attempt
// category is reserved for network-layer evidence and is never assigned from
// command text alone.
func classifyBash(command string) string {
	switch {
	case strings.Contains(command, "oapi-codegen"), strings.Contains(command, "sqlc"),
		strings.Contains(command, "make generate"), strings.Contains(command, "make verify-generate"):
		return "generate"
	case strings.Contains(command, "migrate"):
		return "migration"
	case strings.Contains(command, "go build"), strings.Contains(command, "make build"),
		strings.Contains(command, "go vet"):
		return "build"
	case strings.Contains(command, "go test"), strings.Contains(command, "make test"),
		strings.Contains(command, "make check"):
		return "test"
	case strings.Contains(command, "make run"), strings.Contains(command, "task-service"):
		return "service"
	default:
		return "other"
	}
}

func containsMakeCheck(command string) bool {
	return strings.Contains(command, "make check")
}

func indicatesTimeout(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout")
}

// eventTouchedFiles reports workspace-relative paths modified by edit, write,
// or apply_patch tool calls. Bash side effects are not tracked.
func eventTouchedFiles(line transcriptLine) []string {
	state := line.Part.State
	workspaceRelative := func(path string) string {
		path = filepath.ToSlash(path)
		return strings.TrimPrefix(path, "/workspace/")
	}
	switch line.Part.Tool {
	case "edit", "write":
		var input struct {
			FilePath string `json:"filePath"`
		}
		if err := json.Unmarshal(state.Input, &input); err == nil && input.FilePath != "" {
			return []string{workspaceRelative(input.FilePath)}
		}
	case "apply_patch":
		var input struct {
			PatchText string `json:"patchText"`
		}
		if err := json.Unmarshal(state.Input, &input); err != nil {
			return nil
		}
		paths := []string{}
		for _, patchLine := range strings.Split(strings.ReplaceAll(input.PatchText, "\r\n", "\n"), "\n") {
			for _, prefix := range []string{"*** Add File: ", "*** Update File: ", "*** Delete File: ", "*** Move to: "} {
				if strings.HasPrefix(patchLine, prefix) {
					paths = append(paths, workspaceRelative(strings.TrimSpace(strings.TrimPrefix(patchLine, prefix))))
				}
			}
		}
		return paths
	}
	return nil
}

var goDiagnosticPattern = regexp.MustCompile(`(?m)^([A-Za-z0-9_./-]+\.go):([0-9]+):([0-9]+):`)

// extractCompilerEvent codes Go compiler diagnostics from one failed build or
// test command. followedByRelevantRepair stays null until the analysis freeze
// defines repair-relevance coding.
func extractCompilerEvent(event commandEvent, treatment string) (CompilerEvent, bool) {
	matches := goDiagnosticPattern.FindAllStringSubmatch(event.stdout+"\n"+event.stderr, -1)
	if len(matches) == 0 {
		return CompilerEvent{}, false
	}
	files := map[string]bool{}
	locations := map[string]bool{}
	anyHandwritten := false
	for _, match := range matches {
		file := strings.TrimPrefix(match[1], "./")
		files[file] = true
		locations[file+":"+match[2]+":"+match[3]] = true
		if !isGeneratedPath(file, treatment) {
			anyHandwritten = true
		}
	}
	category := "handwritten"
	if !anyHandwritten {
		category = "generated-type"
	}
	return CompilerEvent{
		CommandIndex:                  event.Index,
		Category:                      category,
		Files:                         sortedKeys(files),
		Locations:                     sortedKeys(locations),
		PointsToHandwrittenAdaptation: &anyHandwritten,
		FollowedByRelevantRepair:      nil,
	}, true
}

// isGeneratedPath recognizes the frozen generated outputs of the Codegen
// fixture. Direct runs never have generated paths.
func isGeneratedPath(path, treatment string) bool {
	if treatment != "codegen" {
		return false
	}
	return path == "internal/httpapi/generated.gen.go" ||
		strings.HasPrefix(path, "internal/repository/generated/")
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// writeCommands stores per-event captured output and the commands.json index.
func writeCommands(runDir, name string, events []commandEvent) error {
	commandsDir := filepath.Join(runDir, "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		return fmt.Errorf("create commands directory: %w", err)
	}
	for i := range events {
		stdoutRel := fmt.Sprintf("commands/%04d.stdout.txt", events[i].Index)
		stderrRel := fmt.Sprintf("commands/%04d.stderr.txt", events[i].Index)
		if err := os.WriteFile(filepath.Join(runDir, stdoutRel), []byte(events[i].stdout), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", stdoutRel, err)
		}
		if err := os.WriteFile(filepath.Join(runDir, stderrRel), []byte(events[i].stderr), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", stderrRel, err)
		}
		events[i].StdoutPath = stdoutRel
		events[i].StderrPath = stderrRel
	}
	encoded, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", name, err)
	}
	if err := os.WriteFile(filepath.Join(runDir, name), append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
