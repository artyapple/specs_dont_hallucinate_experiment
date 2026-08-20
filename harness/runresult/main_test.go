package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validMetadataJSON() string {
	return `{
  "runId": "run-1",
  "cellId": "greenfield-direct",
  "repeatIndex": 1,
  "phase": "pilot",
  "status": "submitted",
  "startedAt": "2026-08-17T10:00:00Z",
  "finishedAt": "2026-08-17T10:10:00Z",
  "protocolViolations": []
}`
}

func writeMetadata(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metadata.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadMetadataValid(t *testing.T) {
	metadata, err := readMetadata(writeMetadata(t, validMetadataJSON()))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.RunID != "run-1" || metadata.CellID != "greenfield-direct" || metadata.Status != statusSubmitted {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func TestReadMetadataAcceptsStrictProductionOrchestration(t *testing.T) {
	content := strings.TrimSuffix(validMetadataJSON(), "}") + `,
  "orchestration": {
    "schedulePath": "/schedule.json", "scheduleOrdinal": 1, "scheduleRunId": "run-1",
    "model": "openrouter/model", "agentVersion": "1.18.18",
    "resolvedSources": {"fixture": "fixtures/base1"},
    "images": {"coordinator": "sha256:abc"}, "timeoutSeconds": 2700,
    "resourceLabels": {"experiment.run-id": "run-1"}, "candidateExitCode": 0
  }
}`
	metadata, err := readMetadata(writeMetadata(t, content))
	if err != nil || metadata.Orchestration == nil || metadata.Orchestration.TimeoutSeconds != 2700 {
		t.Fatalf("metadata = %+v, %v", metadata, err)
	}
	bad := strings.Replace(content, `"candidateExitCode": 0`, `"candidateExitCode": 0, "unknown": true`, 1)
	if _, err := readMetadata(writeMetadata(t, bad)); err == nil {
		t.Fatal("unknown production orchestration field must be rejected")
	}
}

func TestReadMetadataRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"missing cell":  `{"runId":"r","repeatIndex":1,"phase":"pilot","status":"submitted","startedAt":"2026-08-17T10:00:00Z","finishedAt":"2026-08-17T10:10:00Z"}`,
		"bad repeat":    `{"runId":"r","cellId":"c","repeatIndex":6,"phase":"pilot","status":"submitted","startedAt":"2026-08-17T10:00:00Z","finishedAt":"2026-08-17T10:10:00Z"}`,
		"bad phase":     `{"runId":"r","cellId":"c","repeatIndex":1,"phase":"other","status":"submitted","startedAt":"2026-08-17T10:00:00Z","finishedAt":"2026-08-17T10:10:00Z"}`,
		"unknown field": `{"runId":"r","cellId":"c","repeatIndex":1,"phase":"pilot","status":"submitted","startedAt":"2026-08-17T10:00:00Z","finishedAt":"2026-08-17T10:10:00Z","surprise":true}`,
		"wall too long": `{"runId":"r","cellId":"c","repeatIndex":1,"phase":"pilot","status":"timed-out","startedAt":"2026-08-17T10:00:00Z","finishedAt":"2026-08-17T10:50:00Z"}`,
		"inverted":      `{"runId":"r","cellId":"c","repeatIndex":1,"phase":"pilot","status":"submitted","startedAt":"2026-08-17T10:10:00Z","finishedAt":"2026-08-17T10:00:00Z"}`,
	}
	for name, content := range cases {
		if _, err := readMetadata(writeMetadata(t, content)); err == nil {
			t.Errorf("%s: must reject", name)
		}
	}
}

func TestResolveCell(t *testing.T) {
	cell, err := resolveCell(filepath.Join("..", "..", "config", "experiment.json"), "nullable-patch-propagation-codegen")
	if err != nil {
		t.Fatal(err)
	}
	if cell.Stage != "existing-service" || cell.Task != "nullable-patch" || cell.Mode != "propagation-only" || cell.Treatment != "codegen" {
		t.Fatalf("cell = %+v", cell)
	}
	if _, err := resolveCell(filepath.Join("..", "..", "config", "experiment.json"), "no-such-cell"); err == nil {
		t.Fatal("unknown cell must fail loudly")
	}
}

func TestBuildTiming(t *testing.T) {
	started := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	metadata := RunMetadata{StartedAt: started, FinishedAt: started.Add(10 * time.Minute)}
	zero := int64(0)
	events := []commandEvent{
		{Index: 0, Command: "go build ./...", Category: "build", ExitCode: nil, StartedAt: started.Add(2 * time.Minute).Format(time.RFC3339Nano)},
		{Index: 1, Command: "go build ./...", Category: "build", ExitCode: &zero, StartedAt: started.Add(3 * time.Minute).Format(time.RFC3339Nano)},
		{Index: 2, Command: "make check", Category: "test", ExitCode: &zero, StartedAt: started.Add(5 * time.Minute).Format(time.RFC3339Nano)},
	}
	timing := buildTiming(metadata, events)
	if timing.WallClockMilliseconds != 600000 {
		t.Fatalf("wall clock = %d", timing.WallClockMilliseconds)
	}
	if timing.FirstSuccessfulBuildMilliseconds == nil || *timing.FirstSuccessfulBuildMilliseconds != 180000 {
		t.Fatalf("first successful build = %v", timing.FirstSuccessfulBuildMilliseconds)
	}
	if timing.FirstVisibleBehaviorSuccessMilliseconds == nil || *timing.FirstVisibleBehaviorSuccessMilliseconds != 300000 {
		t.Fatalf("first visible behavior success = %v", timing.FirstVisibleBehaviorSuccessMilliseconds)
	}
}

func TestBuildTimingNoSuccesses(t *testing.T) {
	started := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	metadata := RunMetadata{StartedAt: started, FinishedAt: started.Add(time.Minute)}
	timing := buildTiming(metadata, nil)
	if timing.FirstSuccessfulBuildMilliseconds != nil || timing.FirstVisibleBehaviorSuccessMilliseconds != nil {
		t.Fatalf("optional timing fields must stay null: %+v", timing)
	}
}

func TestWithoutProviderCredentials(t *testing.T) {
	env := withoutProviderCredentials([]string{
		"PATH=/usr/bin",
		"OPENROUTER_API_KEY=sk-test-secret",
		"HOME=/home/evaluator",
	})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "OPENROUTER_API_KEY") || strings.Contains(joined, "sk-test-secret") {
		t.Fatalf("provider key survived scrubbing: %v", env)
	}
	if len(env) != 2 {
		t.Fatalf("unrelated variables were dropped: %v", env)
	}
}

func TestFailureInfrastructure(t *testing.T) {
	infra := failureInfrastructure(infraCategoryEvaluatorFailure, "postgres did not start", nil)
	if !infra.IsFailure || infra.Category == nil || *infra.Category != infraCategoryEvaluatorFailure {
		t.Fatalf("infra = %+v", infra)
	}
	if infra.ExclusionEligible || infra.Excluded || infra.ReplacementRunID != nil {
		t.Fatal("assembly never pre-decides exclusion")
	}
}
