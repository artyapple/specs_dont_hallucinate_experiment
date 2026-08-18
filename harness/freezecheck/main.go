// Command freezecheck performs deterministic pre-freeze and post-run validation.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if err := runCLI(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "freezecheck:", err)
		os.Exit(1)
	}
}

func runCLI(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: freezecheck config|schedule|run|results ...")
	}
	switch args[0] {
	case "config":
		flags := newFlags("config")
		root := flags.String("root", "", "repository root")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		if *root == "" {
			return fmt.Errorf("config requires --root")
		}
		return validateConfig(*root)
	case "schedule":
		return scheduleCLI(args[1:])
	case "run":
		flags := newFlags("run")
		root := flags.String("root", "", "repository root")
		runDir := flags.String("run-dir", "", "run directory")
		schedulePath := flags.String("schedule", "", "optional schedule")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		if *root == "" || *runDir == "" {
			return fmt.Errorf("run requires --root and --run-dir")
		}
		return validateRun(*root, *runDir, *schedulePath)
	case "results":
		flags := newFlags("results")
		root := flags.String("root", "", "repository root")
		resultsDir := flags.String("results-dir", "", "directory containing run directories")
		schedulePath := flags.String("schedule", "", "optional schedule")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		if *root == "" || *resultsDir == "" {
			return fmt.Errorf("results requires --root and --results-dir")
		}
		return validateResults(*root, *resultsDir, *schedulePath)
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func scheduleCLI(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("schedule requires generate or validate")
	}
	switch args[0] {
	case "generate":
		flags := newFlags("schedule generate")
		configPath := flags.String("config", "", "experiment config")
		phase := flags.String("phase", "", "measured or pilot")
		seed := flags.String("seed", "", "explicit seed")
		revision := flags.String("config-revision", "", "explicit config revision")
		generatedAt := flags.String("generated-at", "", "explicit RFC3339 timestamp")
		output := flags.String("output", "", "output path or -")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		if *configPath == "" || *phase == "" || *seed == "" || *revision == "" || *generatedAt == "" || *output == "" {
			return fmt.Errorf("schedule generate requires --config, --phase, --seed, --config-revision, --generated-at, and --output")
		}
		data, err := generateSchedule(*configPath, *phase, *seed, *revision, *generatedAt)
		if err != nil {
			return err
		}
		root := filepath.Dir(filepath.Dir(*configPath))
		if err := validateSchema(filepath.Join(root, "schemas", "schedule.schema.json"), data, false); err != nil {
			return fmt.Errorf("generated schedule: %w", err)
		}
		return writeOutput(*output, data)
	case "validate":
		flags := newFlags("schedule validate")
		configPath := flags.String("config", "", "experiment config")
		schedulePath := flags.String("schedule", "", "schedule")
		phase := flags.String("phase", "", "measured or pilot")
		if err := parseFlags(flags, args[1:]); err != nil {
			return err
		}
		if *configPath == "" || *schedulePath == "" || *phase == "" {
			return fmt.Errorf("schedule validate requires --config, --schedule, and --phase")
		}
		return validateScheduleFiles(*configPath, *schedulePath, *phase)
	default:
		return fmt.Errorf("unknown schedule subcommand %q", args[0])
	}
}

func newFlags(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	return nil
}
