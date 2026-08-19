# Global Freeze Readiness

Status: prepared draft. This document defines how final values are produced; it does not execute the global freeze.

## Revision Rule

Task 12 creates one clean freeze-input commit after pilots and revalidation. Every Git revision in `config/experiment.json`, `config/schedule.json`, and `evaluator/case-manifest.json` identifies that commit. A following manifest-only commit records those values and changes statuses to `frozen`. This two-commit protocol avoids a self-referential commit hash.

`frozenInputs.designRevision` identifies the freeze-input commit containing `config/design-revision.json`. That manifest is the repository-owned, content-addressed link to the authoritative `../experiment-decisions.md`; Task 12 must recompute its SHA-256 after the final design edit.

Task content hashes are lowercase SHA-256 of raw file bytes. Their key-to-path mapping is implemented by `harness/freezecheck/config.go` and checked by `make validate-config` when status is `frozen`.

## Final Values

| Value | Owner | Preparation or verification | Finalization task |
|---|---|---|---|
| Config, design, evaluator, result-schema, fixture, and treatment revisions | Freeze operator | `git status --short` and `git rev-parse HEAD` on the clean freeze-input commit; `make validate-config` after recording | Task 12 |
| Ten task content hashes | Freeze operator | `make validate-config` recomputes raw-byte SHA-256 in frozen mode | Task 12 |
| Four OCI image digests and archive SHA-256 values | Independent OCI validator | `./images/prepare-oci-bundle.sh <external-directory>`, then the Task 6 clean-machine protocol in `independent-oci-validation.md` | Tasks 6 and 12 |
| Schedule schema and blocked-randomization algorithm | Harness owner | `make test-freezecheck` | Task 5 |
| Measured seed and 70-run schedule | Freeze operator | `make generate-schedule PHASE=measured SEED=... REVISION=... GENERATED_AT=... OUTPUT=...` followed by `make validate-schedule PHASE=measured SCHEDULE=...` | Task 12 |
| Failure, protocol, evaluator, run-result, and metric-extraction policies | Harness and evaluator owners | `make validate-config`, `make test-evaluator`, `make test-runresult`, and independent document review after pilots | Tasks 11 and 12 |
| Analysis rules and optional review protocol | Analysis and review owners | Commands remain owned by Tasks 15 and optional review preparation; Task 12 must not freeze until their pre-measurement implementations pass | Tasks 12 and 15 |

The committed `config/schedule.json` remains an empty draft with `TODO_FREEZE_BEFORE_MEASURED_RUNS` until Task 12. `networkPolicyEnforcementStatus` is `validated` by the completed Task 6 separate-clean-machine evidence.

Task 6 followed the required order: a clean source revision produced the external selected bundle and backup; access handoff contained only hostname, SSH user, public-key method, and non-secret machine metadata; evidence was copied back and verified before the owner manually deleted the disposable server.

## Definition Of Ready

| Definition of Ready item | Owner | Verification command | Current dependency |
|---|---|---|---|
| Base 1 fixed; both treatment setups valid | Fixture owner | `make validate-formal test-base1-skeleton` | Automated check available |
| Both Base 2 variants pass one evaluator | Fixture and evaluator owners | `make test-base2-direct test-base2-codegen evaluate-bases` | Automated check available |
| Three full-workflow task texts frozen | Task owner | `make validate-config` in frozen mode | Task 12 records revisions and hashes |
| Three propagation patches frozen | Task owner | `make validate-task-targets`; `make validate-config` in frozen mode | Task 12 records revisions and hashes |
| Nullable request distinguishes omitted/null/value | Compatibility owner | `make test-nullable-compatibility` | Automated check available |
| Generated output regenerates byte-for-byte | Fixture owner | `make test-base2-codegen verify-task-solutions` | Automated check available |
| Evaluator passes canonical correct solutions | Evaluator owner | `make evaluate-task-solutions` | Automated check available; human review remains Task 8 |
| Evaluator rejects every important known-broken class | Evaluator and harness owners | `make test-evaluator test-runresult-integration` | Automated mutation coverage completed in Task 9 |
| Result schema and extraction work end-to-end | Harness owner | `make test-runresult test-runresult-integration test-freezecheck` | Automated checks available |
| All 14 unmeasured pilots complete | Pilot operator | `make validate-results RESULTS_DIR=pilots SCHEDULE=<pilot-schedule>` | Run driver Task 7 and pilots Task 10; no pilots in Task 5 |
| Pilot artifacts stay outside measured results | Pilot operator | `make validate-results RESULTS_DIR=pilots SCHEDULE=<pilot-schedule>` and independent directory review | Task 10 |
| Blocked randomized schedule generated and frozen | Freeze operator | `make validate-schedule PHASE=measured SCHEDULE=config/schedule.json` | Seed and final schedule remain Task 12 |
| Every run automatically preserves all required artifacts | Harness owner | `make test-task7-dry-run`; `make validate-results RESULTS_DIR=<dry-run-results> SCHEDULE=<schedule>` | Synthetic finalization proof passes; real agent execution remains a pre-pilot dependency |

Commands whose dependencies are not complete are assigned now but are not claimed as passing. Task 5 does not supersede Tasks 6-12.
