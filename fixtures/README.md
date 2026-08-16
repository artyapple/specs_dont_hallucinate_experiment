# Fixtures

Fixtures are immutable starting repositories for agent runs.

## Required Fixtures

- `base1`: shared Greenfield inputs and infrastructure skeleton.
- `base2-direct`: canonical service with handwritten HTTP and database boundaries.
- `base2-codegen`: behaviorally equivalent service with generated HTTP and database boundaries.

## Rules

- Every fixture revision is frozen by commit hash before measured runs.
- Base 2 OpenAPI, migrations, and canonical SQL query text are byte-identical.
- Base 2 variants pass one external baseline evaluator.
- `manifest.md` lists every allowed difference between Base 2 variants.
- Hidden evaluator files never enter an agent-visible fixture.

## Shared Formal Layout

Every fixture uses these paths:

- `api/openapi.yaml`: public HTTP contract;
- `db/migrations/*.sql`: ordered forward-only PostgreSQL migrations;
- `db/queries/*.sql`: canonical named SQL operations.

The initial baseline formal inputs live in `base1`. Base 2 construction must copy them byte-for-byte before adding complete implementations.
