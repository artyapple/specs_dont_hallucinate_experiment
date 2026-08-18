package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidationAndTampering(t *testing.T) {
	root := repositoryRoot(t)
	runDir := t.TempDir()
	workspace := filepath.Join(runDir, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("candidate\n")
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"transcript.jsonl", "final.patch", "commands.json", "evaluation.json"} {
		if err := os.WriteFile(filepath.Join(runDir, name), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	metadata := []byte(`{"workspace":"workspace"}`)
	if err := os.WriteFile(filepath.Join(runDir, "metadata.json"), metadata, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	manifest := map[string]any{"schemaVersion": 1, "files": []any{map[string]any{"path": "main.go", "sha256": hex.EncodeToString(sum[:]), "sizeBytes": len(content)}}}
	writeJSONTest(t, filepath.Join(runDir, "workspace-manifest.json"), manifest)
	result := validRunResult(t, root)
	writeJSONTest(t, filepath.Join(runDir, "run-result.json"), result)
	if err := validateRun(root, runDir, ""); err != nil {
		t.Fatalf("valid run rejected: %v", err)
	}

	manifest["files"].([]any)[0].(map[string]any)["sha256"] = strings.Repeat("0", 64)
	writeJSONTest(t, filepath.Join(runDir, "workspace-manifest.json"), manifest)
	if err := validateRun(root, runDir, ""); err == nil || !strings.Contains(err.Error(), "sha256 does not match") {
		t.Fatalf("manifest tampering was not rejected: %v", err)
	}
	manifest["files"].([]any)[0].(map[string]any)["sha256"] = hex.EncodeToString(sum[:])
	writeJSONTest(t, filepath.Join(runDir, "workspace-manifest.json"), manifest)
	result["evaluation"].(map[string]any)["completeSuccess"] = false
	writeJSONTest(t, filepath.Join(runDir, "run-result.json"), result)
	if err := validateRun(root, runDir, ""); err == nil || !strings.Contains(err.Error(), "recomputed value") {
		t.Fatalf("completeSuccess tampering was not rejected: %v", err)
	}
}

func TestRunRejectsArtifactEscapeAndReplacementLink(t *testing.T) {
	root := repositoryRoot(t)
	runDir := t.TempDir()
	result := validRunResult(t, root)
	result["artifacts"].(map[string]any)["metadata"] = "../outside"
	result["infrastructure"].(map[string]any)["replacesRunId"] = "old-run"
	writeJSONTest(t, filepath.Join(runDir, "run-result.json"), result)
	err := validateRun(root, runDir, "")
	if err == nil || !strings.Contains(err.Error(), "lexically escapes") || !strings.Contains(err.Error(), "freezecheck results") {
		t.Fatalf("escape/replacement was not rejected clearly: %v", err)
	}
}

func validRunResult(t *testing.T, root string) map[string]any {
	t.Helper()
	var manifest caseManifest
	if _, err := readJSON(filepath.Join(root, "evaluator", "case-manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	cases := make([]any, 0, len(manifest.Cases))
	for _, item := range manifest.Cases {
		if item.Task == "all" {
			cases = append(cases, map[string]any{"id": item.ID, "applicable": true, "passed": true, "evidence": "verified"})
		} else {
			cases = append(cases, map[string]any{"id": item.ID, "applicable": false, "passed": nil, "evidence": nil})
		}
	}
	gates := map[string]any{}
	for _, name := range []string{"formal-inputs", "build", "migrations", "service-start", "baseline-behavior", "task-behavior", "regressions", "api-conformance", "database-consistency"} {
		gates[name] = true
	}
	return map[string]any{
		"schemaVersion": 1, "runId": "pilot-001-greenfield-direct-r1", "cellId": "greenfield-direct", "repeatIndex": 1,
		"phase": "pilot", "stage": "greenfield", "task": "baseline-service", "mode": "greenfield", "treatment": "direct", "status": "submitted",
		"protocolViolations": []any{},
		"infrastructure":     map[string]any{"isFailure": false, "category": nil, "evidence": nil, "exclusionEligible": false, "excluded": false, "replacementRunId": nil, "replacesRunId": nil},
		"timing":             map[string]any{"startedAt": "2026-08-18T00:00:00Z", "finishedAt": "2026-08-18T00:00:01Z", "wallClockMilliseconds": 1000, "firstSuccessfulBuildMilliseconds": nil, "firstVisibleBehaviorSuccessMilliseconds": nil},
		"usage":              map[string]any{"inputTokens": nil, "outputTokens": nil, "turns": 1, "toolCalls": 1},
		"process":            map[string]any{"repairIterations": 0, "filesTouched": 1, "diff": map[string]any{"contractLines": 0, "handwrittenLines": 1, "generatedLines": 0}, "compilerEvents": []any{}},
		"artifacts":          map[string]any{"metadata": "metadata.json", "transcript": "transcript.jsonl", "finalPatch": "final.patch", "commands": "commands.json", "evaluation": "evaluation.json", "workspaceManifest": "workspace-manifest.json"},
		"evaluation":         map[string]any{"completeSuccess": true, "commonGates": gates, "behaviorCases": cases, "codegenHealth": nil, "candidateTests": map[string]any{"present": false, "testFiles": 0}, "residualFailures": []any{}},
	}
}

func writeJSONTest(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
