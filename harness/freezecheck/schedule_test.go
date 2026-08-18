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

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestGenerateScheduleDeterministicAndBalanced(t *testing.T) {
	root := repositoryRoot(t)
	configPath := filepath.Join(root, "config", "experiment.json")
	one, err := generateSchedule(configPath, "measured", "test-seed", "abc123", "2026-08-18T12:34:56Z")
	if err != nil {
		t.Fatal(err)
	}
	two, err := generateSchedule(configPath, "measured", "test-seed", "abc123", "2026-08-18T12:34:56Z")
	if err != nil {
		t.Fatal(err)
	}
	if string(one) != string(two) {
		t.Fatal("same inputs produced different schedules")
	}
	sum := sha256.Sum256(one)
	const expected = "f4b21ec4a535ea36ec141b0ae7dca1be9342f015716284f7bafdde6100311785"
	if got := hex.EncodeToString(sum[:]); got != expected {
		t.Fatalf("deterministic schedule changed: got %s", got)
	}
	var config experimentConfig
	if _, err := readJSON(configPath, &config); err != nil {
		t.Fatal(err)
	}
	var document schedule
	if err := json.Unmarshal(one, &document); err != nil {
		t.Fatal(err)
	}
	if err := validateScheduleSemantic(config, document, "measured"); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleTamperingRejected(t *testing.T) {
	configPath := filepath.Join(repositoryRoot(t), "config", "experiment.json")
	data, err := generateSchedule(configPath, "pilot", "pilot-seed", "abc123", "2026-08-18T12:34:56Z")
	if err != nil {
		t.Fatal(err)
	}
	var config experimentConfig
	_, _ = readJSON(configPath, &config)
	var document schedule
	_ = json.Unmarshal(data, &document)
	document.Runs[1], document.Runs[2] = document.Runs[2], document.Runs[1]
	if err := validateScheduleSemantic(config, document, "pilot"); err == nil || !strings.Contains(err.Error(), "adjacent direct/codegen pair") {
		t.Fatalf("pair tampering was not rejected clearly: %v", err)
	}
	document.Runs[1], document.Runs[2] = document.Runs[2], document.Runs[1]
	document.Runs[0].RunID = document.Runs[1].RunID
	if err := validateScheduleSemantic(config, document, "pilot"); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate run ID was not rejected: %v", err)
	}
}

func TestGenerateRequiresExplicitInputsAndDoesNotOverwrite(t *testing.T) {
	configPath := filepath.Join(repositoryRoot(t), "config", "experiment.json")
	if _, err := generateSchedule(configPath, "pilot", "TODO_SEED", "abc", "2026-08-18T00:00:00Z"); err == nil {
		t.Fatal("TODO seed accepted")
	}
	data, err := generateSchedule(configPath, "pilot", "seed", "abc", "2026-08-18T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "schedule.json")
	if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeOutput(path, data); err == nil {
		t.Fatal("existing output was overwritten")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "keep" {
		t.Fatal("existing output content changed")
	}
}
