package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResultsValidReciprocalReplacement(t *testing.T) {
	root := repositoryRoot(t)
	resultsDir := t.TempDir()
	original, replacement := replacementPair(t, root)
	writeRunFixture(t, resultsDir, "01-original", original)
	writeRunFixture(t, resultsDir, "02-replacement", replacement)
	if err := validateResults(root, resultsDir, ""); err != nil {
		t.Fatalf("valid reciprocal replacement rejected: %v", err)
	}
}

func TestResultsReplacementLinkTampering(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]any, map[string]any)
		want    string
		omitNew bool
	}{
		{
			name:    "missing replacement",
			mutate:  func(_, _ map[string]any) {},
			want:    "does not resolve uniquely",
			omitNew: true,
		},
		{
			name: "wrong reciprocal",
			mutate: func(_, replacement map[string]any) {
				replacement["infrastructure"].(map[string]any)["replacesRunId"] = "another-run"
			},
			want: "does not point back",
		},
		{
			name: "wrong cell",
			mutate: func(_, replacement map[string]any) {
				replacement["cellId"] = "greenfield-codegen"
			},
			want: "points to cell",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := repositoryRoot(t)
			resultsDir := t.TempDir()
			original, replacement := replacementPair(t, root)
			test.mutate(original, replacement)
			writeRunFixture(t, resultsDir, "01-original", original)
			if !test.omitNew {
				writeRunFixture(t, resultsDir, "02-replacement", replacement)
			}
			err := validateResults(root, resultsDir, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("tampering was not rejected with %q: %v", test.want, err)
			}
		})
	}
}

func TestResultsRejectsDuplicateIDsAndEmptySet(t *testing.T) {
	root := repositoryRoot(t)
	if err := validateResults(root, t.TempDir(), ""); err == nil || !strings.Contains(err.Error(), "no immediate child") {
		t.Fatalf("empty result set was not rejected: %v", err)
	}
	resultsDir := t.TempDir()
	first := validRunResult(t, root)
	second := validRunResult(t, root)
	writeRunFixture(t, resultsDir, "a", first)
	writeRunFixture(t, resultsDir, "b", second)
	if err := validateResults(root, resultsDir, ""); err == nil || !strings.Contains(err.Error(), "duplicate runId") {
		t.Fatalf("duplicate run IDs were not rejected: %v", err)
	}
}

func TestResultsCLIRequiresFlags(t *testing.T) {
	if err := runCLI([]string{"results", "--root", repositoryRoot(t)}); err == nil || !strings.Contains(err.Error(), "--results-dir") {
		t.Fatalf("missing results-dir was not rejected: %v", err)
	}
}

func replacementPair(t *testing.T, root string) (map[string]any, map[string]any) {
	t.Helper()
	original := validRunResult(t, root)
	original["runId"] = "original-run"
	original["status"] = "infrastructure-failure"
	original["evaluation"] = nil
	original["infrastructure"] = map[string]any{
		"isFailure": true, "category": "model-provider-outage", "evidence": "independently confirmed",
		"exclusionEligible": true, "excluded": true, "replacementRunId": "replacement-run", "replacesRunId": nil,
	}
	replacement := validRunResult(t, root)
	replacement["runId"] = "replacement-run"
	replacement["infrastructure"].(map[string]any)["replacesRunId"] = "original-run"
	return original, replacement
}

func writeRunFixture(t *testing.T, resultsDir, name string, result map[string]any) {
	t.Helper()
	runDir := filepath.Join(resultsDir, name)
	workspace := filepath.Join(runDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("candidate\n")
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []string{"transcript.jsonl", "final.patch", "commands.json", "evaluation.json"} {
		if err := os.WriteFile(filepath.Join(runDir, artifact), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(runDir, "metadata.json"), []byte(`{"workspace":"workspace"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	manifest := map[string]any{"schemaVersion": 1, "files": []any{map[string]any{
		"path": "main.go", "sha256": hex.EncodeToString(sum[:]), "sizeBytes": len(content),
	}}}
	writeJSONTest(t, filepath.Join(runDir, "workspace-manifest.json"), manifest)
	writeJSONTest(t, filepath.Join(runDir, "run-result.json"), result)
}
