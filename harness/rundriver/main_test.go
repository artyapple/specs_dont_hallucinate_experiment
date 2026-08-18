package main

import (
	"os"
	"path/filepath"
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
