# Canonical Task Solutions Independent Review

Reviewer: sultix
Review date: 2026-08-19
Amended: 2026-08-20 — the experiment owner supplied `experiment-decisions.md`; its SHA-256 was verified against `config/design-revision.json`, every conclusion was re-checked against it, no decision changed, and REVIEW-004 was added.
Reviewed revision: e244d406c818c5f4297f3993f24e8c999e4e6bd1
Relevant experience: automated review of Go/chi/pgx HTTP services, OpenAPI/sqlc/oapi-codegen toolchains, and concurrency/pagination correctness analysis

## Independence Statement

I did not author the canonical solutions under review. This review session created no commits and modified no repository content; the only artifact it produced is this report.

I reviewed the implementation directly — every changed file of all six solutions relative to its Base 2 starting fixture, the formal inputs, the generated outputs, the candidate tests, and the relevant evaluator case implementations — and did not delegate any part of the judgment to another system.

## Review Scope And Environment

- Working repository: the repository root itself plays the role of `experiment/` from the instruction (there is no `experiment/` subdirectory); all paths below are relative to the repository root.
- Verified at review start: branch `main`, clean worktree, `git rev-parse HEAD` = `e244d406c818c5f4297f3993f24e8c999e4e6bd1`, and `git merge-base --is-ancestor f2a1910 HEAD` succeeded (exit 0).
- The only commit after the `f2a1910` baseline is `e244d40`, which adds `reviewer-instructions.md`; no canonical solution content changed after `f2a1910`.
- **The authoritative design document was initially absent and later verified.** At review time `../experiment-decisions.md` was not present on the review machine, so the initial pass fell back (per the instruction's priority order) to `ROADMAP.md`, the experiment config/schemas, the implementations, and the scaffold documentation. On 2026-08-20 the owner supplied the document; its SHA-256 (`4f0832e57c4502604f8827ceaada44b3fef95d8a4cd29a2dc6faa1f43d6875f6`) exactly matches `config/design-revision.json`, so the reviewed text is the recorded authoritative revision. All task requirements (Tasks A/B/C), the Base 1 canonical-SQL constraint, the Base 2 sharing rules, the Problem Details contract, and the evaluation principles were re-checked against it: the fallback sources were consistent with the authoritative document, and no solution decision, equivalence decision, or treatment-constraint conclusion changed. The re-check confirmed REVIEW-001 and REVIEW-002 as non-blocking using the document's own wording and surfaced one additional inherited observation, recorded as REVIEW-004.
- Toolchain used: Go 1.26.2 (darwin/arm64), Docker Desktop 28.4.0, jq 1.7.1. Codegen regeneration ran in the pinned `specs-export/tool-codegen:go1.26.6` image (linux/amd64 under emulation on arm64; outputs compared byte-for-byte, so emulation does not weaken the check).

## Automated Checks

| Command | Result | Notes |
|---|---|---|
| make validate-task-targets | passed | Formal patches re-applied to the Base 2 formal tree; patched trees compared byte-for-byte against the `api/` and `db/` trees of all six solutions; target-manifest hashes validated. |
| make verify-task-solutions | passed | Per solution: `go mod verify`, `go test ./...`, `go vet ./...`. Per Codegen solution: `make verify-generate` inside the pinned read-only tool image reported "generated files are canonical" (byte-for-byte regeneration). |
| make evaluate-task-solutions | passed | External evaluator run against all six solutions with digest-pinned PostgreSQL 18.6 via Testcontainers. |

Evaluator results (`results/task-solutions/*.json`, verified with the prescribed `jq` filter):

| Result file | completeSuccess | Setup | Applicable cases passed |
|---|---|---|---|
| nullable-patch-direct.json | true | all true | 24/24, failed: [] |
| nullable-patch-codegen.json | true | all true | 24/24, failed: [] |
| optimistic-locking-direct.json | true | all true | 20/20, failed: [] |
| optimistic-locking-codegen.json | true | all true | 20/20, failed: [] |
| cursor-pagination-direct.json | true | all true | 20/20, failed: [] |
| cursor-pagination-codegen.json | true | all true | 20/20, failed: [] |

All six results carry the full 44-case roster; non-applicable cases encode `passed: null` as required by the evaluator contract.

The automated checks were treated as corroboration only; every conclusion below is grounded in reading the implementation.

## Solution Decisions

| Solution | Decision | Blocking findings |
|---|---|---|
| nullable-patch-direct | approved | none |
| nullable-patch-codegen | approved | none (non-blocking REVIEW-001) |
| optimistic-locking-direct | approved | none (non-blocking REVIEW-002, REVIEW-003) |
| optimistic-locking-codegen | approved | none (non-blocking REVIEW-003) |
| cursor-pagination-direct | approved | none (non-blocking REVIEW-003) |
| cursor-pagination-codegen | approved | none (non-blocking REVIEW-003) |

Non-blocking REVIEW-004 (dependency-version skew inherited from the Base 2 fixtures) applies to all six solutions equally and is resolvable only at the fixtures level.

### Review basis per task

**Nullable PATCH.** The three `dueAt` states are represented losslessly end to end in both variants. Direct: `patchTaskRequest` uses `json.RawMessage` for `title`/`dueAt`, so absent (`nil`), literal `null`, and a value are distinguished at the HTTP boundary and carried as `task.Patch{Title *string, DueAtPresent bool, DueAt *time.Time}` (`internal/httpapi/handler.go:117-153`, `internal/task/service.go:78-91`). Codegen: `oapi-codegen.yaml` enables `nullable-type: true`, the generated `PatchTaskRequest.DueAt` is `nullable.Nullable[time.Time]`, and the handwritten adapter maps specified/null/value onto the same `task.Patch` (`internal/httpadapter/handler.go:128-159`). Both variants execute the canonical `PatchTask` SQL — a single `UPDATE` with `CASE WHEN <present>` guards per column — so "omitted" never touches the column, `null` writes SQL `NULL`, and a timestamp writes the value in one atomic statement; the updated row is returned via `RETURNING` and mapped straight into the response. `title: null` yields 400 in both variants (Direct: unmarshals to an empty string that fails 1–200 code-point validation; Codegen: an explicit raw-member check in the strict-body middleware). Empty PATCH objects, unknown fields, `POST` with `dueAt` (unknown field under `DisallowUnknownFields`), unknown task (404 via `pgx.ErrNoRows`), and UTC-with-six-fractional-digits rendering (`.UTC().Format("2006-01-02T15:04:05.000000Z")`, applied identically to `createdAt` and `dueAt`) were all verified in both variants. `dueAt` is a required member and marshals as JSON `null` when unset in both (`*string` without `omitempty`; always-assigned `nullable.Nullable[string]`). The dueAt timestamp parse paths (`time.Parse(time.RFC3339, …)` vs `time.Time.UnmarshalJSON`) were probed empirically on Go 1.26 with lowercase `t`/`z`, `+00:00`/`-00:00` offsets, and nanosecond precision: both accept and reject identical inputs.

**Optimistic Locking.** The canonical `UpdateTask` SQL (`UPDATE … SET title = $2, version = version + 1 WHERE id = $1 AND version = $3 RETURNING …`) is used verbatim by both variants — Direct as a handwritten constant (guarded against drift by `TestQueriesMatchCanonicalSQL`), Codegen through the sqlc-generated `Queries.UpdateTask`. Match, title update, and version increment happen in one atomic statement; there is no read-then-write pattern anywhere. For two concurrent PUTs with the same current ETag, PostgreSQL row locking guarantees exactly one row match, so exactly one request succeeds and the loser observes zero rows; the follow-up existence probe is used only to classify the failure as 404 vs 412 and never influences the update itself. `If-Match` parsing is a character-for-character identical handwritten function in both variants: missing header → 428; two header lines, a list in one line, unquoted, weak (`W/"1"`), wildcard, zero, signs, leading zeros, and 64-bit overflow → 400; error precedence is header syntax → resource existence → version match in both (Direct checks the header before body decoding; Codegen checks it in middleware before the generated router binds anything). Response ETags are produced by `strconv.Quote(strconv.FormatInt(version, 10))` (strong, canonical decimal, no leading zeros) on POST 201, item GET 200, and successful PUT 200; the collection response carries no ETag. Version starts at 1 via the migration default (`version bigint NOT NULL DEFAULT 1` with a positive-value CHECK) and appears as an integer in every Task JSON. The evaluator's `locking.malformed-if-match` case was read and exercises all ten malformed forms from instruction §17; `locking.stale-if-match` additionally verifies the stored row (winner title, version 2) against the database.

**Cursor Pagination.** The canonical `ListTasks` SQL uses the correct tuple predicate `WHERE NOT $1::boolean OR (created_at, id) > ($2::timestamptz, $3::uuid) ORDER BY created_at ASC, id ASC LIMIT $4::integer`, so timestamp ties are broken by UUID and pages can neither duplicate nor skip rows; both variants execute it (Direct as a drift-guarded constant, Codegen via generated sqlc code). The service layer is line-for-line identical in both variants: default limit 20, accepted range 1–100 (else 400), fetch `limit+1` to detect a further page, truncate to `limit`, and derive `nextCursor` from the last returned item. The cursor is unpadded Base64URL (`base64.RawURLEncoding.Strict()`) over a JSON object `{"createdAt", "id"}`; decoding rejects invalid Base64, unknown JSON members, trailing JSON values, invalid RFC3339 timestamps, and invalid UUIDs with 400. `nextCursor` is set only when a further page exists (never JSON `null`, never an empty string) and is omitted on the final page (`omitempty` string in Direct, `*string` with `omitempty` set only for non-empty in Codegen). The cursor is predicate-based, so deleting rows at or after the cursor cannot invalidate it (confirmed by the `pagination.cursor-after-delete` evaluator case). The HTTP layer treats the cursor as opaque in both variants; internal cursor fields are not exposed as query parameters. Limit parsing was compared: Direct rejects repeated `limit`/`cursor` parameters explicitly; the oapi-codegen runtime (v1.7.0, read in source) equally rejects multiple values for a single-value parameter, and both sides parse integers via `strconv` with identical laxness, so no observable divergence was found.

### Evaluator-fit and hardcoding checks

No solution contains hardcoded responses for known evaluator inputs, evaluator-specific environment checks, or fixed identifiers other than the all-zero UUID used as an inert sentinel bound to the unused branch of the pagination predicate. The only environment variables read are the contract-mandated `DATABASE_URL` and `HTTP_ADDR`. No inline SQL replaces or alters any canonical operation (see REVIEW-002 for the one auxiliary read-only query). Baseline behavior (create/get/list/delete, strict decoding, Problem Details values per `tasks/problem-details.md`) is unchanged except where the task requires it.

## Pair Equivalence

| Pair | Equivalent | Notes |
|---|---|---|
| Nullable PATCH | yes | Same three-state mapping, same canonical SQL, same validation outcomes and rendering; timestamp parse paths empirically identical on Go 1.26. |
| Optimistic Locking | yes | Identical `parseIfMatch` implementations, identical canonical atomic UPDATE, identical ETag formatting and error precedence. |
| Cursor Pagination | yes | Service pagination/cursor code is textually identical across the pair; identical canonical SQL; query-parameter parsing verified equivalent including repeated-parameter rejection. |

Inherited baseline note (applies to all three pairs equally, not introduced by these solutions): for exotic UUID path-parameter spellings outside the evaluator contract (32-hex without hyphens, `urn:uuid:` prefix, braced form), the Codegen variants accept and canonicalize via the generated `uuid.Parse` binding while the Direct variants reject with 400 (`validUUID` requires the 8-4-4-4-12 form). This divergence already exists between `base2-direct` and `base2-codegen`, is outside every task's requirements and all evaluator cases, and was accepted with the baseline; it is recorded here for completeness only.

## Treatment Constraints

| Check | Result | Notes |
|---|---|---|
| Direct solutions contain no generator usage or generated implementation | confirmed | No `oapi-codegen.yaml`, `sqlc.yaml`, generator commands, or generated files in any `-direct` tree; Makefiles are byte-identical to `base2-direct` (no generate targets); HTTP types, routing, strict decoding, and repository wrappers are handwritten and idiomatic, with no traces of copied generated code. |
| Codegen generated files regenerate byte-for-byte | confirmed | `make verify-task-solutions` regenerated all three Codegen solutions inside the pinned read-only tool image and compared every generated file byte-for-byte ("generated files are canonical"). |
| Codegen generated files contain no manual semantic edits | confirmed | Byte-for-byte regeneration from committed configs and formal inputs excludes manual edits; the diffs of `generated.gen.go`, `models.go`, `querier.go`, and `tasks.sql.go` against Base 2 are consistent with generator output for the changed OpenAPI/SQL inputs. Business logic lives in handwritten adapters/services; the nullable solution's `nullable-type: true` is a configuration change, not a generated-file edit. |
| Formal inputs match within each Direct/Codegen pair | confirmed | `diff` verified `api/openapi.yaml`, `db/migrations/` (recursive), and `db/queries/tasks.sql` are byte-identical within each pair; `make validate-task-targets` additionally proved both trees equal the canonical formal patch applied to Base 2. Shared direct dependencies (Go, chi v5.3.1, pgx v5.10.0) are aligned within each pair; Codegen adds only generator/runtime dependencies (`oapi-codegen/runtime v1.7.0`, `google/uuid v1.6.0`, plus `oapi-codegen/nullable v1.2.0` for the nullable task, matching the recorded compatibility decision and the Task A resolution in `experiment-decisions.md`). The `golang.org/x/sync`/`x/text` indirect version skew between treatments is inherited unchanged from the Base 2 fixtures and is behavior-neutral, but it sits in tension with the authoritative Base 2 sharing rule — see REVIEW-004. |

## Findings

```text
ID: REVIEW-001
Severity: non-blocking
Solution: nullable-patch-codegen
File: internal/httpadapter/handler_test.go
Line: 33-35
Requirement: candidate-authored tests verify the core task semantics (checklist §9)
Observation: the solution's only test file carries the unchanged Base 2 tests (unknown field on create, malformed path, timestamp formatting); Patch exists only as an interface stub, so no candidate-authored test exercises the omitted/null/value dueAt distinction. The Direct counterpart tests this at handler and service level, and both other Codegen solutions test their task semantics; this is the single gap. Task semantics are still verified externally by the 24 applicable evaluator cases, and my reading found the implementation correct, so semantic correctness is not in question. The non-blocking classification matches the authoritative evaluation principle in experiment-decisions.md: "Missing candidate-authored tests do not fail complete_success. They are reported as a separate quality finding."
Required change: add adapter-level tests covering PATCH with dueAt omitted, dueAt null, and dueAt set (asserting the task.Patch values passed to the service), plus the title:null rejection.
```

```text
ID: REVIEW-002
Severity: non-blocking
Solution: optimistic-locking-direct
File: internal/repository/postgres/tasks.go
Line: 95
Requirement: canonical SQL is used without semantic changes; no bypassing of canonical SQL via extraneous inline SQL (checklists §9/§10)
Observation: after the atomic conditional UPDATE returns no rows, the repository runs an auxiliary inline query `SELECT EXISTS (SELECT 1 FROM tasks WHERE id = $1)` to classify the failure as 404 versus 412. The query is read-only, does not replace or weaken any canonical operation (the update itself is the required single atomic statement), and cannot cause lost updates. However, it is SQL text outside db/queries/tasks.sql, whereas the Codegen counterpart reuses the canonical GetTask query for the same purpose. The authoritative constraint in experiment-decisions.md forbids T1 from "replac[ing] the operations with unrelated inline SQL"; this auxiliary probe replaces nothing, which confirms the non-blocking classification, but reusing canonical GetTask would remove the question entirely.
Required change: none strictly required; for symmetry and to keep all SQL canonical, replace the EXISTS probe with the existing canonical GetTask query (as the Codegen variant does).
```

```text
ID: REVIEW-003
Severity: non-blocking
Solution: optimistic-locking-direct, optimistic-locking-codegen, cursor-pagination-direct, cursor-pagination-codegen
File: README.md (in each listed solution)
Line: 1
Requirement: scaffold documentation reflects the solution it describes
Observation: all four READMEs are unmodified copies of the Base 2 READMEs (titled "Base 2 Direct"/"Base 2 Codegen" with baseline status text) and do not mention the implemented task. The nullable pair's READMEs were updated; these four were not. Documentation only; no behavioral impact.
Required change: retitle each README to the task it implements and describe the task-specific behavior, mirroring the nullable pair's READMEs.
```

```text
ID: REVIEW-004
Severity: non-blocking
Solution: all six (inherited unchanged from fixtures/base2-direct and fixtures/base2-codegen)
File: go.mod
Line: 14-15 (direct variants) / 17-19 (codegen variants), the golang.org/x indirect requirements
Requirement: experiment-decisions.md requires the Base 2 variants to share "Go, framework, driver, and dependency versions"; fixtures/manifest.md forbids differences in "Go, framework, driver, or shared dependency versions" and requires written justification plus a new review for every additional difference
Observation: the Direct treatment pins golang.org/x/sync v0.17.0 and golang.org/x/text v0.29.0 while the Codegen treatment pins v0.19.0 and v0.32.0 — indirect dependencies present in both variants at different versions, pulled up on the Codegen side by the oapi-codegen runtime. The skew originates in the Base 2 fixtures and is carried unchanged into all three task-solution pairs. It is behavior-neutral for the task contracts (chi, pgx, and Go versions are aligned; the x libraries are transitive utility code), and the reviewer instruction (§8) scopes required alignment to shared Go/framework/database dependencies, which is satisfied. No written justification for the skew was found in the repository.
Required change: before the global freeze, either align the x/sync and x/text versions across treatments at the Base 2 level (and regenerate the task solutions' go.mod/go.sum), or record the written justification that the manifest's allowed-difference policy requires. Resolution belongs at the fixtures level, not in the six task solutions.
```

Per instruction §29: I explicitly confirm that none of these non-blocking findings makes any solution incorrect. REVIEW-001 is a test-coverage gap fully compensated by external evaluator coverage (and classified exactly as the authoritative design prescribes); REVIEW-002 is a stylistic/consistency concern around a read-only auxiliary query that replaces no canonical operation; REVIEW-003 is documentation staleness; REVIEW-004 is a behavior-neutral, fixtures-inherited version-policy gap that needs alignment or written justification before freeze.

## Final Conclusion

All six canonical task solutions are suitable as experiment references: yes

Remaining concerns:

1. The four non-blocking findings above (REVIEW-001..004) are recommended cleanups before the global freeze, ideally before the pilots, since fixture changes after the freeze would require broader revalidation. REVIEW-004 in particular should be resolved (by version alignment or written justification) at the Base 2 fixtures level, where any change also triggers the manifest's re-review requirement.

Resolved during amendment (2026-08-20): the previously reported unavailability of the authoritative design document. `experiment-decisions.md` was supplied by the owner, hash-verified against `config/design-revision.json` (SHA-256 match), and the full re-check confirmed every conclusion of the initial pass. The document is not stored inside the repository; the owner may want to place it at the expected external path (`../experiment-decisions.md` relative to the repository root) so future validation and review sessions can consult it directly.
