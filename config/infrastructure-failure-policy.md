# Infrastructure Failure and Replacement Policy

Status: draft. Freeze before measured runs.

## Eligible External Failures

A run may be excluded and replaced only when evidence proves that the failure occurred outside candidate-controlled actions. Candidate compilation, dependency use, commands, and resource consumption are not infrastructure failures.

Candidate categories for exclusion eligibility:

- model-provider outage confirmed independently;
- harness process crash unrelated to candidate commands;
- host or container-runtime failure affecting the run environment;
- evaluator infrastructure failure reproduced on a canonical solution;
- artifact storage failure after a preserved final workspace exists.

## Ineligible Failures

- agent invokes a missing or forbidden command;
- candidate code fails to build or start;
- candidate exhausts the 45-minute limit;
- candidate tests hang or consume resources;
- generator or migration command fails because of candidate edits;
- agent violates treatment rules.

## Replacement Rules

- Preserve the original run and all available evidence.
- Record exclusion eligibility, final exclusion decision, and reviewer of the decision.
- Assign a new run ID to the replacement.
- Link original and replacement IDs in both run-result artifacts.
- Use the same cell and a newly scheduled execution slot.
- Publish both artifacts and the reason for replacement.
