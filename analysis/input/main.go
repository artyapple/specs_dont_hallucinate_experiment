// Command analysis-input validates run results at the deterministic boundary
// of the analysis pipeline. It does not aggregate, chart, or count pilots as
// measured observations.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type runInput struct {
	RunID      string          `json:"runId"`
	CellID     string          `json:"cellId"`
	Repeat     int             `json:"repeatIndex"`
	Phase      string          `json:"phase"`
	Status     string          `json:"status"`
	Evaluation json.RawMessage `json:"evaluation"`
}

type indexEntry struct {
	RunID            string `json:"runId"`
	CellID           string `json:"cellId"`
	RepeatIndex      int    `json:"repeatIndex"`
	Phase            string `json:"phase"`
	Status           string `json:"status"`
	CompleteSuccess  *bool  `json:"completeSuccess"`
	CandidateFailure bool   `json:"candidateFailure"`
}

type analysisIndex struct {
	SchemaVersion int          `json:"schemaVersion"`
	Runs          []indexEntry `json:"runs"`
}

func main() {
	root := flag.String("root", ".", "experiment repository root")
	resultsDir := flag.String("results-dir", "", "directory containing immediate child run directories")
	output := flag.String("output", "-", "analysis input index path or - for stdout")
	flag.Parse()
	if flag.NArg() != 0 || *resultsDir == "" {
		fmt.Fprintln(os.Stderr, "analysis-input: -results-dir is required and positional arguments are not accepted")
		os.Exit(2)
	}
	index, err := load(*root, *resultsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "analysis-input:", err)
		os.Exit(2)
	}
	data, _ := json.MarshalIndent(index, "", "  ")
	data = append(data, '\n')
	if *output == "-" {
		_, err = os.Stdout.Write(data)
	} else {
		err = os.WriteFile(*output, data, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "analysis-input: write output:", err)
		os.Exit(2)
	}
}

func load(root, resultsDir string) (analysisIndex, error) {
	compiler := jsonschema.NewCompiler()
	schemaPath, err := filepath.Abs(filepath.Join(root, "schemas", "run-result.schema.json"))
	if err != nil {
		return analysisIndex{}, err
	}
	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		return analysisIndex{}, fmt.Errorf("compile run-result schema: %w", err)
	}
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return analysisIndex{}, err
	}
	index := analysisIndex{SchemaVersion: 1, Runs: []indexEntry{}}
	seen := map[string]string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(resultsDir, entry.Name(), "run-result.json")
		data, err := os.ReadFile(path)
		if err != nil {
			return analysisIndex{}, fmt.Errorf("run directory %q lacks readable run-result.json: %w", entry.Name(), err)
		}
		var raw any
		if err := json.Unmarshal(data, &raw); err != nil {
			return analysisIndex{}, fmt.Errorf("parse %s: %w", path, err)
		}
		if err := schema.Validate(raw); err != nil {
			return analysisIndex{}, fmt.Errorf("validate %s: %w", path, err)
		}
		var run runInput
		if err := json.Unmarshal(data, &run); err != nil {
			return analysisIndex{}, err
		}
		if previous, exists := seen[run.RunID]; exists {
			return analysisIndex{}, fmt.Errorf("duplicate runId %q in %q and %q", run.RunID, previous, entry.Name())
		}
		seen[run.RunID] = entry.Name()
		item := indexEntry{RunID: run.RunID, CellID: run.CellID, RepeatIndex: run.Repeat, Phase: run.Phase, Status: run.Status}
		if string(run.Evaluation) != "null" {
			var evaluation struct {
				CompleteSuccess bool `json:"completeSuccess"`
			}
			if err := json.Unmarshal(run.Evaluation, &evaluation); err != nil {
				return analysisIndex{}, err
			}
			item.CompleteSuccess = &evaluation.CompleteSuccess
			item.CandidateFailure = !evaluation.CompleteSuccess
		} else if run.Status != "infrastructure-failure" && run.Status != "harness-failure" {
			return analysisIndex{}, errors.New("null evaluation is only valid for failure statuses")
		}
		index.Runs = append(index.Runs, item)
	}
	if len(index.Runs) == 0 {
		return analysisIndex{}, errors.New("results directory contains no runs")
	}
	sort.Slice(index.Runs, func(i, j int) bool { return index.Runs[i].RunID < index.Runs[j].RunID })
	return index, nil
}
