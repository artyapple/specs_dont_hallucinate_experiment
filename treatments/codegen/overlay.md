# Codegen Treatment Overlay

## Agent-visible rules

- Read and maintain the OpenAPI, migrations, and canonical SQL query files.
- Run the provided canonical generation command after changing formal inputs.
- Do not edit generated files manually.
- Implement handlers, service logic, mappings, tests, and wiring.
- Run the repository-visible checks before finishing.

## Environment rules

- Pinned `oapi-codegen` and `sqlc` binaries are available.
- Frozen generator configs are present.
- `make generate` is the canonical generation command.
- `make verify-generate` checks byte-for-byte canonical output.
