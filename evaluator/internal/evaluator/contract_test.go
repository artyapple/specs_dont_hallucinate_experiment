package evaluator

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCandidateEnvironmentRedactsProviderKey proves candidate commands never
// observe provider credentials even when the evaluator process has them.
func TestCandidateEnvironmentRedactsProviderKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-test-secret")
	t.Setenv("MARKER", "preserved")

	env := candidateEnvironment("DATABASE_URL=postgres://example")
	found := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		found[key] = value
	}
	if _, ok := found["OPENROUTER_API_KEY"]; ok {
		t.Fatal("candidate environment contains OPENROUTER_API_KEY")
	}
	if found["MARKER"] != "preserved" {
		t.Fatal("candidate environment lost unrelated variables")
	}
	if found["DATABASE_URL"] != "postgres://example" {
		t.Fatal("candidate environment lost injected variables")
	}

	output, err := runCommand(context.Background(), t.TempDir(), "postgres://unused", "sh", "-c", "printf '%s' \"$OPENROUTER_API_KEY\"")
	if err != nil {
		t.Fatal(err)
	}
	if output != "" {
		t.Fatalf("candidate command observed a provider key: %q", output)
	}
}

// TestEvaluateRosterRepresentationContract runs a full evaluation against a
// candidate whose build fails and asserts the frozen result representation:
// the complete roster in registry order, setup-failure evidence prefixes, and
// the null passed and empty evidence encoding for non-applicable cases.
func TestEvaluateRosterRepresentationContract(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker is required for the roster representation contract test")
	}
	candidate := t.TempDir()
	makefile := "build:\n\tfalse\nmigrate:\n\tfalse\nrun:\n\tfalse\n"
	if err := os.WriteFile(filepath.Join(candidate, "Makefile"), []byte(makefile), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result := Evaluate(ctx, Options{Candidate: candidate, Task: TaskNullable})

	if !result.Setup.Postgres {
		t.Fatal("PostgreSQL setup gate must be true; the candidate never runs before it")
	}
	if result.Setup.Build {
		t.Fatal("build gate must be false for the failing candidate")
	}
	if result.CompleteSuccess {
		t.Fatal("completeSuccess must be false")
	}

	definitions := caseDefinitions()
	if len(result.BehaviorCases) != len(definitions) {
		t.Fatalf("roster size = %d, want %d", len(result.BehaviorCases), len(definitions))
	}
	for index, definition := range definitions {
		got := result.BehaviorCases[index]
		if got.ID != definition.ID {
			t.Fatalf("roster order at %d = %s, want %s", index, got.ID, definition.ID)
		}
		applicable := definition.Task == taskAll || definition.Task == TaskNullable
		if got.Applicable != applicable {
			t.Fatalf("case %s applicable = %v, want %v", got.ID, got.Applicable, applicable)
		}
		if applicable {
			if got.Passed == nil || *got.Passed {
				t.Fatalf("applicable case %s passed = %v, want false", got.ID, got.Passed)
			}
			if !strings.HasPrefix(got.Evidence, "not run: ") {
				t.Fatalf("applicable case %s evidence = %q, want a not-run prefix", got.ID, got.Evidence)
			}
		} else {
			if got.Passed != nil {
				t.Fatalf("non-applicable case %s passed = %v, want null", got.ID, *got.Passed)
			}
			if got.Evidence != "" {
				t.Fatalf("non-applicable case %s evidence = %q, want empty", got.ID, got.Evidence)
			}
		}
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"passed":null`) {
		t.Fatal("non-applicable cases must encode passed as null")
	}
}

// TestSetupEvidenceIsBounded proves that pathological candidate command output
// cannot produce unbounded setup evidence.
func TestSetupEvidenceIsBounded(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker is required for the bounded-evidence contract test")
	}
	candidate := t.TempDir()
	makefile := "build:\n\t@yes this line repeats forever | head -n 200000; false\nmigrate:\n\ttrue\nrun:\n\ttrue\n"
	if err := os.WriteFile(filepath.Join(candidate, "Makefile"), []byte(makefile), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result := Evaluate(ctx, Options{Candidate: candidate, Task: TaskBaseline})

	if result.Setup.Build {
		t.Fatal("build gate must be false")
	}
	if len(result.Setup.Evidence) > evidenceLogLimit+1024 {
		t.Fatalf("setup evidence length = %d, want bounded by the evidence log limit", len(result.Setup.Evidence))
	}
	for _, behaviorCase := range result.BehaviorCases {
		if behaviorCase.Applicable && len(behaviorCase.Evidence) > evidenceLogLimit+1024 {
			t.Fatalf("case %s evidence length = %d, want bounded", behaviorCase.ID, len(behaviorCase.Evidence))
		}
	}
	if !strings.Contains(result.Setup.Evidence, "truncated") {
		t.Fatal("truncated evidence must be marked")
	}
}
