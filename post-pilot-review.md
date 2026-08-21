# Post-Pilot Review

Status: corrections implemented; full post-correction pilot phase pending.

## Accepted Pilot Set

The first pilot phase produced one accepted run for every experiment cell. Eleven of fourteen completed successfully. The three candidate failures remain included outcomes:

- `optimistic-locking-full-direct`: handwritten PostgreSQL wrappers selected four columns but scanned three, and repeated `If-Match` field-lines were not rejected;
- `greenfield-codegen`: generated decoding was not wrapped with runtime unknown-field rejection, and handwritten Problem Details prose differed from the exact catalog;
- `optimistic-locking-full-codegen`: canonical generation succeeded, but handwritten adapter identifier mismatches prevented compilation and `formal.sha256` was stale.

None of these three failures was caused by provider, host, container, credential, network, timeout, or evaluator infrastructure. They are retained as non-excluded model outcomes.

## Infrastructure Findings

Two excluded attempts exposed pre-agent PostgreSQL 18 mount incompatibility and Linux Docker socket GID detection in hidden evaluation. The reviewed replacement chain and all artifacts are preserved outside Git. Commits `4d345d9` and `c8db13f` fixed those defects and added a real-container model-free production lifecycle test. The accepted fourteen-run set contains no infrastructure or harness failure.

## Contract Findings

The authoritative design requires the exact Problem Details catalog, but production prompts previously included only the treatment and cell task. Greenfield OpenAPI examples and full-workflow references to the repository contract did not make it explicit that every catalog value was normative. This was a hidden-oracle ambiguity even though the evaluator implemented the intended design correctly.

The full/Greenfield `formal-inputs` gate checked parseability and presence but did not verify the candidate-owned `formal.sha256`. A stale manifest could therefore report a passing formal gate while the repository-visible validation failed.

Visible checks covered the happy path but did not expose strict request decoding or exact validation Problem Details.

## Corrections

- Production prompts include `tasks/problem-details.md` as a separately recorded source for every cell, and the catalog has its own frozen content hash.
- The Greenfield task explicitly requires runtime enforcement of `additionalProperties: false` and treats supplied Problem Details examples as normative.
- Full/Greenfield formal gates and candidate-visible validation verify that `formal.sha256` contains exactly OpenAPI, canonical SQL, and every validly named migration with matching hashes.
- Base 1 and both Base 2 visible checks cover blank titles, unknown request members, malformed UUIDs, exact validation Problem Details, and content type.
- Evaluator behavior, case roster, OCI image bytes, task business semantics, model, agent version, and treatment rules are unchanged.

Because candidate-visible task inputs changed, all fourteen cells must run again as a separate post-correction pilot phase before the global freeze.
