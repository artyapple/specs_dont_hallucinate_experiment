# Freeze and Result Semantic Validation

JSON Schema validates artifact shape. A deterministic semantic validator must enforce cross-file invariants that JSON Schema cannot express reliably.

## Configuration Freeze

Reject `status: frozen` unless:

- no string in config or version metadata begins with `TODO`;
- every Git revision resolves to the expected repository object;
- every content SHA-256 matches the referenced task or formal patch;
- every image reference contains a verified digest;
- network-policy status is `validated` and all negative tests passed;
- experiment config, evaluator, result schema, treatments, tasks, and fixtures are clean and committed;
- the schedule seed and schedule-manifest path match the frozen schedule.

## Schedule Freeze

Reject a frozen measured schedule unless:

- it contains exactly 70 entries;
- ordinals are unique and contiguous from 1 through 70;
- run IDs are unique;
- every one of the 14 frozen cells appears exactly five times;
- each `(cellId, repeatIndex)` pair from 1 through 5 appears exactly once;
- every cell ID exists in the frozen experiment matrix;
- seed and config revision match the frozen experiment config;
- ordering satisfies the documented blocked-randomization balance rules.

Pilot schedules are generated and validated separately with one run per cell.

## Run Identity

For every `run-result.json`:

- `cellId` resolves to the frozen matrix;
- stage, task, mode, and treatment exactly match that cell;
- run ID and repeat index match the frozen schedule;
- artifact paths remain inside that run directory;
- artifact hashes in `workspace-manifest.json` match stored files.

## Behavior Case Set

For evaluated runs, `behaviorCases` contains every frozen manifest case exactly once:

- cases with `task: all` are applicable;
- cases matching the run task are applicable;
- all other task-specific cases are inapplicable with `passed: null`;
- applicable cases have Boolean outcomes and evidence;
- duplicate, unknown, or missing IDs are rejected.

For `infrastructure-failure` and `harness-failure`, evaluation is null and no behavior outcomes are fabricated.

## Derived Complete Success

The evaluator derives `completeSuccess`; agents and analysis code do not supply it independently.

```text
completeSuccess =
  every common gate is true
  AND every applicable required behavior case passed
```

The semantic validator recomputes this value and rejects mismatches in either direction.

Codegen health never changes common `completeSuccess`.

## Infrastructure and Replacement Links

- Infrastructure-failure status requires `isFailure: true` and evidence.
- `excluded: true` requires `exclusionEligible: true` and a replacement run ID.
- Replacement run IDs must resolve to the same frozen cell.
- A replacement links back through `replacesRunId`.
- Nonfailed original runs cannot name a replacement.
- Protocol events contain category, timestamp, evidence, and forced-termination status.

`TODO`: Implement this validator as a required pre-freeze and post-run command before pilots.
