# Hidden Evaluator

Status: baseline milestone implemented; task-specific cases and the evaluator image remain pending.

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

## Runtime Contract

The evaluator accepts a candidate repository path. A candidate must expose these repository-owned commands and environment variables:

- `make build` builds the candidate.
- `make migrate` applies candidate migrations using required `DATABASE_URL`.
- `make run` starts the service using required `DATABASE_URL` and `HTTP_ADDR`.
- `GET /healthz` returns `200` only when the service is ready.

For each invocation the evaluator:

1. Starts fresh PostgreSQL from `docker.io/library/postgres:18.6@sha256:06cad38a5d9f5d24b4d83d86def30795d5e4b757fedbf5281172b576dedcd941` using Testcontainers for Go `v0.44.0`.
2. Waits for the PostgreSQL readiness log with no fixed sleep.
3. Uses the container's dynamically mapped PostgreSQL port.
4. Runs the candidate build and migration commands.
5. Allocates a dynamic loopback HTTP port, starts `make run`, and polls `/healthz` with a 30-second deadline.
6. Runs the same HTTP and direct database checks for every candidate.
7. sends `SIGTERM` to the candidate process group, escalates after 10 seconds if necessary, and terminates PostgreSQL through Testcontainers cleanup.

The standalone result is JSON with setup gates and per-case `id`, `applicable`, `passed`, and `evidence` fields. These case records use the IDs and shape required by `schemas/run-result.schema.json`; the harness can embed them into its complete run result later.

## Commands

From the experiment root:

```text
make build-evaluator
make test-evaluator
make evaluate-base2-direct
make evaluate-base2-codegen
make evaluate-bases
```

Direct invocation from `evaluator/`:

```text
go run ./cmd/evaluator -candidate ../fixtures/base2-direct -output ../results/evaluator-baselines/base2-direct.json
```

The root Make targets create `results/evaluator-baselines/`; the repository ignores that artifact directory.

The process exits `0` only when setup and every baseline case pass, `1` for a candidate/setup failure recorded in JSON, and `2` for invalid CLI usage or failure to write the result.

## Self-Tests

`go test ./...` includes known-broken response candidates and proves rejection by the expected cases:

- reversed list order fails `baseline.list-ordered`;
- an unexpected response member fails `contract.openapi-conformance`;
- wrong Problem Details content type or detail fails `contract.problem-details`.
- a create response without a persisted row fails `contract.database-consistency` against fresh pinned PostgreSQL.

The same production decoding and comparison functions are used by the evaluator and self-tests. `TestBaselineCaseIDsAgree` also checks that all manifest IDs equal the enum in `schemas/run-result.schema.json` and that every implemented baseline ID is named in `cases.md`.

## Remaining Work

- Add task-specific cases and known-broken coverage before the corresponding pilots.
- Integrate standalone output into complete harness-owned `run-result.json` artifacts.
- Stabilize the binary/runtime contract, then add `images/evaluator.Dockerfile` as a separate step.
- Freeze evaluator revision only during the global freeze.
