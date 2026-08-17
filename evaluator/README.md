# Hidden Evaluator

Status: baseline and task-specific cases validated against both Direct and Codegen draft canonical references. Harness run-result integration is implemented, the binary contract is stabilized in `contract.md`, and the OCI image is defined in `../images/evaluator.Dockerfile`. Complete known-broken coverage, independent review, and clean-machine image validation remain pending.

The evaluator is an external black-box Go binary. It does not import candidate code or depend on candidate package names and internal architecture.

## Baseline Cases

The current binary runs these manifest cases:

- `baseline.create-valid`
- `baseline.create-invalid-title`
- `baseline.get-existing`
- `baseline.get-not-found`
- `baseline.list-ordered`
- `baseline.delete-existing`
- `baseline.delete-again-not-found`
- `contract.openapi-conformance`
- `contract.problem-details`
- `contract.database-consistency`

The checks cover UUIDv4 identifiers, Unicode trim and code-point validation, canonical timestamps, stable `(createdAt, id)` ordering including a timestamp tie, exact Problem Details, strict JSON shapes, content types, status codes, and agreement between HTTP create/delete behavior and persisted PostgreSQL rows.

## Task-Specific Cases

The evaluator implements every task-specific ID in `case-manifest.json`:

- 14 Nullable PATCH cases, including the separately decided `nullable.unknown-task` case;
- 10 Optimistic Locking cases, including malformed ETags and a concurrent single-winner check;
- 10 Cursor Pagination cases, including page boundaries, timestamp ties, duplicates, gaps, and cursor reuse after deletion.

`-task` selects `baseline-service`, `nullable-patch`, `optimistic-locking`, or `cursor-pagination`. Every result contains the complete 44-case manifest roster exactly once. Cases for the selected task and all common cases are applicable; other task cases use `applicable: false` and `passed: null`. Each task-specific case starts with `TRUNCATE TABLE tasks` and creates its own state.

## Runtime Contract

The stable binary contract — CLI flags, environment, candidate input and output paths, task selection, exit codes, timeouts, diagnostics bounds, signal handling, cleanup, and case representation — is defined in `contract.md` and covered by contract tests. In brief: the evaluator accepts a candidate repository path and task selector, requires candidate-owned `make build`, `make migrate`, and `make run` plus a `GET /healthz` readiness endpoint, and emits a single result JSON.

## Commands

From the experiment root:

```text
make build-evaluator
make test-evaluator
make build-evaluator-image
make test-evaluator-image
make evaluate-base2-direct
make evaluate-base2-codegen
make evaluate-bases
```

Direct invocation from `evaluator/`:

```text
go run ./cmd/evaluator -task baseline-service -candidate ../fixtures/base2-direct -output ../results/evaluator-baselines/base2-direct.json
```

The root Make targets create `results/evaluator-baselines/`; the repository ignores that artifact directory.

The process exits `0` only when setup and every applicable case pass, `1` for a candidate/setup failure recorded in JSON, and `2` for invalid CLI usage, an unwritable result, or an aborted run (signal or evaluation budget exhausted; no partial result is written).

## Self-Tests

`go test ./...` includes known-broken response candidates and proves rejection by the expected cases:

- reversed list order fails `baseline.list-ordered`;
- an unexpected response member fails `contract.openapi-conformance`;
- wrong Problem Details content type or detail fails `contract.problem-details`.
- a create response without a persisted row fails `contract.database-consistency` against fresh pinned PostgreSQL.
- treating omitted `dueAt` as null fails `nullable.omitted-preserves`;
- allowing two concurrent winners fails `locking.concurrent-single-winner`;
- returning a duplicate across pages fails `pagination.multiple-pages`.

The same production case functions, decoding, and comparison functions are used by the evaluator and self-tests. `TestCaseRegistryManifestAndSchemaAgree` checks registry IDs and task applicability against `case-manifest.json`, the enum in `schemas/run-result.schema.json`, and `cases.md`. Applicability tests verify the complete roster and null `passed` representation.

## Canonical Matrix

Run `make evaluate-task-solutions` to evaluate all six draft canonical references. The expected applicable counts are 24 for each Nullable candidate and 20 for each Locking or Pagination candidate. Result artifacts are written under ignored `results/task-solutions/`.

## Remaining Work

- Complete known-broken coverage for every important task behavior class.
- Complete independent human review of all six draft canonical references.
- Validate the evaluator OCI archive on a separate clean machine during the global freeze.
- Freeze evaluator revision only during the global freeze.
