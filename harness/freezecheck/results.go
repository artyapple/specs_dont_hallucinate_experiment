package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func validateResults(root, resultsDir, schedulePath string) error {
	resultsDir, err := filepath.Abs(resultsDir)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return fmt.Errorf("read results directory: %w", err)
	}
	type resultEntry struct {
		directory string
		result    runResult
	}
	validated := []resultEntry{}
	errs := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(resultsDir, entry.Name())
		if _, err := os.Lstat(filepath.Join(runDir, "run-result.json")); os.IsNotExist(err) {
			continue
		} else if err != nil {
			errs = append(errs, fmt.Sprintf("run directory %q: inspect run-result.json: %v", entry.Name(), err))
			continue
		}
		result, err := validateRunDocument(root, runDir, schedulePath, true)
		validated = append(validated, resultEntry{directory: entry.Name(), result: result})
		if err != nil {
			errs = append(errs, fmt.Sprintf("run directory %q: %v", entry.Name(), err))
		}
	}
	if len(validated) == 0 {
		errs = append(errs, "results directory contains no immediate child run directories with run-result.json")
		return diagnostics(errs)
	}
	byID := map[string]resultEntry{}
	duplicates := map[string]bool{}
	for _, entry := range validated {
		id := entry.result.RunID
		if previous, exists := byID[id]; exists {
			errs = append(errs, fmt.Sprintf("duplicate runId %q in directories %q and %q", id, previous.directory, entry.directory))
			duplicates[id] = true
			continue
		}
		byID[id] = entry
	}
	for _, entry := range validated {
		result := entry.result
		if duplicates[result.RunID] {
			continue
		}
		if link := result.Infrastructure.ReplacementRunID; link != nil {
			if *link == result.RunID {
				errs = append(errs, fmt.Sprintf("runId %q replacementRunId must not self-link", result.RunID))
			} else if target, ok := byID[*link]; !ok || duplicates[*link] {
				errs = append(errs, fmt.Sprintf("runId %q replacementRunId %q does not resolve uniquely in result set", result.RunID, *link))
			} else {
				if target.result.CellID != result.CellID {
					errs = append(errs, fmt.Sprintf("runId %q replacementRunId %q points to cell %q, expected %q", result.RunID, *link, target.result.CellID, result.CellID))
				}
				if target.result.Infrastructure.ReplacesRunID == nil || *target.result.Infrastructure.ReplacesRunID != result.RunID {
					errs = append(errs, fmt.Sprintf("runId %q replacementRunId %q does not point back through replacesRunId", result.RunID, *link))
				}
			}
		}
		if link := result.Infrastructure.ReplacesRunID; link != nil {
			if *link == result.RunID {
				errs = append(errs, fmt.Sprintf("runId %q replacesRunId must not self-link", result.RunID))
			} else if original, ok := byID[*link]; !ok || duplicates[*link] {
				errs = append(errs, fmt.Sprintf("runId %q replacesRunId %q does not resolve uniquely in result set", result.RunID, *link))
			} else {
				if original.result.CellID != result.CellID {
					errs = append(errs, fmt.Sprintf("runId %q replacesRunId %q points to cell %q, expected %q", result.RunID, *link, original.result.CellID, result.CellID))
				}
				if original.result.Infrastructure.ReplacementRunID == nil || *original.result.Infrastructure.ReplacementRunID != result.RunID {
					errs = append(errs, fmt.Sprintf("runId %q replacesRunId %q does not point back through replacementRunId", result.RunID, *link))
				}
			}
		}
	}
	return diagnostics(errs)
}
