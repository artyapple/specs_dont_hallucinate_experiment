package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

var (
	binaryOnce sync.Once
	binaryPath string
	binaryErr  error
)

func evaluatorBinary(t *testing.T) string {
	t.Helper()
	binaryOnce.Do(func() {
		binaryPath = filepath.Join(os.TempDir(), "evaluator-contract-test")
		binaryErr = exec.Command("go", "build", "-o", binaryPath, ".").Run()
	})
	if binaryErr != nil {
		t.Fatalf("build evaluator binary: %v", binaryErr)
	}
	return binaryPath
}

func fakeCandidate(t *testing.T, makefile string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(makefile), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runEvaluator(t *testing.T, args ...string) (int, string) {
	t.Helper()
	command := exec.Command(evaluatorBinary(t), args...)
	var stderr strings.Builder
	command.Stderr = &stderr
	err := command.Run()
	if err == nil {
		return 0, stderr.String()
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("run evaluator: %v", err)
	}
	return exitErr.ExitCode(), stderr.String()
}

func TestCLIContract(t *testing.T) {
	candidate := fakeCandidate(t, "build:\n\ttrue\n")
	cases := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"missing candidate", []string{"-task", "baseline-service"}},
		{"missing task", []string{"-candidate", candidate}},
		{"invalid task", []string{"-candidate", candidate, "-task", "sideways"}},
		{"extra positional", []string{"-candidate", candidate, "-task", "baseline-service", "extra"}},
		{"candidate not a directory", []string{"-candidate", filepath.Join(candidate, "absent"), "-task", "baseline-service"}},
		{"output parent missing", []string{"-candidate", candidate, "-task", "baseline-service", "-output", filepath.Join(candidate, "absent-dir", "result.json")}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			code, _ := runEvaluator(t, testCase.args...)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
		})
	}
}

func TestSignalAbortWritesNoResult(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker is required for the abort contract test")
	}
	candidate := fakeCandidate(t, "build:\n\tsleep 600\nmigrate:\n\ttrue\nrun:\n\ttrue\n")
	output := filepath.Join(t.TempDir(), "result.json")
	command := exec.Command(evaluatorBinary(t), "-candidate", candidate, "-task", "baseline-service", "-output", output)
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	// PostgreSQL startup and the hanging candidate build both keep the process
	// alive; the abort path is identical in either phase.
	time.Sleep(15 * time.Second)
	started := time.Now()
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- command.Wait() }()
	select {
	case err := <-waitErr:
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("evaluator exit: %v", err)
		}
		if exitErr.ExitCode() != 2 {
			t.Fatalf("exit code = %d, want 2; stderr: %s", exitErr.ExitCode(), stderr.String())
		}
	case <-time.After(30 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("evaluator did not exit promptly after SIGTERM")
	}
	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Fatalf("cleanup took %s, want prompt exit", elapsed)
	}
	if !strings.Contains(stderr.String(), "aborted") {
		t.Fatalf("stderr = %q, want an abort diagnostic", stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("an aborted run must not write a result file")
	}
	entries, err := os.ReadDir(filepath.Dir(output))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".evaluator-result-") {
			t.Fatalf("temporary result file leaked: %s", entry.Name())
		}
	}
}

func dockerAvailable() bool {
	return exec.Command("docker", "info").Run() == nil
}
