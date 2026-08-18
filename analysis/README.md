# Analysis

Analysis is descriptive and reproducible from machine-readable run artifacts.

## Required Outputs

- Complete-success count out of five for every cell.
- Per-run behavior-case heatmaps.
- All observations for wall time, tokens, turns, tool calls, repair cycles, and time to first build.
- Contract, handwritten, and generated diff surfaces.
- Protocol violations, infrastructure failures, exclusions, and replacements.
- Codegen regeneration health shown separately.
- Compiler-event profiles shown separately for Direct and Codegen.

## Rules

- Do not pool tasks into one success rate.
- Do not pool full-workflow and propagation-only modes.
- Do not create a composite score.
- Do not report p-values or statistical significance.
- Show all five observations, plus median and range when useful.
- Do not count generated LOC as handwritten review effort.
- Select representative examples only after aggregate analysis.
- Keep pilot artifacts out of measured aggregation.

## Input Validation

`analysis/input/` is the deterministic pre-aggregation boundary. It validates every immediate child `run-result.json` against `schemas/run-result.schema.json`, rejects incomplete directories and duplicate run IDs, preserves null evaluations for infrastructure and harness failures, distinguishes evaluated candidate failures, and emits a run-ID-sorted JSON index. It does not aggregate cells, produce charts, or count pilot inputs as measured observations.

```text
make validate-analysis-input RESULTS_DIR=<path> OUTPUT=<path>
```

## Compiler Event Coding

Freeze coding rules for:

- generated-interface mismatch;
- generated-type mismatch;
- handwritten compile error;
- unrelated tooling error;
- diagnostic locality;
- handwritten adaptation point;
- subsequent relevant repair.

This analysis remains exploratory.

## TODO

- Choose the implementation language and chart library.
- Implement deterministic aggregation after the schema-valid input boundary.
- Define diff classification rules and generated-file manifests.
- Define repair-iteration extraction from command events.
- Produce placeholder charts from synthetic data before measured runs.
