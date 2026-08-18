# Experiment Completion Roadmap

Status: active checklist. The experiment remains `draft` until Task 12 is complete.

Update this file in the same commit as completed work. Change `[ ]` to `[x]` only after every acceptance criterion for the task passes, and add dated evidence with relevant commit hashes, artifact paths, or command results.

## Infrastructure And Evaluator Completion

### 1. Task-Specific Evaluator Cases

- [x] Implement evaluator cases for Nullable PATCH.
- [x] Implement evaluator cases for Optimistic Locking.
- [x] Implement evaluator cases for Cursor Pagination.
- [x] Emit a machine-readable result for every applicable case ID.
- [x] Pass every case against the corresponding canonical correct implementations.
- [x] Reject known-broken implementations for every important behavior class.

Evidence (2026-08-17): task-aware registry implements all 44 manifest IDs; `nullable.unknown-task` was added by explicit decision; non-applicable cases encode `passed: null`; baseline Direct and Codegen still pass. All six draft canonical task references pass: Nullable 24/24 applicable cases per treatment, Locking 20/20, and Pagination 20/20. Production-case known-broken tests reject nullable omitted/null confusion, locking concurrent winners, pagination duplicates, baseline ordering/shape/Problem Details defects, and HTTP/database inconsistency. Complete per-class mutation coverage remains tracked separately in Task 9.

### 2. Complete `run-result.json` Integration

- [x] Make the harness invoke the evaluator after submission or timeout.
- [x] Merge evaluator cases and common gates into the complete run result.
- [x] Record timing, usage, process, protocol, infrastructure, and artifact metadata.
- [x] Distinguish candidate failure, harness failure, and external infrastructure failure.
- [x] Validate every produced result against `schemas/run-result.schema.json`.

Evidence (2026-08-17): `harness/runresult/` implements the assembler (`bin/runresult`, built by `make build-runresult`). For `submitted` and `timed-out` metadata statuses it invokes the evaluator on the preserved workspace and embeds the full 44-case roster with the nine derived common gates; draft derivation and extraction rules are documented in `harness/README.md`. Candidate build/migration/start failures keep the run status with failed gates; evaluator exit 2, unparseable output, and roster mismatches classify as `harness-failure`; evaluator PostgreSQL setup failure classifies as `infrastructure-failure` with `evaluation: null` and no fabricated outcomes; the assembler never pre-decides exclusion. Codegen health regenerates derived code in the pinned tool image with harness-owned commands. Transcript parsing produces `usage` and draft `commands.json`; `final.patch` classification, `candidateTests`, `residualFailures`, and `workspace-manifest.json` are recorded. Every produced result is validated against `schemas/run-result.schema.json` (draft 2020-12, format assertions) before writing. Passing checks: `go test -race ./...`, `go vet ./...`, `go mod verify` in `harness/runresult`, and `make test-runresult-integration` covering a greenfield Direct pass, a greenfield Codegen pass with healthy codegen metrics, a candidate migration failure, and both driver-classified failure statuses against canonical fixtures as synthetic workspaces (no agent run). Run-driver orchestration that creates workspaces and finalizes `metadata.json` remains in Task 7.

### 3. Stable Evaluator Binary Contract

- [x] Freeze required CLI flags and environment variables.
- [x] Freeze candidate input and artifact output paths.
- [x] Freeze task, stage, and treatment selection behavior.
- [x] Freeze exit codes, timeout handling, diagnostics, and log limits.
- [x] Freeze signal handling and cleanup behavior.
- [x] Freeze applicable and non-applicable case representation.
- [x] Document the final contract and cover it with tests.

Evidence (2026-08-17): `evaluator/contract.md` defines the stable draft contract. CLI: required `-candidate` (existing directory) and `-task` (four task values), optional `-output` (existing parent directory or `-`), positional arguments rejected. Environment: no evaluator-specific variables; candidate commands inherit the evaluator environment plus exactly `DATABASE_URL` (build/migrate/run) and `HTTP_ADDR` (run only). The evaluator is stage- and treatment-agnostic; the harness maps cells to tasks and owns Codegen health. Timeouts are frozen constants: 60 s PostgreSQL startup wait, 30 s readiness deadline, 5 s per HTTP request, 5 minutes per candidate build/migration command, and a 15 minute overall evaluation budget enforced by the binary; the harness assembly deadline is 16 minutes so a healthy evaluator always finishes first. Diagnostics are bounded: 16 KiB tails for command output and service logs with explicit truncation markers, 8 KiB response bodies in case evidence, 64 KiB buffered decode limit; result files are written atomically. Abort on SIGINT/SIGTERM or budget exhaustion kills candidate command process groups (Setpgid plus group SIGKILL and WaitDelay), stops the service process group, terminates PostgreSQL, and exits 2 without writing a result. Case representation frozen as the full 44-case roster in registry order with Boolean or null `passed` and string or empty `evidence`. Contract tests: `cmd/evaluator/main_test.go` covers seven CLI rejection cases (exit 2) and SIGTERM abort (exit 2, prompt exit, no result or temp file); `internal/evaluator/contract_test.go` covers roster order and null representation and bounded setup evidence against Docker. Passing checks: evaluator `go test -race ./...`, `go vet ./...`, `go mod verify`; full canonical matrix re-verified (`make evaluate-task-solutions`, all six references completeSuccess true); `make test-runresult-integration` still passes end to end. The experiment status remains `draft`; the contract freezes fully only at the global freeze.

Evidence amendment (2026-08-18): the documented standard Testcontainers environment now records the digest-pinned Ryuk cleanup image used by the evaluator OCI runtime. No evaluator CLI, candidate environment, timeout, result, exit-code, or cleanup behavior changed.

### 4. Evaluator OCI Image

- [x] Add `images/evaluator.Dockerfile` only after Task 3 is complete.
- [x] Build the evaluator binary in a pinned environment.
- [x] Run as an unprivileged user with the minimum required mounts and access.
- [x] Keep provider credentials out of the evaluator environment.
- [x] Run baseline and task-specific evaluator checks from the image.
- [x] Produce a reproducible OCI archive and record its identities.

Evidence (2026-08-17): `images/evaluator.Dockerfile` builds the evaluator from the pinned `golang:1.26.6-bookworm` base (`CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`) and preloads the union of fixture module dependencies (`base1`, `base2-direct`, `base2-codegen`, `nullable-patch-codegen`) so candidate builds are hermetic under `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`. The image runs as uid 10001 with no provider credentials; the candidate tree mounts read-only at `/candidate`, and Docker socket access is granted only through a supplementary group (daemon socket group on Linux, gid 0 on Docker Desktop) — root is never used. Testcontainers for Go v0.44.0 detects the in-container environment and reaches PostgreSQL through the bridge gateway. Credential hygiene is enforced in code and tested: the evaluator strips `OPENROUTER_API_KEY` from every candidate command environment (`TestCandidateEnvironmentRedactsProviderKey` proves it through the real `runCommand` path), and the run-result assembler strips provider keys from the evaluator process environment (`TestWithoutProviderCredentials`). `make test-evaluator-image` passes: credential absence, uid 10001, offline module policy, and full evaluations from the image for baseline (`base2-direct`, `base2-codegen`), `nullable-patch`, `optimistic-locking`, and `cursor-pagination` canonical references, all `completeSuccess: true`. `images/export-oci.sh` now exports all four images; the development export in `.cache/oci-export/oci-export-20260817T144946Z-11647/manifest.json` records evaluator OCI image digest `sha256:f6dd3ad3c91760eba6c214fb4b5be3f6ed715e8fdcbfcb8514bb581573a6959b` and archive SHA-256 `1152fd4df55e32acd35d5047a8d71b7cbc9d8a79c718aeaad6b062cb7c19fe42`, both reproduced byte-for-byte by an independent second build (coordinator and tool digests also matched the previous session). The local development image ID is recorded in `config/versions.json` `observedLocalImages`; `frozen.evaluatorImage` stays `TODO_PIN_DIGEST` until the global freeze, and clean-machine import validation remains in Task 6.

Evidence amendment (2026-08-18): Task 6 preparation pinned the required Ryuk `0.14.0` host image by digest in `config/versions.json` and the evaluator OCI environment, and aligned the nullable preload dependency with selected `v1.2.0`. A fresh local evaluator image build and all eight canonical image evaluations (both baseline fixtures and all six task references) pass; the previous development evaluator digest is superseded and must not be used as a freeze identity.

### 5. Global Freeze Readiness

- [x] Resolve all remaining `TODO` items required before pilots or measured runs.
- [x] Prepare final revision, digest, schedule, schema, and policy values.
- [x] Confirm every Definition of Ready item has an owner and verification command.
- [x] Do not change experiment status to `frozen` in this task.
- [x] Do not start measured runs in this task.

Evidence (2026-08-18): `harness/freezecheck/` implements deterministic draft/frozen config validation, measured and pilot schedule generation/validation, one-run semantic validation, and result-set validation with reciprocal replacement links. The schedule contract is `sha256-fisher-yates-v1`: five measured blocks (one pilot block), seven shuffled task/mode strata per block, and adjacent randomized Direct/Codegen pairs; generation requires an explicit seed, config revision, timestamp, and non-existing output. Unit and tampering tests pass under `go test -race ./...`; `go vet ./...`, `go mod verify`, `make test-freezecheck`, `make validate-config`, and independent generation/validation of a 70-entry test-seed schedule pass. `config/freeze-readiness.md` fixes the two-commit revision protocol, raw-byte task hashes, final-value ownership, commands, and an owner/verification matrix for every Definition of Ready item. `config/design-revision.json` is a content-addressed repository link to the authoritative design; its SHA-256 is verified by `validate-config`. The authoritative nullable, OpenCode executable, and exact-version decisions are resolved with repository evidence; the stale Base 2 pre-pilot placeholder is now an explicit pilot-fixed draft. The Nullable Codegen reference now uses selected `oapi-codegen/nullable v1.2.0`; `make test-nullable-compatibility`, `make validate-task-targets`, and `make verify-task-solutions` pass. Final OCI/archive identities and clean-machine evidence remain exclusively Task 6; seed, final revisions/hashes, policy statuses, schedule contents, and all `frozen` status changes remain Task 12. Experiment status is still `draft`, network enforcement is still `unvalidated`, committed measured schedule runs remain empty, and no agent, pilot, or measured run was started.

### 6. Independent OCI Validation

- [ ] Owner manually rents a disposable clean `linux/amd64` server only after the final archives and checksums are ready.
- [ ] Record the provider, region, host identity, OS, architecture, CPU, memory, disk, Docker version, and validation operator; exchange only hostname, SSH user, and public-key-based access details, never passwords or private keys.
- [ ] Preserve final OCI archives outside disposable cache.
- [ ] Record every OCI image digest and archive SHA-256.
- [ ] Import the exact archives on a separate clean machine.
- [ ] Repeat bridge positive and negative checks.
- [ ] Repeat network-policy checks.
- [ ] Confirm provider credentials are absent from tool and evaluator environments.
- [ ] Mark network enforcement `validated` only if the frozen validation protocol passes.
- [ ] Copy all non-secret evidence off the server, verify it locally, and have the owner manually delete the disposable server before completing this task.

Evidence: TODO

## Experiment Execution And Publication

### 7. End-To-End Dry Run

- [ ] Exercise candidate workspace creation through final result production without an agent pilot.
- [ ] Validate submitted, timed-out, candidate-failure, and infrastructure-failure paths.
- [ ] Preserve workspace, transcript, commands, patch, evaluation, and manifest artifacts.
- [ ] Feed the produced result into the analysis input pipeline.

Evidence: TODO

### 8. Canonical Task Solutions

- [ ] Create and human-review Direct and Codegen canonical solutions for Nullable PATCH.
- [ ] Create and human-review Direct and Codegen canonical solutions for Optimistic Locking.
- [ ] Create and human-review Direct and Codegen canonical solutions for Cursor Pagination.
- [x] Preserve treatment constraints and byte-identical formal inputs where required.
- [x] Pass visible checks, task evaluators, and Codegen regeneration checks.

Evidence (2026-08-17): six draft references exist under `fixtures/task-solutions/`; `make validate-task-targets`, `make verify-task-solutions`, and `make evaluate-task-solutions` pass. The three Direct variants contain no generator configs or generated outputs; Codegen regeneration is byte-for-byte canonical in the pinned tool image. The task remains open until the six implementations receive independent human review.

### 9. Known-Broken Coverage Completion

- [ ] Cover nullable omitted, null, and value confusion.
- [ ] Cover malformed and stale ETag behavior.
- [ ] Cover lost-update concurrency failures.
- [ ] Cover unstable pagination, timestamp ties, duplicates, and gaps.
- [ ] Cover HTTP and database inconsistency.
- [ ] Cover generated drift or forbidden generated-file edits where applicable.
- [ ] Record the expected failing case for every known-broken candidate.

Evidence: TODO

### 10. Fourteen Unmeasured Pilots

- [ ] Run exactly one unmeasured pilot for each of the 14 experiment cells.
- [ ] Keep pilot artifacts separate from measured results.
- [ ] Verify prompts, timeouts, tool isolation, caches, evaluator applicability, and cleanup.
- [ ] Do not include pilot outcomes in the measured dataset.

Evidence: TODO

### 11. Post-Pilot Corrections And Revalidation

- [ ] Classify every issue found during pilots.
- [ ] Correct ambiguous tasks, harness defects, evaluator errors, artifact gaps, and cleanup failures.
- [ ] Repeat affected pilots after changes to semantics, evaluator rules, or treatments.
- [ ] Re-run all readiness and reproducibility checks after the final correction.

Evidence: TODO

### 12. Execute Global Freeze

- [ ] Freeze Git revisions for fixtures, treatments, tasks, evaluator, harness, schemas, and analysis.
- [ ] Freeze model, agent, dependency, and image versions.
- [ ] Freeze OCI image digests and archive hashes.
- [ ] Freeze the randomized schedule and seed.
- [ ] Freeze failure, exclusion, replacement, and metric extraction rules.
- [ ] Change experiment status from `draft` to `frozen` only after all prerequisites pass.

Evidence: TODO

### 13. Seventy Measured Runs

- [ ] Execute the frozen blocked-randomized schedule without human intervention.
- [ ] Complete 10 Greenfield measured runs.
- [ ] Complete 60 Part 2 measured runs.
- [ ] Preserve all run artifacts and allow at most two concurrent runs.
- [ ] Never provide hidden evaluator feedback to an agent.

Evidence: TODO

### 14. Hidden Evaluation And Classification

- [ ] Evaluate every submitted or timed-out workspace exactly once.
- [ ] Compute `complete_success` and preserve every per-case result.
- [ ] Classify infrastructure failures and protocol violations under frozen rules.
- [ ] Apply exclusions and replacement runs only under the frozen policy.
- [ ] Do not tune the evaluator against measured candidates.

Evidence: TODO

### 15. Deterministic Analysis

- [ ] Validate all run results before aggregation.
- [ ] Report successful runs out of five for every cell.
- [ ] Report individual points, medians, ranges, process metrics, and failure profiles.
- [ ] Compare Direct and Codegen separately within each task and mode.
- [ ] Do not pool unrelated tasks or create a composite score.
- [ ] Generate deterministic tables and charts from committed analysis code.

Evidence: TODO

### 16. Talk Results And Publication Bundle

- [ ] Prepare the result narrative, charts, representative traces, limitations, and threats to validity.
- [ ] Build the English talk deck around the frozen research question.
- [ ] Publish fixtures, tasks, treatments, harness, evaluator, schemas, and analysis code.
- [ ] Publish cleaned transcripts, final patches, machine-readable results, and infrastructure failures.
- [ ] Verify that no credentials or sensitive local artifacts are present.
- [ ] Run optional human review only after automated evaluation is complete.

Evidence: TODO
