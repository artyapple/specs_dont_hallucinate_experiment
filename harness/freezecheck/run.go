package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type workspaceManifest struct {
	SchemaVersion int `json:"schemaVersion"`
	Files         []struct {
		Path      string `json:"path"`
		SHA256    string `json:"sha256"`
		SizeBytes int64  `json:"sizeBytes"`
	} `json:"files"`
}

type runMetadata struct {
	Workspace string `json:"workspace"`
}

func validateRun(root, runDir, schedulePath string) error {
	_, err := validateRunDocument(root, runDir, schedulePath, false)
	return err
}

func validateRunDocument(root, runDir, schedulePath string, allowReplacementLinks bool) (runResult, error) {
	var result runResult
	root, err := filepath.Abs(root)
	if err != nil {
		return result, err
	}
	runDir, err = filepath.Abs(runDir)
	if err != nil {
		return result, err
	}
	resolvedRun, err := filepath.EvalSymlinks(runDir)
	if err != nil {
		return result, fmt.Errorf("resolve run directory: %w", err)
	}
	data, err := readJSON(filepath.Join(runDir, "run-result.json"), &result)
	if err != nil {
		return result, err
	}
	if err := validateSchema(filepath.Join(root, "schemas", "run-result.schema.json"), data, false); err != nil {
		return result, fmt.Errorf("run result: %w", err)
	}
	var config experimentConfig
	if _, err := readJSON(filepath.Join(root, "config", "experiment.json"), &config); err != nil {
		return result, err
	}
	errs := []string{}
	cells := map[string]cell{}
	for _, c := range config.Cells {
		cells[c.ID] = c
	}
	c, ok := cells[result.CellID]
	if !ok {
		errs = append(errs, fmt.Sprintf("cellId %q is not in experiment matrix", result.CellID))
	} else {
		for label, pair := range map[string][2]string{
			"stage": {result.Stage, c.Stage}, "task": {result.Task, c.Task}, "mode": {result.Mode, c.Mode}, "treatment": {result.Treatment, c.Treatment},
		} {
			if pair[0] != pair[1] {
				errs = append(errs, fmt.Sprintf("%s %q does not match matrix value %q", label, pair[0], pair[1]))
			}
		}
	}
	if schedulePath != "" {
		var document schedule
		scheduleData, err := readJSON(schedulePath, &document)
		if err != nil {
			errs = append(errs, "schedule: "+err.Error())
		} else if err := validateSchema(filepath.Join(root, "schemas", "schedule.schema.json"), scheduleData, false); err != nil {
			errs = append(errs, "schedule: "+err.Error())
		} else {
			matches := []scheduleRun{}
			for _, run := range document.Runs {
				if run.RunID == result.RunID {
					matches = append(matches, run)
				}
			}
			if len(matches) != 1 {
				errs = append(errs, fmt.Sprintf("runId %q must occur exactly once in schedule, got %d", result.RunID, len(matches)))
			} else if matches[0].CellID != result.CellID || matches[0].RepeatIndex != result.RepeatIndex {
				errs = append(errs, "runId schedule entry does not match result cellId and repeatIndex")
			}
		}
	}
	artifactValues := map[string]string{
		"metadata": result.Artifacts.Metadata, "transcript": result.Artifacts.Transcript, "finalPatch": result.Artifacts.FinalPatch,
		"commands": result.Artifacts.Commands, "evaluation": result.Artifacts.Evaluation, "workspaceManifest": result.Artifacts.WorkspaceManifest,
	}
	artifactPaths := map[string]string{}
	for label, value := range artifactValues {
		path, pathErr := secureExistingPath(runDir, resolvedRun, value)
		if pathErr != nil {
			errs = append(errs, fmt.Sprintf("artifact %s: %v", label, pathErr))
		} else {
			artifactPaths[label] = path
		}
	}
	if path := artifactPaths["workspaceManifest"]; path != "" {
		errs = append(errs, verifyManifest(runDir, resolvedRun, artifactPaths["metadata"], path)...)
	}
	var manifest caseManifest
	if _, err := readJSON(filepath.Join(root, "evaluator", "case-manifest.json"), &manifest); err != nil {
		errs = append(errs, "case manifest: "+err.Error())
	} else {
		errs = append(errs, validateEvaluation(result, manifest)...)
	}
	if !allowReplacementLinks && (result.Infrastructure.ReplacementRunID != nil || result.Infrastructure.ReplacesRunID != nil) {
		errs = append(errs, "replacement links cannot be verified in one-run mode; use freezecheck results --root <repo> --results-dir <dir>")
	}
	return result, diagnostics(errs)
}

func secureExistingPath(runDir, resolvedRun, value string) (string, error) {
	if filepath.IsAbs(value) {
		return "", fmt.Errorf("path %q must be relative", value)
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q lexically escapes run directory", value)
	}
	path := filepath.Join(runDir, clean)
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("path %q does not exist or cannot be resolved: %w", value, err)
	}
	if !pathWithin(resolvedRun, resolved) {
		return "", fmt.Errorf("path %q resolves outside run directory", value)
	}
	return path, nil
}

func pathWithin(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func verifyManifest(runDir, resolvedRun, metadataPath, manifestPath string) []string {
	errs := []string{}
	var manifest workspaceManifest
	if _, err := readJSON(manifestPath, &manifest); err != nil {
		return []string{"workspace manifest: " + err.Error()}
	}
	if manifest.SchemaVersion != 1 {
		errs = append(errs, fmt.Sprintf("workspace manifest schemaVersion must be 1, got %d", manifest.SchemaVersion))
	}
	workspace := "workspace"
	if metadataPath != "" {
		var metadata runMetadata
		if _, err := readJSON(metadataPath, &metadata); err != nil {
			errs = append(errs, "metadata: "+err.Error())
		} else if metadata.Workspace != "" {
			workspace = metadata.Workspace
		}
	}
	workspacePath, err := secureExistingPath(runDir, resolvedRun, workspace)
	if err != nil {
		return append(errs, "workspace: "+err.Error())
	}
	info, err := os.Stat(workspacePath)
	if err != nil || !info.IsDir() {
		return append(errs, fmt.Sprintf("workspace path %q is not a directory", workspace))
	}
	expected := map[string]bool{}
	_ = filepath.WalkDir(workspacePath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			errs = append(errs, fmt.Sprintf("walk workspace: %v", walkErr))
			return nil
		}
		if entry.IsDir() {
			if path != workspacePath && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			rel, relErr := filepath.Rel(workspacePath, path)
			if relErr != nil {
				errs = append(errs, fmt.Sprintf("workspace relative path: %v", relErr))
			} else {
				expected[filepath.ToSlash(rel)] = true
			}
		}
		return nil
	})
	seen := map[string]bool{}
	for index, entry := range manifest.Files {
		label := fmt.Sprintf("workspace manifest files[%d]", index)
		if seen[entry.Path] {
			errs = append(errs, fmt.Sprintf("%s path %q is duplicated", label, entry.Path))
			continue
		}
		seen[entry.Path] = true
		if index > 0 && manifest.Files[index-1].Path >= entry.Path {
			errs = append(errs, fmt.Sprintf("%s path is not in strictly sorted order", label))
		}
		if !expected[entry.Path] {
			errs = append(errs, fmt.Sprintf("%s path %q is not a regular workspace file", label, entry.Path))
		}
		path, err := secureExistingPath(workspacePath, mustEval(workspacePath), filepath.FromSlash(entry.Path))
		if err != nil {
			errs = append(errs, label+": "+err.Error())
			continue
		}
		sum, size, err := hashFile(path)
		if err != nil {
			errs = append(errs, label+": "+err.Error())
			continue
		}
		if entry.SizeBytes != size {
			errs = append(errs, fmt.Sprintf("%s sizeBytes is %d, actual %d", label, entry.SizeBytes, size))
		}
		if !strings.EqualFold(entry.SHA256, sum) {
			errs = append(errs, fmt.Sprintf("%s sha256 does not match file %q", label, entry.Path))
		}
	}
	for path := range expected {
		if !seen[path] {
			errs = append(errs, fmt.Sprintf("workspace manifest is missing regular file %q", path))
		}
	}
	return errs
}

func mustEval(path string) string {
	resolved, _ := filepath.EvalSymlinks(path)
	return resolved
}

func hashFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("not a regular file")
	}
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

func validateEvaluation(result runResult, manifest caseManifest) []string {
	errs := []string{}
	failureStatus := result.Status == "infrastructure-failure" || result.Status == "harness-failure"
	if failureStatus {
		if result.Evaluation != nil {
			errs = append(errs, "failure status requires evaluation null")
		}
		if !result.Infrastructure.IsFailure || result.Infrastructure.Category == nil || strings.TrimSpace(valueOrEmpty(result.Infrastructure.Category)) == "" || result.Infrastructure.Evidence == nil || strings.TrimSpace(valueOrEmpty(result.Infrastructure.Evidence)) == "" {
			errs = append(errs, "failure status requires infrastructure isFailure true with category and evidence")
		}
		allowed := map[string]bool{
			"model-provider-outage": true, "harness-process-crash": true, "host-container-runtime-failure": true,
			"evaluator-infrastructure-failure": true, "artifact-storage-failure": true,
		}
		if result.Infrastructure.Category != nil && !allowed[*result.Infrastructure.Category] {
			errs = append(errs, fmt.Sprintf("infrastructure category %q is not in the local failure policy", *result.Infrastructure.Category))
		}
	} else {
		if result.Evaluation == nil {
			errs = append(errs, "submitted/timed-out status requires evaluation")
			return errs
		}
		if result.Infrastructure.IsFailure {
			errs = append(errs, "submitted/timed-out status cannot be an infrastructure failure")
		}
	}
	if result.Infrastructure.Excluded && (!result.Infrastructure.IsFailure || !result.Infrastructure.ExclusionEligible || result.Infrastructure.ReplacementRunID == nil) {
		errs = append(errs, "excluded infrastructure result requires failure, exclusion eligibility, and replacementRunId")
	}
	if !result.Infrastructure.IsFailure && result.Infrastructure.ReplacementRunID != nil {
		errs = append(errs, "nonfailed result cannot name a replacement")
	}
	if result.Evaluation == nil {
		return errs
	}
	expected := map[string]struct {
		task     string
		required bool
	}{}
	for _, item := range manifest.Cases {
		expected[item.ID] = struct {
			task     string
			required bool
		}{item.Task, item.Required}
	}
	seen := map[string]bool{}
	complete := true
	for name, passed := range result.Evaluation.CommonGates {
		_ = name
		complete = complete && passed
	}
	for _, item := range result.Evaluation.BehaviorCases {
		spec, ok := expected[item.ID]
		if !ok {
			errs = append(errs, fmt.Sprintf("behavior case %q is unknown", item.ID))
			continue
		}
		if seen[item.ID] {
			errs = append(errs, fmt.Sprintf("behavior case %q is duplicated", item.ID))
			continue
		}
		seen[item.ID] = true
		applicable := spec.task == "all" || spec.task == result.Task
		if item.Applicable != applicable {
			errs = append(errs, fmt.Sprintf("behavior case %q applicability is %t, expected %t", item.ID, item.Applicable, applicable))
		}
		if applicable {
			if item.Passed == nil {
				errs = append(errs, fmt.Sprintf("applicable behavior case %q requires Boolean passed", item.ID))
			} else if spec.required && !*item.Passed {
				complete = false
			}
			if item.Evidence == nil || strings.TrimSpace(valueOrEmpty(item.Evidence)) == "" {
				errs = append(errs, fmt.Sprintf("applicable behavior case %q requires non-empty evidence", item.ID))
			}
		} else if item.Passed != nil || item.Evidence != nil {
			errs = append(errs, fmt.Sprintf("inapplicable behavior case %q requires passed and evidence null", item.ID))
		}
	}
	for id := range expected {
		if !seen[id] {
			errs = append(errs, fmt.Sprintf("behavior case %q is missing", id))
		}
	}
	if len(seen) != len(expected) {
		complete = false
	}
	if result.Evaluation.CompleteSuccess != complete {
		errs = append(errs, fmt.Sprintf("evaluation.completeSuccess is %t, recomputed value is %t", result.Evaluation.CompleteSuccess, complete))
	}
	return errs
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
