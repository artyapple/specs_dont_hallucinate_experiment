# Visible Check Equivalence

Every fixture exposes `make check` with the same purpose:

1. Validate formal inputs.
2. Run `go test ./...`.
3. Apply migrations to a clean harness-managed visible PostgreSQL sidecar.
4. Start the service and wait for actual readiness.
5. Run one baseline happy-path HTTP scenario.
6. Verify the baseline request envelope: strict JSON decoding, malformed identifiers, and the exact validation Problem Details response.

Treatment-specific implementation is allowed only where required:

- Codegen may invoke its pinned generation command before validation.
- Direct has no generator step.
- Generator canonicality remains a separate Codegen health check.

The visible checks expose only the baseline HTTP contract documented in `tasks/part1.md` and `tasks/problem-details.md`. They must not reveal nullable, locking, pagination, or other task-specific hidden regression cases.

## Command Contract

- The harness supplies a fresh visible PostgreSQL sidecar through required `DATABASE_URL`.
- `HTTP_ADDR` selects the service listen address; the visible check defaults to `127.0.0.1:18080`.
- `make migrate` applies forward-only migrations.
- `make run` starts the service.
- `GET /healthz` is the readiness endpoint.
- `make check` performs all six steps above, allows 30 seconds for readiness, and limits each HTTP operation to 5 seconds.
- Success prints exactly `visible check passed` as its final line.

The harness owns the clean sidecar lifecycle. `make check` must not create containers or require a Docker socket.
