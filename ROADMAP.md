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

- [ ] Make the harness invoke the evaluator after submission or timeout.
- [ ] Merge evaluator cases and common gates into the complete run result.
- [ ] Record timing, usage, process, protocol, infrastructure, and artifact metadata.
- [ ] Distinguish candidate failure, harness failure, and external infrastructure failure.
- [ ] Validate every produced result against `schemas/run-result.schema.json`.

Evidence: TODO

### 3. Stable Evaluator Binary Contract

- [ ] Freeze required CLI flags and environment variables.
- [ ] Freeze candidate input and artifact output paths.
- [ ] Freeze task, stage, and treatment selection behavior.
- [ ] Freeze exit codes, timeout handling, diagnostics, and log limits.
- [ ] Freeze signal handling and cleanup behavior.
- [ ] Freeze applicable and non-applicable case representation.
- [ ] Document the final contract and cover it with tests.

Evidence: TODO

### 4. Evaluator OCI Image

- [ ] Add `images/evaluator.Dockerfile` only after Task 3 is complete.
- [ ] Build the evaluator binary in a pinned environment.
- [ ] Run as an unprivileged user with the minimum required mounts and access.
- [ ] Keep provider credentials out of the evaluator environment.
- [ ] Run baseline and task-specific evaluator checks from the image.
- [ ] Produce a reproducible OCI archive and record its identities.

Evidence: TODO

### 5. Global Freeze Readiness

- [ ] Resolve all remaining `TODO` items required before pilots or measured runs.
- [ ] Prepare final revision, digest, schedule, schema, and policy values.
- [ ] Confirm every Definition of Ready item has an owner and verification command.
- [ ] Do not change experiment status to `frozen` in this task.
- [ ] Do not start measured runs in this task.

Evidence: TODO

### 6. Independent OCI Validation

- [ ] Preserve final OCI archives outside disposable cache.
- [ ] Record every OCI image digest and archive SHA-256.
- [ ] Import the exact archives on a separate clean machine.
- [ ] Repeat bridge positive and negative checks.
- [ ] Repeat network-policy checks.
- [ ] Confirm provider credentials are absent from tool and evaluator environments.
- [ ] Mark network enforcement `validated` only if the frozen validation protocol passes.

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
