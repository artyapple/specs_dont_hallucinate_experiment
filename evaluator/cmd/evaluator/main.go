// Command evaluator is the hidden black-box evaluator. Its stable binary
// contract is documented in evaluator/contract.md.
//
// Exit codes:
//
//	0: setup and every applicable behavior case passed;
//	1: a candidate or setup failure, recorded in the result JSON;
//	2: invalid CLI usage, failure to write the result, or an aborted run.
//
// An aborted run (SIGINT, SIGTERM, or exhaustion of the evaluation budget)
// performs full cleanup and exits 2 without writing a result, so no partial
// behavior outcomes are ever fabricated.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"specs-dont-hallucinate/evaluator/internal/evaluator"
)

func main() {
	candidate := flag.String("candidate", "", "path to the candidate repository")
	task := flag.String("task", "", "candidate task: baseline-service, nullable-patch, optimistic-locking, or cursor-pagination")
	output := flag.String("output", "-", "result JSON path, or - for stdout")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments: %v\n", flag.Args())
		os.Exit(2)
	}
	if *candidate == "" {
		fmt.Fprintln(os.Stderr, "-candidate is required")
		os.Exit(2)
	}
	if !evaluator.ValidTask(*task) {
		fmt.Fprintln(os.Stderr, "-task must be baseline-service, nullable-patch, optimistic-locking, or cursor-pagination")
		os.Exit(2)
	}

	absCandidate, err := filepath.Abs(*candidate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve candidate path: %v\n", err)
		os.Exit(2)
	}
	if info, err := os.Stat(absCandidate); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "candidate %s is not an existing directory\n", absCandidate)
		os.Exit(2)
	}
	if *output != "-" {
		if info, err := os.Stat(filepath.Dir(*output)); err != nil || !info.IsDir() {
			fmt.Fprintf(os.Stderr, "output directory of %s does not exist\n", *output)
			os.Exit(2)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, evaluator.EvaluationBudget)
	defer cancel()
	result := evaluator.Evaluate(ctx, evaluator.Options{Candidate: absCandidate, Task: *task})
	if ctx.Err() != nil {
		fmt.Fprintf(os.Stderr, "evaluation aborted (%v); no result written\n", ctx.Err())
		os.Exit(2)
	}

	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "encode result: %v\n", err)
		os.Exit(2)
	}
	if *output == "-" {
		if _, err := os.Stdout.Write(buffer.Bytes()); err != nil {
			fmt.Fprintf(os.Stderr, "write result: %v\n", err)
			os.Exit(2)
		}
	} else if err := writeAtomic(*output, buffer.Bytes()); err != nil {
		fmt.Fprintf(os.Stderr, "write result: %v\n", err)
		os.Exit(2)
	}
	if !result.CompleteSuccess {
		os.Exit(1)
	}
}

// writeAtomic writes the result through a temporary file and rename so an
// interrupted write can never leave a truncated JSON artifact behind.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".evaluator-result-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
