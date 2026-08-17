# AI Direct vs AI + Codegen Experiment

This repository will host the reproducible experiment for the talk `Specs Don't Hallucinate`.

## Status

Development infrastructure only. It is not ready for pilots or measured runs.

Version selection, nullable-generation compatibility, the isolated tool bridge, a real-provider smoke, reproducible development OCI exports, baseline formal inputs, the Base 1 infrastructure skeleton, both canonical Base 2 services, and the external baseline and task-specific evaluator are implemented. Both Base 2 variants and all six draft canonical task references pass the applicable evaluator cases. The Base 1 Task API is intentionally absent; independent review of task references, complete run orchestration, and remaining freeze requirements remain open.

The authoritative design decisions currently live in `../experiment-decisions.md`. Before the first pilot, copy or link a frozen revision into this repository so every run can reference an immutable design artifact.

## Research Question

> AI can write the first version. Can deterministic code generation make the next change safer?

## Matrix

- 10 Greenfield measured runs.
- 60 existing-service-change measured runs.
- 70 measured runs total.
- 14 unmeasured pilots, one per cell.
- Five measured repetitions per cell.

See `config/experiment.json` for the machine-readable matrix.

## Layout

```text
experiment/
├── config/                  experiment matrix and frozen version metadata
├── fixtures/                immutable starting repositories
│   ├── base1/
│   ├── base2-direct/
│   └── base2-codegen/
├── treatments/              Direct and Codegen overlays
├── tasks/                   agent-visible task texts and propagation patches
├── harness/                 run orchestration and artifact capture
├── evaluator/               hidden black-box evaluation
├── compatibility/           generator capability probes
├── review/                  optional human-review protocol
├── schemas/                 machine-readable artifact schemas
├── analysis/                deterministic aggregation and charts
├── pilots/                  ignored unmeasured artifacts
└── results/                 ignored measured artifacts
```

## Required Commands

The implemented repository must eventually expose:

```text
make validate-config
make build-fixtures
make verify-bases
make pilot CELL=<cell-id> REPEAT=<n>
make run CELL=<cell-id> REPEAT=<n>
make evaluate RUN_ID=<run-id>
make analyze
```

Fixture repositories must expose a treatment-appropriate `make check`. Codegen fixtures also expose `make generate` and `make verify-generate`.

## Implementation Order

1. Pin versions and image digests in `config/versions.json`.
2. Verify `oapi-codegen` nullable three-state output.
3. Build the baseline OpenAPI, migrations, and canonical SQL operations.
4. Build and validate Base 1.
5. Build equivalent Base 2 Direct and Codegen repositories.
6. Implement evaluator gates and known-broken evaluator tests.
7. Implement containerized OpenCode run orchestration.
8. Validate end-to-end artifact production against `schemas/run-result.schema.json`.
9. Execute all 14 pilots.
10. Freeze revisions, schedule, and analysis rules before measured runs.

## Safety

- Never commit provider credentials.
- Scrub transcripts before publication.
- Preserve infrastructure failures and protocol violations.
- Never tune the hidden evaluator against measured candidates.
