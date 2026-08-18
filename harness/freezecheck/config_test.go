package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDraftRepositoryConfigAccepted(t *testing.T) {
	if err := validateConfig(repositoryRoot(t)); err != nil {
		t.Fatalf("canonical draft config rejected: %v", err)
	}
}

func TestFrozenTODORejected(t *testing.T) {
	source := repositoryRoot(t)
	root := t.TempDir()
	for _, dir := range []string{"config", "schemas"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"schemas/experiment-config.schema.json", "schemas/schedule.schema.json", "config/versions.json", "config/schedule.json"} {
		copyTestFile(t, filepath.Join(source, name), filepath.Join(root, name))
	}
	data, err := os.ReadFile(filepath.Join(source, "config", "experiment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	document["status"] = "frozen"
	encoded, _ := json.Marshal(document)
	if err := os.WriteFile(filepath.Join(root, "config", "experiment.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	err = validateConfig(root)
	if err == nil || !strings.Contains(err.Error(), "TODO") {
		t.Fatalf("frozen TODOs were not rejected: %v", err)
	}
}

func copyTestFile(t *testing.T, source, target string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
