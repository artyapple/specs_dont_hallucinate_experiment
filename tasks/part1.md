# Task: Implement the Baseline Task Service

Implement the service described by the repository's OpenAPI contract, migrations, canonical SQL operations, and README.

Requirements:

- Preserve the formal artifacts as the public and database contracts.
- Enforce every OpenAPI `additionalProperties: false` constraint at runtime; generated JSON decoding alone may require a handwritten strict-decoding boundary.
- Treat the supplied Problem Details catalog and OpenAPI response examples as exact normative values, not illustrative prose.
- Implement all documented baseline Task API operations.
- Use PostgreSQL through the provided `pgx` pool.
- Keep HTTP, service, and repository responsibilities separated.
- Add meaningful tests for the behavior you implement.
- Make the repository-visible checks pass.
- Report unresolved contract or behavior gaps before finishing.

Do not assume access to hidden evaluator cases.
