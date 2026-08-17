package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func schemaPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "schemas", "run-result.schema.json")
}

func baseResult() RunResult {
	started := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	return RunResult{
		SchemaVersion:      1,
		RunID:              "run-1",
		CellID:             "greenfield-direct",
		RepeatIndex:        1,
		Phase:              "pilot",
		Stage:              "greenfield",
		Task:               "baseline-service",
		Mode:               "greenfield",
		Treatment:          "direct",
		Status:             statusSubmitted,
		ProtocolViolations: []ProtocolViolation{},
		Infrastructure:     Infrastructure{},
		Timing: Timing{
			StartedAt:             started,
			FinishedAt:            started.Add(10 * time.Minute),
			WallClockMilliseconds: 600000,
		},
		Usage: Usage{Turns: 3, ToolCalls: 5},
		Process: Process{
			Diff:           DiffLines{ContractLines: 2, HandwrittenLines: 40},
			CompilerEvents: []CompilerEvent{},
		},
		Artifacts: Artifacts{
			Metadata:          "metadata.json",
			Transcript:        "transcript.jsonl",
			FinalPatch:        "final.patch",
			Commands:          "commands.json",
			Evaluation:        "evaluation.json",
			WorkspaceManifest: "workspace-manifest.json",
		},
	}
}

func passingEvaluation() *Evaluation {
	passed := true
	evidence := "passed"
	return &Evaluation{
		CompleteSuccess: true,
		CommonGates: CommonGates{
			FormalInputs: true, Build: true, Migrations: true, ServiceStart: true,
			BaselineBehavior: true, TaskBehavior: true, Regressions: true,
			APIConformance: true, DatabaseConsistency: true,
		},
		BehaviorCases: []BehaviorCase{
			{ID: "baseline.create-valid", Applicable: true, Passed: &passed, Evidence: &evidence},
			{ID: "nullable.omitted-preserves", Applicable: false},
		},
		CandidateTests:   CandidateTests{Present: true, TestFiles: 2},
		ResidualFailures: []string{},
	}
}

func marshal(t *testing.T, result RunResult) []byte {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestSchemaAcceptsSubmittedDirect(t *testing.T) {
	result := baseResult()
	result.Evaluation = passingEvaluation()
	if err := validateRunResult(schemaPath(t), marshal(t, result)); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaAcceptsSubmittedCodegen(t *testing.T) {
	result := baseResult()
	result.Treatment = "codegen"
	result.CellID = "greenfield-codegen"
	evaluation := passingEvaluation()
	evaluation.CodegenHealth = &CodegenHealth{GenerationSucceeded: true, Canonical: true, Idempotent: true}
	result.Evaluation = evaluation
	if err := validateRunResult(schemaPath(t), marshal(t, result)); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaAcceptsTimedOut(t *testing.T) {
	result := baseResult()
	result.Status = statusTimedOut
	result.Evaluation = passingEvaluation()
	if err := validateRunResult(schemaPath(t), marshal(t, result)); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaAcceptsFailureStatuses(t *testing.T) {
	for _, status := range []string{statusInfraFail, statusHarnessFail} {
		result := baseResult()
		result.Status = status
		category := infraCategoryEvaluatorFailure
		evidence := "postgres did not start"
		result.Infrastructure = Infrastructure{IsFailure: true, Category: &category, Evidence: &evidence}
		if err := validateRunResult(schemaPath(t), marshal(t, result)); err != nil {
			t.Fatalf("%s: %v", status, err)
		}
	}
}

func TestSchemaRejectsCompleteSuccessWithFailedCase(t *testing.T) {
	result := baseResult()
	evaluation := passingEvaluation()
	failed := false
	evidence := "boom"
	evaluation.BehaviorCases[0].Passed = &failed
	evaluation.BehaviorCases[0].Evidence = &evidence
	result.Evaluation = evaluation
	if err := validateRunResult(schemaPath(t), marshal(t, result)); err == nil {
		t.Fatal("completeSuccess true with a failed applicable case must be rejected")
	}
}

func TestSchemaRejectsCodegenHealthOnDirect(t *testing.T) {
	result := baseResult()
	evaluation := passingEvaluation()
	evaluation.CodegenHealth = &CodegenHealth{GenerationSucceeded: true}
	result.Evaluation = evaluation
	if err := validateRunResult(schemaPath(t), marshal(t, result)); err == nil {
		t.Fatal("direct treatment with codegenHealth must be rejected")
	}
}

func TestSchemaRejectsEvaluationOnFailureStatus(t *testing.T) {
	result := baseResult()
	result.Status = statusInfraFail
	category := infraCategoryEvaluatorFailure
	evidence := "postgres did not start"
	result.Infrastructure = Infrastructure{IsFailure: true, Category: &category, Evidence: &evidence}
	result.Evaluation = passingEvaluation()
	if err := validateRunResult(schemaPath(t), marshal(t, result)); err == nil {
		t.Fatal("infrastructure-failure with a non-null evaluation must be rejected")
	}
}

func TestSchemaRejectsInconsistentCompleteSuccess(t *testing.T) {
	result := baseResult()
	evaluation := passingEvaluation()
	evaluation.CommonGates.Build = false
	result.Evaluation = evaluation
	err := validateRunResult(schemaPath(t), marshal(t, result))
	if err == nil || !strings.Contains(err.Error(), "schema validation") {
		t.Fatalf("completeSuccess true with a false gate must be rejected, got %v", err)
	}
}
