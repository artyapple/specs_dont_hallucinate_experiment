package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupOK() evaluatorSetup {
	return evaluatorSetup{Postgres: true, Build: true, Migrations: true, Ready: true}
}

func passCase(id string, applicable bool) evaluatorCase {
	passed := true
	c := evaluatorCase{ID: id, Applicable: applicable}
	if applicable {
		c.Passed = &passed
	}
	return c
}

func failCase(id string) evaluatorCase {
	passed := false
	return evaluatorCase{ID: id, Applicable: true, Passed: &passed, Evidence: "boom"}
}

func allBaselineAndContract(pass bool) []evaluatorCase {
	ids := append(append([]string{}, baselineCaseIDs...),
		"contract.openapi-conformance", "contract.problem-details", "contract.database-consistency")
	cases := make([]evaluatorCase, 0, len(ids))
	for _, id := range ids {
		if pass {
			cases = append(cases, passCase(id, true))
		} else {
			cases = append(cases, failCase(id))
		}
	}
	return cases
}

func TestDeriveGatesAllPassBaselineService(t *testing.T) {
	standalone := evaluatorResult{Setup: setupOK(), BehaviorCases: allBaselineAndContract(true)}
	gates := deriveGates(standalone, true, TaskBaselineService)
	if !gates.FormalInputs || !gates.Build || !gates.Migrations || !gates.ServiceStart ||
		!gates.BaselineBehavior || !gates.TaskBehavior || !gates.Regressions ||
		!gates.APIConformance || !gates.DatabaseConsistency {
		t.Fatalf("all gates must be true: %+v", gates)
	}
}

func TestDeriveGatesTaskBehaviorMirrorsBaselineForBaselineService(t *testing.T) {
	standalone := evaluatorResult{Setup: setupOK(), BehaviorCases: allBaselineAndContract(false)}
	gates := deriveGates(standalone, true, TaskBaselineService)
	if gates.TaskBehavior {
		t.Fatal("task-behavior must mirror baseline-behavior for baseline-service")
	}
	if gates.BaselineBehavior {
		t.Fatal("baseline-behavior must be false when a baseline case fails")
	}
}

func TestDeriveGatesTaskSpecificFailure(t *testing.T) {
	cases := allBaselineAndContract(true)
	cases = append(cases, passCase("nullable.omitted-preserves", true), failCase("nullable.null-clears"))
	standalone := evaluatorResult{Setup: setupOK(), BehaviorCases: cases}
	gates := deriveGates(standalone, true, "nullable-patch")
	if gates.TaskBehavior {
		t.Fatal("task-behavior must fail when a nullable case fails")
	}
	if !gates.Regressions || !gates.BaselineBehavior {
		t.Fatal("regressions and baseline-behavior must stay true when common cases pass")
	}
}

func TestDeriveGatesContractMapping(t *testing.T) {
	cases := allBaselineAndContract(true)
	for i := range cases {
		if cases[i].ID == "contract.problem-details" {
			cases[i] = failCase(cases[i].ID)
		}
	}
	standalone := evaluatorResult{Setup: setupOK(), BehaviorCases: cases}
	gates := deriveGates(standalone, true, TaskBaselineService)
	if gates.APIConformance {
		t.Fatal("api-conformance must include contract.problem-details")
	}
	if gates.Regressions {
		t.Fatal("regressions must include contract cases")
	}
	if !gates.DatabaseConsistency {
		t.Fatal("database-consistency must stay true when its case passes")
	}
}

func TestDeriveGatesSetupFailure(t *testing.T) {
	setup := setupOK()
	setup.Build = false
	standalone := evaluatorResult{Setup: setup, BehaviorCases: allBaselineAndContract(true)}
	gates := deriveGates(standalone, true, TaskBaselineService)
	if gates.Build {
		t.Fatal("build gate must follow setup.build")
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckFormalInputsPropagation(t *testing.T) {
	root := filepath.Join("..", "..")
	canonical := filepath.Join(root, "fixtures", "task-solutions", "nullable-patch-direct")
	propagation := filepath.Join(root, "tasks", "propagation")
	cell := Cell{ID: "nullable-patch-propagation-direct", Stage: "existing-service", Task: "nullable-patch", Mode: "propagation-only", Treatment: "direct"}

	workspace := t.TempDir()
	for _, rel := range []string{"api/openapi.yaml", "db/migrations/000002_add_task_due_at.sql", "db/queries/tasks.sql"} {
		copyFile(t, filepath.Join(canonical, rel), filepath.Join(workspace, rel))
	}
	ok, evidence := checkFormalInputs(workspace, cell, propagation)
	if !ok {
		t.Fatalf("canonical propagation target must pass: %s", evidence)
	}

	mutated := t.TempDir()
	for _, rel := range []string{"api/openapi.yaml", "db/migrations/000002_add_task_due_at.sql", "db/queries/tasks.sql"} {
		copyFile(t, filepath.Join(canonical, rel), filepath.Join(mutated, rel))
	}
	if err := os.WriteFile(filepath.Join(mutated, "db", "queries", "tasks.sql"), []byte("-- redesigned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, evidence = checkFormalInputs(mutated, cell, propagation)
	if ok {
		t.Fatal("a redesigned formal target must fail the gate")
	}
	if evidence == "" {
		t.Fatal("a redesigned formal target must produce evidence")
	}

	missing := t.TempDir()
	ok, _ = checkFormalInputs(missing, cell, propagation)
	if ok {
		t.Fatal("missing formal targets must fail the gate")
	}
}

func TestCheckFormalInputsFullWorkflow(t *testing.T) {
	propagation := filepath.Join("..", "..", "tasks", "propagation")
	cell := Cell{ID: "nullable-patch-full-direct", Stage: "existing-service", Task: "nullable-patch", Mode: "full-workflow", Treatment: "direct"}

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "db", "queries"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "db", "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	openapi := "openapi: 3.1.0\ninfo:\n  title: t\n  version: v\npaths:\n  /tasks: {}\n"
	if err := os.WriteFile(filepath.Join(workspace, "api", "openapi.yaml"), []byte(openapi), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "db", "queries", "tasks.sql"), []byte("-- name: ListTasks :many\nSELECT 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "db", "migrations", "000001_create_tasks.sql"), []byte("SELECT 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var manifest strings.Builder
	for _, rel := range []string{"api/openapi.yaml", "db/migrations/000001_create_tasks.sql", "db/queries/tasks.sql"} {
		data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		manifest.WriteString(hex.EncodeToString(sum[:]) + "  " + rel + "\n")
	}
	if err := os.WriteFile(filepath.Join(workspace, "formal.sha256"), []byte(manifest.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, evidence := checkFormalInputs(workspace, cell, propagation)
	if !ok {
		t.Fatalf("minimal valid formal inputs must pass: %s", evidence)
	}
	if err := os.WriteFile(filepath.Join(workspace, "db", "queries", "tasks.sql"), []byte("-- stale manifest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := checkFormalInputs(workspace, cell, propagation); ok {
		t.Fatal("stale formal.sha256 must fail the gate")
	}
	if err := os.WriteFile(filepath.Join(workspace, "db", "queries", "tasks.sql"), []byte("-- name: ListTasks :many\nSELECT 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	secondMigration := filepath.Join(workspace, "db", "migrations", "000002_new.sql")
	if err := os.WriteFile(secondMigration, []byte("SELECT 2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := checkFormalInputs(workspace, cell, propagation); ok {
		t.Fatal("a migration omitted from formal.sha256 must fail the gate")
	}
	if err := os.Remove(secondMigration); err != nil {
		t.Fatal(err)
	}
	unsafeManifest := strings.Replace(manifest.String(), "api/openapi.yaml", "../openapi.yaml", 1)
	if err := os.WriteFile(filepath.Join(workspace, "formal.sha256"), []byte(unsafeManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := checkFormalInputs(workspace, cell, propagation); ok {
		t.Fatal("an unsafe formal.sha256 path must fail the gate")
	}
	if err := os.WriteFile(filepath.Join(workspace, "formal.sha256"), []byte(manifest.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(workspace, "api", "openapi.yaml"), []byte("openapi: [broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, _ = checkFormalInputs(workspace, cell, propagation)
	if ok {
		t.Fatal("unparseable OpenAPI must fail the gate")
	}
}

func TestCountCandidateTests(t *testing.T) {
	workspace := t.TempDir()
	tests := countCandidateTests(workspace)
	if tests.Present || tests.TestFiles != 0 {
		t.Fatalf("empty workspace: %+v", tests)
	}
	if err := os.WriteFile(filepath.Join(workspace, "main_test.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "svc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "svc", "svc_test.go"), []byte("package svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "helper.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests = countCandidateTests(workspace)
	if !tests.Present || tests.TestFiles != 2 {
		t.Fatalf("workspace with two test files: %+v", tests)
	}
}
