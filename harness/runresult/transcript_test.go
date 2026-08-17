package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "transcript.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestParseTranscriptUsageAndEvents(t *testing.T) {
	dir := writeTranscript(t,
		`{"type":"step_start","timestamp":1000,"part":{"type":"step-start"}}`,
		`{"type":"tool_use","timestamp":2000,"part":{"type":"tool","tool":"read","state":{"status":"completed","input":{"filePath":"/workspace/main.go"},"output":"package main","time":{"start":1500,"end":1900}}}}`,
		`{"type":"step_finish","timestamp":3000,"part":{"type":"step-finish","tokens":{"total":100,"input":70,"output":30,"reasoning":0,"cache":{"write":0,"read":5}}}}`,
		`{"type":"tool_use","timestamp":4000,"part":{"type":"tool","tool":"bash","state":{"status":"completed","input":{"command":"go build ./...","workdir":"/workspace"},"output":"ok","metadata":{"exit":0},"time":{"start":3500,"end":3900}}}}`,
		`{"type":"tool_use","timestamp":5000,"part":{"type":"tool","tool":"edit","state":{"status":"completed","input":{"filePath":"/workspace/main.go"},"output":"ok","time":{"start":4500,"end":4900}}}}`,
		`{"type":"step_finish","timestamp":6000,"part":{"type":"step-finish","tokens":{"total":60,"input":40,"output":20,"reasoning":0,"cache":{"write":0,"read":0}}}}`,
	)
	usage, events, process, err := parseTranscript(dir, "transcript.jsonl", "direct")
	if err != nil {
		t.Fatal(err)
	}
	if usage.Turns != 2 || usage.ToolCalls != 3 {
		t.Fatalf("usage = %+v", usage)
	}
	if usage.InputTokens == nil || *usage.InputTokens != 110 {
		t.Fatalf("input tokens = %v", usage.InputTokens)
	}
	if usage.OutputTokens == nil || *usage.OutputTokens != 50 {
		t.Fatalf("output tokens = %v", usage.OutputTokens)
	}
	if events[0].Category != "read" || events[1].Category != "build" || events[2].Category != "edit" {
		t.Fatalf("categories = %s, %s, %s", events[0].Category, events[1].Category, events[2].Category)
	}
	if events[1].ExitCode == nil || *events[1].ExitCode != 0 {
		t.Fatalf("bash exit code = %v", events[1].ExitCode)
	}
	if process.FilesTouched != 1 {
		t.Fatalf("files touched = %d", process.FilesTouched)
	}
	if process.RepairIterations != 0 {
		t.Fatalf("repair iterations = %d", process.RepairIterations)
	}
}

func TestParseTranscriptFailedBuildCompilerEvent(t *testing.T) {
	dir := writeTranscript(t,
		`{"type":"tool_use","timestamp":1000,"part":{"type":"tool","tool":"bash","state":{"status":"completed","input":{"command":"go build ./..."},"output":"# tasks/internal/service\ninternal/service/handler.go:12:3: undefined: thing","metadata":{"exit":1},"time":{"start":900,"end":990}}}}`,
	)
	_, events, process, err := parseTranscript(dir, "transcript.jsonl", "direct")
	if err != nil {
		t.Fatal(err)
	}
	if process.RepairIterations != 1 {
		t.Fatalf("repair iterations = %d", process.RepairIterations)
	}
	if len(process.CompilerEvents) != 1 {
		t.Fatalf("compiler events = %d", len(process.CompilerEvents))
	}
	compiler := process.CompilerEvents[0]
	if compiler.Category != "handwritten" {
		t.Fatalf("category = %s", compiler.Category)
	}
	if len(compiler.Files) != 1 || compiler.Files[0] != "internal/service/handler.go" {
		t.Fatalf("files = %v", compiler.Files)
	}
	if len(compiler.Locations) != 1 || compiler.Locations[0] != "internal/service/handler.go:12:3" {
		t.Fatalf("locations = %v", compiler.Locations)
	}
	if compiler.PointsToHandwrittenAdaptation == nil || !*compiler.PointsToHandwrittenAdaptation {
		t.Fatal("pointsToHandwrittenAdaptation must be true")
	}
	if compiler.FollowedByRelevantRepair != nil {
		t.Fatal("followedByRelevantRepair must stay null until the analysis freeze")
	}
	if events[0].Index != 0 || compiler.CommandIndex != 0 {
		t.Fatal("command index must reference the command event")
	}
}

func TestParseTranscriptGeneratedDiagnostics(t *testing.T) {
	dir := writeTranscript(t,
		`{"type":"tool_use","timestamp":1000,"part":{"type":"tool","tool":"bash","state":{"status":"completed","input":{"command":"go test ./..."},"output":"internal/repository/generated/db.go:5:1: syntax error","metadata":{"exit":1},"time":{"start":900,"end":990}}}}`,
	)
	_, _, process, err := parseTranscript(dir, "transcript.jsonl", "codegen")
	if err != nil {
		t.Fatal(err)
	}
	if len(process.CompilerEvents) != 1 {
		t.Fatalf("compiler events = %d", len(process.CompilerEvents))
	}
	compiler := process.CompilerEvents[0]
	if compiler.Category != "generated-type" {
		t.Fatalf("category = %s", compiler.Category)
	}
	if compiler.PointsToHandwrittenAdaptation == nil || *compiler.PointsToHandwrittenAdaptation {
		t.Fatal("pointsToHandwrittenAdaptation must be false for generated-only diagnostics")
	}
}

func TestParseTranscriptEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "transcript.jsonl"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	usage, events, process, err := parseTranscript(dir, "transcript.jsonl", "direct")
	if err != nil {
		t.Fatal(err)
	}
	if usage.Turns != 0 || usage.ToolCalls != 0 || usage.InputTokens != nil || usage.OutputTokens != nil {
		t.Fatalf("usage = %+v", usage)
	}
	if len(events) != 0 || process.RepairIterations != 0 || process.FilesTouched != 0 {
		t.Fatalf("events = %d process = %+v", len(events), process)
	}
}

func TestParseTranscriptMalformedLine(t *testing.T) {
	dir := writeTranscript(t, `{"type":"step_finish", broken`)
	if _, _, _, err := parseTranscript(dir, "transcript.jsonl", "direct"); err == nil {
		t.Fatal("a malformed transcript line must fail loudly")
	}
}

func TestClassifyBash(t *testing.T) {
	cases := map[string]string{
		"oapi-codegen --config oapi-codegen.yaml api/openapi.yaml": "generate",
		"sqlc generate -f sqlc.yaml":                               "generate",
		"make generate":                                            "generate",
		"make verify-generate":                                     "generate",
		"make migrate":                                             "migration",
		"go run ./cmd/migrate":                                     "migration",
		"go build ./...":                                           "build",
		"make build":                                               "build",
		"go vet ./...":                                             "build",
		"go test ./...":                                            "test",
		"make test":                                                "test",
		"make check":                                               "test",
		"make run":                                                 "service",
		"go run ./cmd/task-service":                                "service",
		"curl http://localhost:8080/tasks":                         "other",
		"git diff":                                                 "other",
	}
	for command, want := range cases {
		if got := classifyBash(command); got != want {
			t.Errorf("classifyBash(%q) = %s, want %s", command, got, want)
		}
	}
}

func TestEventTouchedFilesApplyPatch(t *testing.T) {
	dir := writeTranscript(t,
		`{"type":"tool_use","timestamp":1000,"part":{"type":"tool","tool":"apply_patch","state":{"status":"completed","input":{"patchText":"*** Begin Patch\n*** Add File: /workspace/new.go\n+package main\n*** Update File: existing.go\n@@\n-x\n+y\n*** End Patch\n"},"output":"ok","time":{"start":900,"end":990}}}}`,
		`{"type":"tool_use","timestamp":2000,"part":{"type":"tool","tool":"write","state":{"status":"completed","input":{"filePath":"/workspace/new.go","content":"package main\n"},"output":"ok","time":{"start":1900,"end":1990}}}}`,
	)
	_, _, process, err := parseTranscript(dir, "transcript.jsonl", "direct")
	if err != nil {
		t.Fatal(err)
	}
	if process.FilesTouched != 2 {
		t.Fatalf("files touched = %d, want 2", process.FilesTouched)
	}
}

func TestWriteCommands(t *testing.T) {
	dir := t.TempDir()
	events := []commandEvent{{Index: 0, Command: "go build ./...", Category: "build", stdout: "out", stderr: "err"}}
	if err := writeCommands(dir, "commands.json", events); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"commands.json", "commands/0000.stdout.txt", "commands/0000.stderr.txt"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err != nil || info.IsDir() {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "commands", "0000.stdout.txt"))
	if err != nil || string(data) != "out" {
		t.Fatalf("stdout capture = %q, %v", data, err)
	}
}
