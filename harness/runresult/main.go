// Command runresult assembles a complete run-result.json from a preserved run
// directory. It invokes the hidden evaluator for submitted and timed-out runs,
// merges evaluator cases into the nine common gates, records timing, usage,
// process, protocol, infrastructure, and artifact metadata, distinguishes
// candidate, harness, and external infrastructure failures, and validates the
// produced result against schemas/run-result.schema.json.
//
// Exit codes: 0 when a schema-valid run-result.json was produced (including
// candidate-failure results), 2 when no honest result could be assembled.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	artifactMetadata   = "metadata.json"
	artifactTranscript = "transcript.jsonl"
	artifactPatch      = "final.patch"
	artifactCommands   = "commands.json"
	artifactEvaluation = "evaluation.json"
	artifactManifest   = "workspace-manifest.json"
	artifactResult     = "run-result.json"

	defaultCodegenImage      = "specs-experiment-tool-codegen:go1.26.6"
	maxWallClockMilliseconds = 2700000
)

type options struct {
	runDir       string
	root         string
	evaluatorBin string
	codegenImage string
}

func main() {
	var opts options
	flag.StringVar(&opts.runDir, "run-dir", "", "run artifact directory containing driver-owned metadata.json, transcript.jsonl, and final.patch")
	flag.StringVar(&opts.root, "root", ".", "experiment repository root")
	flag.StringVar(&opts.evaluatorBin, "evaluator", "", "path to the evaluator binary (default <root>/bin/evaluator)")
	flag.StringVar(&opts.codegenImage, "codegen-image", defaultCodegenImage, "pinned codegen tool image used for Codegen health checks")
	flag.Parse()
	if opts.runDir == "" {
		fmt.Fprintln(os.Stderr, "-run-dir is required")
		os.Exit(2)
	}
	if err := assemble(opts); err != nil {
		fmt.Fprintf(os.Stderr, "runresult: %v\n", err)
		os.Exit(2)
	}
}

func assemble(opts options) error {
	runDir, err := filepath.Abs(opts.runDir)
	if err != nil {
		return fmt.Errorf("resolve run directory: %w", err)
	}
	root, err := filepath.Abs(opts.root)
	if err != nil {
		return fmt.Errorf("resolve experiment root: %w", err)
	}
	evaluatorBin := opts.evaluatorBin
	if evaluatorBin == "" {
		evaluatorBin = filepath.Join(root, "bin", "evaluator")
	}

	metadata, err := readMetadata(filepath.Join(runDir, artifactMetadata))
	if err != nil {
		return err
	}
	cell, err := resolveCell(filepath.Join(root, "config", "experiment.json"), metadata.CellID)
	if err != nil {
		return err
	}
	manifestIDs, err := readCaseManifest(filepath.Join(root, "evaluator", "case-manifest.json"))
	if err != nil {
		return err
	}
	for _, required := range []string{artifactTranscript, artifactPatch} {
		if info, err := os.Stat(filepath.Join(runDir, required)); err != nil || info.IsDir() {
			return fmt.Errorf("driver-owned artifact %s is missing in %s", required, runDir)
		}
	}
	workspace := metadata.Workspace
	if workspace == "" {
		workspace = "workspace"
	}
	workspaceDir := filepath.Join(runDir, workspace)

	result := RunResult{
		SchemaVersion:      1,
		RunID:              metadata.RunID,
		CellID:             metadata.CellID,
		RepeatIndex:        metadata.RepeatIndex,
		Phase:              metadata.Phase,
		Stage:              cell.Stage,
		Task:               cell.Task,
		Mode:               cell.Mode,
		Treatment:          cell.Treatment,
		Status:             metadata.Status,
		ProtocolViolations: metadata.ProtocolViolations,
		Infrastructure: Infrastructure{
			ReplacesRunID: metadata.ReplacesRunID,
		},
	}
	if result.ProtocolViolations == nil {
		result.ProtocolViolations = []ProtocolViolation{}
	}

	evaluationPath := filepath.Join(runDir, artifactEvaluation)
	switch metadata.Status {
	case statusSubmitted, statusTimedOut:
		if info, err := os.Stat(workspaceDir); err != nil || !info.IsDir() {
			return fmt.Errorf("candidate workspace %s is missing for status %s", workspaceDir, metadata.Status)
		}
		evaluation, raw, failure := evaluateCandidate(opts, evaluatorBin, root, workspaceDir, cell, manifestIDs)
		if err := os.WriteFile(evaluationPath, raw, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", artifactEvaluation, err)
		}
		if failure != nil {
			result.Status = failure.status
			result.Infrastructure = failureInfrastructure(failure.category, failure.evidence, metadata.ReplacesRunID)
		} else {
			result.Evaluation = evaluation
		}
	case statusInfraFail, statusHarnessFail:
		note := metadata.Infrastructure
		if note == nil || note.Category == "" || note.Evidence == "" {
			return fmt.Errorf("metadata for status %s requires infrastructure.category and infrastructure.evidence", metadata.Status)
		}
		result.Infrastructure = failureInfrastructure(note.Category, note.Evidence, metadata.ReplacesRunID)
		if err := os.WriteFile(evaluationPath, []byte("null\n"), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", artifactEvaluation, err)
		}
	default:
		return fmt.Errorf("unsupported metadata status %q", metadata.Status)
	}

	usage, events, process, err := parseTranscript(runDir, artifactTranscript, cell.Treatment)
	if err != nil {
		return fmt.Errorf("parse transcript: %w", err)
	}
	result.Usage = usage
	if err := writeCommands(runDir, artifactCommands, events); err != nil {
		return err
	}
	diff, err := classifyPatch(filepath.Join(runDir, artifactPatch), cell.Treatment)
	if err != nil {
		return fmt.Errorf("classify final patch: %w", err)
	}
	process.Diff = diff
	result.Process = process
	result.Timing = buildTiming(metadata, events)

	if err := writeWorkspaceManifest(workspaceDir, filepath.Join(runDir, artifactManifest)); err != nil {
		return fmt.Errorf("write workspace manifest: %w", err)
	}
	result.Artifacts = Artifacts{
		Metadata:          artifactMetadata,
		Transcript:        artifactTranscript,
		FinalPatch:        artifactPatch,
		Commands:          artifactCommands,
		Evaluation:        artifactEvaluation,
		WorkspaceManifest: artifactManifest,
	}

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run result: %w", err)
	}
	schemaPath := filepath.Join(root, "schemas", "run-result.schema.json")
	if err := validateRunResult(schemaPath, encoded); err != nil {
		return fmt.Errorf("produced run result violates run-result schema: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, artifactResult), append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", artifactResult, err)
	}
	fmt.Printf("run-result.json assembled for run %s (status %s)\n", result.RunID, result.Status)
	return nil
}

// failureNote describes a failure classification decided during assembly.
type failureNote struct {
	status   string
	category string
	evidence string
}

func failureInfrastructure(category, evidence string, replacesRunID *string) Infrastructure {
	return Infrastructure{
		IsFailure:         true,
		Category:          &category,
		Evidence:          &evidence,
		ExclusionEligible: false,
		Excluded:          false,
		ReplacesRunID:     replacesRunID,
	}
}

func readMetadata(path string) (RunMetadata, error) {
	var metadata RunMetadata
	data, err := os.ReadFile(path)
	if err != nil {
		return metadata, fmt.Errorf("read metadata: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return metadata, fmt.Errorf("parse metadata: %w", err)
	}
	if metadata.RunID == "" || metadata.CellID == "" {
		return metadata, errors.New("metadata requires runId and cellId")
	}
	if metadata.RepeatIndex < 1 || metadata.RepeatIndex > 5 {
		return metadata, fmt.Errorf("metadata repeatIndex %d is outside 1..5", metadata.RepeatIndex)
	}
	if metadata.Phase != "pilot" && metadata.Phase != "measured" {
		return metadata, fmt.Errorf("metadata phase %q is not pilot or measured", metadata.Phase)
	}
	if metadata.StartedAt.IsZero() || metadata.FinishedAt.IsZero() {
		return metadata, errors.New("metadata requires startedAt and finishedAt")
	}
	if metadata.FinishedAt.Before(metadata.StartedAt) {
		return metadata, errors.New("metadata finishedAt is before startedAt")
	}
	wall := metadata.FinishedAt.Sub(metadata.StartedAt).Milliseconds()
	if wall > maxWallClockMilliseconds {
		return metadata, fmt.Errorf("metadata wall clock %d ms exceeds the 45 minute limit; for timed-out runs finishedAt must be the enforced deadline", wall)
	}
	return metadata, nil
}

func resolveCell(path, cellID string) (Cell, error) {
	var config experimentConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return Cell{}, fmt.Errorf("read experiment config: %w", err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return Cell{}, fmt.Errorf("parse experiment config: %w", err)
	}
	for _, cell := range config.Cells {
		if cell.ID == cellID {
			return cell, nil
		}
	}
	return Cell{}, fmt.Errorf("cell %q is not defined in %s", cellID, path)
}

func readCaseManifest(path string) (map[string]bool, error) {
	var manifest caseManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read case manifest: %w", err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse case manifest: %w", err)
	}
	ids := make(map[string]bool, len(manifest.Cases))
	for _, entry := range manifest.Cases {
		if ids[entry.ID] {
			return nil, fmt.Errorf("case manifest contains duplicate id %q", entry.ID)
		}
		ids[entry.ID] = true
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("case manifest %s contains no cases", path)
	}
	return ids, nil
}

func buildTiming(metadata RunMetadata, events []commandEvent) Timing {
	timing := Timing{
		StartedAt:             metadata.StartedAt,
		FinishedAt:            metadata.FinishedAt,
		WallClockMilliseconds: metadata.FinishedAt.Sub(metadata.StartedAt).Milliseconds(),
	}
	for _, event := range events {
		if event.Category == "build" && event.ExitCode != nil && *event.ExitCode == 0 {
			if start, err := time.Parse(time.RFC3339Nano, event.StartedAt); err == nil {
				delta := start.Sub(metadata.StartedAt).Milliseconds()
				if delta >= 0 {
					timing.FirstSuccessfulBuildMilliseconds = &delta
				}
			}
			break
		}
	}
	for _, event := range events {
		if event.Category == "test" && event.ExitCode != nil && *event.ExitCode == 0 && containsMakeCheck(event.Command) {
			if start, err := time.Parse(time.RFC3339Nano, event.StartedAt); err == nil {
				delta := start.Sub(metadata.StartedAt).Milliseconds()
				if delta >= 0 {
					timing.FirstVisibleBehaviorSuccessMilliseconds = &delta
				}
			}
			break
		}
	}
	return timing
}
