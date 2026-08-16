package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"specs-dont-hallucinate/evaluator/internal/evaluator"
)

func main() {
	candidate := flag.String("candidate", "", "path to the candidate repository")
	output := flag.String("output", "-", "result JSON path, or - for stdout")
	flag.Parse()
	if *candidate == "" {
		fmt.Fprintln(os.Stderr, "-candidate is required")
		os.Exit(2)
	}

	absCandidate, err := filepath.Abs(*candidate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve candidate path: %v\n", err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result := evaluator.Evaluate(ctx, absCandidate)

	var writer *os.File
	if *output == "-" {
		writer = os.Stdout
	} else {
		writer, err = os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create result: %v\n", err)
			os.Exit(2)
		}
		defer writer.Close()
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "write result: %v\n", err)
		os.Exit(2)
	}
	if !result.CompleteSuccess {
		os.Exit(1)
	}
}
