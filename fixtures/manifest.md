# Base 2 Allowed Differences

Status: pilot-fixed draft. The allowed-difference policy is fixed before pilots but becomes globally frozen only in Roadmap Task 12.

The Base 2 variants may differ only in these categories:

1. `oapi-codegen` and `sqlc` configuration files in `base2-codegen`.
2. Generated HTTP and database files in `base2-codegen`.
3. Handwritten HTTP and database boundary implementations in `base2-direct`.
4. Treatment-specific generation commands.
5. Minimal wiring required by the different boundary implementations.

They must not differ in:

- business behavior;
- OpenAPI content;
- migration content;
- canonical SQL query text;
- Go, framework, driver, or shared dependency versions;
- service-layer business rules;
- visible baseline behavior checks.

Every additional difference requires written justification and a new review before freeze.
