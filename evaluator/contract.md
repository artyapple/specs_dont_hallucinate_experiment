# Evaluator Binary Contract

Status: stable draft. This contract is frozen as part of the global freeze; until then changes are allowed only with a roadmap Task 3 evidence update. The contract is covered by tests in `cmd/evaluator/main_test.go` and `internal/evaluator/contract_test.go`.

## Invocation

```text
evaluator -task <task> -candidate <dir> [-output <path|->]
```

- `-candidate` (required): path to the candidate repository. Resolved to an absolute path; must be an existing directory. Extra positional arguments are rejected.
- `-task` (required): one of `baseline-service`, `nullable-patch`, `optimistic-locking`, `cursor-pagination`.
- `-output`: result JSON path, or `-` for standard output (default). The parent directory must exist before the run starts.

The evaluator is stage-agnostic and treatment-agnostic. The harness maps a cell to exactly one `-task` value; stage and treatment never reach the evaluator. Treatment-specific Codegen health checks are harness-owned and are not part of this contract.

## Environment

- The evaluator process requires access to a Docker-compatible container runtime (Testcontainers for Go v0.44.0, including its standard `DOCKER_*` and `TESTCONTAINERS_*` environment passthrough).
- The evaluator OCI image sets standard Testcontainers variable `TESTCONTAINERS_RYUK_CONTAINER_IMAGE` to the digest-pinned Ryuk cleanup image recorded in `config/versions.json`; the host daemon must contain that exact image before offline evaluation.
- The evaluator defines no evaluator-specific environment variables.
- Candidate commands inherit the evaluator process environment with provider credentials (`OPENROUTER_API_KEY`) stripped, plus exactly `DATABASE_URL` for `make build`, `make migrate`, and `make run`, plus `HTTP_ADDR` for `make run` only. The evaluator image contains no provider credentials and must never receive any at runtime; the harness run-result assembler additionally strips provider keys from the environment it passes to the evaluator process.

## Candidate Contract

The candidate repository must expose:

- `make build` — builds the candidate;
- `make migrate` — applies forward-only migrations using the required `DATABASE_URL`;
- `make run` — starts the service using `DATABASE_URL` and `HTTP_ADDR`;
- `GET /healthz` — returns `200` only when the service is ready.

The evaluator executes these commands in place inside the candidate tree and writes nothing else into it. Candidate build and migration commands each have a 5 minute budget; on expiry or abort the whole candidate command process group is killed.

## Run Sequence

1. Start the pinned PostgreSQL image (`docker.io/library/postgres:18.6@sha256:06cad38a5d9f5d24b4d83d86def30795d5e4b757fedbf5281172b576dedcd941`) via Testcontainers, waiting for the readiness log (second occurrence, 60 second startup timeout), never a fixed sleep, using the dynamically mapped port.
2. Run `make build`, then `make migrate`.
3. Allocate a dynamic loopback port, start `make run`, and poll `GET /healthz` every 100 ms with a 30 second deadline and a 2 second per-request timeout.
4. Run the behavior cases in registry order. Task-specific cases start with `TRUNCATE TABLE tasks` and create their own state. Every HTTP request has a 5 second timeout.
5. Stop the candidate process group with `SIGTERM`, escalate to `SIGKILL` after 10 seconds, and terminate PostgreSQL through Testcontainers cleanup.

The complete run is bounded by a 15 minute evaluation budget enforced by the binary.

## Result JSON

The standalone result is a single JSON object:

- `schemaVersion` (always `1`), `candidate` (absolute path), `task`, `startedAt`, `finishedAt` (RFC 3339 UTC);
- `completeSuccess`: true only when setup passed and every applicable case passed;
- `setup`: `postgres`, `build`, `migrations`, `serviceReady` booleans plus optional `evidence` string;
- `behaviorCases`: the complete 44-case manifest roster exactly once, in registry order, each with:
  - `id`: the manifest case ID;
  - `applicable`: true for `task: all` cases and cases matching `-task`;
  - `passed`: Boolean for applicable cases, JSON `null` for non-applicable cases;
  - `evidence`: a string for applicable cases (`"passed"`, the failure detail, or `"not run: <setup failure>"` after a setup failure), the empty string for non-applicable cases;
- `serviceLogs` (optional): the bounded tail of candidate service logs.

Diagnostics bounds: candidate command output and service logs are truncated to the last 16 KiB with an explicit truncation marker; response bodies embedded in case evidence are truncated at 8 KiB; buffered response bodies are limited to 64 KiB. Result files are written atomically (temporary file plus rename), so a result is either complete or absent.

## Exit Codes

- `0`: setup passed and every applicable case passed;
- `1`: a candidate or setup failure, recorded in the result JSON;
- `2`: invalid CLI usage, an unwritable result path, or an aborted run.

## Abort Behavior

On `SIGINT`, `SIGTERM`, or exhaustion of the 15 minute evaluation budget, the evaluator cancels in-flight work, kills candidate command process groups, stops the candidate service process group, terminates the PostgreSQL container, prints an abort diagnostic to standard error, and exits `2` without writing a result. Partial behavior outcomes are never written.

## Failure Semantics

- A PostgreSQL startup failure (`setup.postgres=false`) is an evaluator infrastructure failure; the candidate never ran.
- `build`, `migrations`, or `serviceReady` setup failures are candidate failures and are recorded in the result JSON.
- Behavior case failures are candidate failures with per-case evidence.

The harness consumes this contract to distinguish candidate, harness, and external infrastructure failures (see `harness/README.md`).
