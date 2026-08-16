# Direct Treatment Overlay

## Agent-visible rules

- Read and maintain the OpenAPI, migrations, and canonical SQL query files.
- Implement derived HTTP and database Go code manually.
- Implement handlers, service logic, mappings, tests, and wiring.
- Do not invoke or recreate `oapi-codegen`, `sqlc`, or another code generator.
- Use the provided canonical SQL operations. Do not replace them with unrelated inline SQL.
- Run the repository-visible checks before finishing.

## Environment rules

- Generator binaries are absent.
- Generator configuration files are absent.
- Attempts to invoke forbidden generators are recorded as protocol events.
