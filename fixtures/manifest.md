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

Review resolution (2026-08-20): independent review finding `REVIEW-004` identified that the Direct Base 2 fixture retained `golang.org/x/sync v0.17.0` and `golang.org/x/text v0.29.0` while the Codegen fixture selected `v0.19.0` and `v0.32.0` through its generator runtime dependencies. The Direct fixture and all three Direct canonical task solutions now use the same shared indirect versions as Codegen. No production source, formal input, generated output, or public behavior changed.
