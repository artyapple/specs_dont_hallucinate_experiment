#!/usr/bin/env bash
# End-to-end run-result assembly checks against canonical fixtures used as
# synthetic preserved workspaces. No agent run, pilot, or measured run is
# exercised. Requires bin/runresult and bin/evaluator plus Docker.
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly OUT="$ROOT/results/runresult-tests"
readonly RUNRESULT="$ROOT/bin/runresult"

for binary in "$RUNRESULT" "$ROOT/bin/evaluator"; do
  if [ ! -x "$binary" ]; then
    printf 'missing %s; run make build-runresult build-evaluator\n' "$binary" >&2
    exit 1
  fi
done

rm -rf "$OUT"
mkdir -p "$OUT"

fail() {
  printf 'runresult integration FAILURE: %s\n' "$1" >&2
  exit 1
}

assert_jq() {
  local file="$1" filter="$2" label="$3"
  jq -e "$filter" "$file" >/dev/null || fail "$label ($filter)"
}

new_run_dir() { # $1 name, $2 cell, $3 status
  local dir="$OUT/$1"
  mkdir -p "$dir"
  cat >"$dir/metadata.json" <<JSON
{
  "runId": "integration-$1",
  "cellId": "$2",
  "repeatIndex": 1,
  "phase": "pilot",
  "status": "$3",
  "startedAt": "2026-08-17T00:00:00Z",
  "finishedAt": "2026-08-17T00:10:00Z",
  "protocolViolations": []
}
JSON
  : >"$dir/transcript.jsonl"
  : >"$dir/final.patch"
  printf '%s\n' "$dir"
}

add_infrastructure_note() { # $1 dir, $2 category
  local tmp
  tmp="$(mktemp)"
  jq --arg category "$2" '.infrastructure = {"category": $category, "evidence": "synthetic integration evidence"}' \
    "$1/metadata.json" >"$tmp"
  mv "$tmp" "$1/metadata.json"
}

# 1. Submitted greenfield Direct candidate: full pass, 44-case roster, no codegen health.
dir="$(new_run_dir greenfield-direct-pass greenfield-direct submitted)"
cp -R "$ROOT/fixtures/base2-direct" "$dir/workspace"
"$RUNRESULT" -run-dir "$dir" -root "$ROOT" >/dev/null
result="$dir/run-result.json"
assert_jq "$result" '.status == "submitted"' "status stays submitted"
assert_jq "$result" '.evaluation.completeSuccess == true' "canonical base passes"
assert_jq "$result" '[.evaluation.commonGates | to_entries[] | .value] | all' "all common gates true"
assert_jq "$result" '(.evaluation.behaviorCases | length) == 44' "full 44-case roster embedded"
assert_jq "$result" '.evaluation.codegenHealth == null' "direct has null codegenHealth"
assert_jq "$result" '.evaluation.candidateTests.present == true' "candidate tests detected"
assert_jq "$result" '.infrastructure.isFailure == false and .evaluation != null' "no infrastructure failure"
assert_jq "$dir/evaluation.json" '(.behaviorCases | length) == 44' "raw evaluator artifact preserved"
printf 'ok: greenfield-direct-pass\n'

# 2. Submitted greenfield Codegen candidate: codegen health object present and healthy.
dir="$(new_run_dir greenfield-codegen-pass greenfield-codegen submitted)"
cp -R "$ROOT/fixtures/base2-codegen" "$dir/workspace"
"$RUNRESULT" -run-dir "$dir" -root "$ROOT" >/dev/null
result="$dir/run-result.json"
assert_jq "$result" '.evaluation.completeSuccess == true' "canonical codegen base passes"
assert_jq "$result" '.evaluation.codegenHealth == {"generationSucceeded":true,"canonical":true,"idempotent":true,"manualEditDetected":false}' "codegen health canonical"
printf 'ok: greenfield-codegen-pass\n'

# 3. Candidate failure: broken migration keeps status submitted and fails gates.
dir="$(new_run_dir candidate-migration-failure greenfield-direct submitted)"
cp -R "$ROOT/fixtures/base2-direct" "$dir/workspace"
printf '\nTHIS IS NOT VALID SQL;\n' >>"$dir/workspace/db/migrations/000001_create_tasks.sql"
"$RUNRESULT" -run-dir "$dir" -root "$ROOT" >/dev/null
result="$dir/run-result.json"
assert_jq "$result" '.status == "submitted"' "candidate failure keeps submitted status"
assert_jq "$result" '.evaluation.completeSuccess == false' "broken migration fails completeSuccess"
assert_jq "$result" '.evaluation.commonGates.build == true' "build gate true"
assert_jq "$result" '.evaluation.commonGates.migrations == false' "migrations gate false"
assert_jq "$result" '.evaluation.commonGates["service-start"] == false' "service-start gate false"
assert_jq "$result" '(.evaluation.residualFailures | length) > 0' "residual failures recorded"
printf 'ok: candidate-migration-failure\n'

# 4. Driver-classified infrastructure failure: no evaluator run, null evaluation.
dir="$(new_run_dir infra-failure-input greenfield-direct infrastructure-failure)"
add_infrastructure_note "$dir" "model-provider-outage"
"$RUNRESULT" -run-dir "$dir" -root "$ROOT" >/dev/null
result="$dir/run-result.json"
assert_jq "$result" '.status == "infrastructure-failure"' "infrastructure status preserved"
assert_jq "$result" '.evaluation == null' "no fabricated evaluation"
assert_jq "$result" '.infrastructure.isFailure == true and .infrastructure.category == "model-provider-outage"' "infrastructure note recorded"
assert_jq "$result" '.infrastructure.excluded == false and .infrastructure.replacementRunId == null' "no premature exclusion"
assert_jq "$dir/evaluation.json" '. == null' "evaluation.json artifact holds null"
printf 'ok: infra-failure-input\n'

# 5. Driver-classified harness failure: null evaluation, schema-valid result.
dir="$(new_run_dir harness-failure-input greenfield-codegen harness-failure)"
add_infrastructure_note "$dir" "harness-process-crash"
"$RUNRESULT" -run-dir "$dir" -root "$ROOT" >/dev/null
result="$dir/run-result.json"
assert_jq "$result" '.status == "harness-failure"' "harness status preserved"
assert_jq "$result" '.evaluation == null' "no fabricated evaluation"
assert_jq "$result" '.infrastructure.isFailure == true and .infrastructure.category == "harness-process-crash"' "harness note recorded"
printf 'ok: harness-failure-input\n'

printf 'runresult integration checks passed\n'
