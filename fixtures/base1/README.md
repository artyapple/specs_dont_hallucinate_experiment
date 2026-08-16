# Base 1: Greenfield

Status: baseline formal inputs and shared infrastructure skeleton implemented; Task API implementation intentionally absent.

Base 1 will contain:

- common agent README;
- Go module and preloaded dependencies;
- configuration loading;
- `pgx` pool setup;
- HTTP server lifecycle;
- baseline OpenAPI;
- baseline migrations;
- canonical SQL query files;
- visible `make check` support.

Base 1 will not contain ready HTTP models, routing, handlers, semantic mappings, or database wrappers.

The Direct overlay adds no generators. The Codegen overlay adds pinned configs, binaries, and `make generate`.

## Formal Inputs

- `api/openapi.yaml` defines the baseline HTTP API.
- `db/migrations/000001_create_tasks.sql` is the forward-only baseline migration.
- `db/queries/tasks.sql` contains the four canonical SQL operations that both treatments must preserve.

From the experiment root, `make validate-formal` validates these inputs with the pinned development Codegen and PostgreSQL images. That command is fixture-construction tooling; it is not exposed as a generator capability to Direct agent runs.

Inside this fixture, `make validate-formal` checks the frozen formal-input hashes without requiring generator access. `make verify-skeleton` builds and tests the shared infrastructure before an agent run.

## Runtime Contract

- `DATABASE_URL` is required and points to the harness-managed PostgreSQL database.
- `HTTP_ADDR` is optional and defaults to `:8080` for `make run`.
- `make migrate` applies the ordered forward-only migrations to a fresh database.
- `make run` starts the service and handles `SIGINT`/`SIGTERM` with graceful shutdown.
- `GET /healthz` returns `200` after configuration is valid and PostgreSQL is reachable.
- `make check` expects a clean database, uses `HTTP_ADDR` or `127.0.0.1:18080`, waits up to 30 seconds for readiness, and runs one visible create/get/list/delete happy path.
- Successful visible validation prints `visible check passed`.

The starting fixture's `make check` intentionally fails at the first Task API request because no Task routes, models, handlers, service logic, or repository wrappers are provided. Implementing those boundaries is the agent task.
