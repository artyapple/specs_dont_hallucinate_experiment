# Base 2 Codegen

Status: canonical baseline implementation complete; common external baseline evaluator passed.

This canonical repository will implement the baseline Task API using:

- `oapi-codegen` strict server generation for `chi`;
- `sqlc` generation targeting `pgx/v5` with a query interface;
- handwritten handlers, service logic, mappings, tests, and wiring.

It must:

- pass the common baseline evaluator;
- use byte-identical OpenAPI, migrations, and SQL operations from Base 2 Direct;
- regenerate byte-for-byte;
- contain no manual edits in generated files;
- expose `make generate`, `make verify-generate`, and `make check`.

`make verify-generate` regenerates into a temporary tree and compares every generated file byte-for-byte. Handwritten adapters provide strict unknown-field handling, frozen Problem Details, service mappings, and wiring without modifying generated files.
