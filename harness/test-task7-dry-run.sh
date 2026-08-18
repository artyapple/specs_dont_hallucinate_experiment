#!/usr/bin/env bash
# Synthetic end-to-end finalization proof. This script never invokes OpenCode
# or a model provider; submitted outcomes still come from the real evaluator.
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly OUT="$ROOT/results/task7-dry-run"
readonly DRIVER="$ROOT/bin/rundriver"
readonly FREEZECHECK="$ROOT/bin/freezecheck"
readonly TRANSCRIPT="$ROOT/harness/testdata/task7/transcript.jsonl"
readonly EMPTY_PATCH="$ROOT/harness/testdata/task7/empty.patch"
readonly START="2026-08-18T00:00:00Z"

for binary in "$DRIVER" "$ROOT/bin/runresult" "$ROOT/bin/evaluator" "$FREEZECHECK" "$ROOT/bin/analysis-input"; do
  if [ ! -x "$binary" ]; then
    printf 'missing required binary %s\n' "$binary" >&2
    exit 1
  fi
done

rm -rf "$OUT"
mkdir -p "$OUT"

fail() {
  printf 'Task 7 dry-run FAILURE: %s\n' "$1" >&2
  exit 1
}

run_driver() {
  "$DRIVER" \
    -root "$ROOT" \
    -run-dir "$OUT/$1" \
    -run-id "task7-$1" \
    -cell-id greenfield-direct \
    -repeat-index 1 \
    -phase pilot \
    -status "$2" \
    -workspace-source "$ROOT/fixtures/base2-direct" \
    -transcript-source "$TRANSCRIPT" \
    -patch-source "$3" \
    -started-at "$START" \
    -finished-at "$4" \
    "${@:5}"
}

run_driver submitted-success submitted "$EMPTY_PATCH" "2026-08-18T00:10:00Z"
run_driver timed-out timed-out "$EMPTY_PATCH" "2026-08-18T00:45:00Z"
run_driver candidate-failure submitted "$ROOT/harness/testdata/task7/broken.patch" "2026-08-18T00:10:00Z" \
  -workspace-overlay "$ROOT/harness/testdata/task7/broken-overlay"
run_driver infrastructure-failure infrastructure-failure "$EMPTY_PATCH" "2026-08-18T00:01:00Z" \
  -infrastructure-category model-provider-outage \
  -infrastructure-evidence "synthetic provider outage before agent start" \
  -evaluator "$OUT/evaluator-must-not-be-invoked"

for name in submitted-success timed-out candidate-failure infrastructure-failure; do
  "$FREEZECHECK" run -root "$ROOT" -run-dir "$OUT/$name"
  for artifact in metadata.json transcript.jsonl final.patch commands.json evaluation.json workspace-manifest.json run-result.json; do
    [ -f "$OUT/$name/$artifact" ] || fail "$name lacks $artifact"
  done
  [ -d "$OUT/$name/workspace" ] || fail "$name lacks preserved workspace"
  [ -f "$OUT/$name/commands/0000.stdout.txt" ] || fail "$name lacks command output captures"
done

"$FREEZECHECK" results -root "$ROOT" -results-dir "$OUT"
"$ROOT/bin/analysis-input" -root "$ROOT" -results-dir "$OUT" -output "$OUT/analysis-input.json"

jq -e '.status == "submitted" and .evaluation.completeSuccess == true' "$OUT/submitted-success/run-result.json" >/dev/null || fail "submitted success result"
jq -e '.status == "timed-out" and .timing.wallClockMilliseconds == 2700000 and .evaluation != null' "$OUT/timed-out/run-result.json" >/dev/null || fail "timed-out result"
jq -e '.status == "submitted" and .evaluation.completeSuccess == false and .evaluation.commonGates.migrations == false and .infrastructure.isFailure == false' "$OUT/candidate-failure/run-result.json" >/dev/null || fail "candidate failure classification"
jq -e '.status == "infrastructure-failure" and .evaluation == null and .infrastructure.isFailure == true and .infrastructure.excluded == false' "$OUT/infrastructure-failure/run-result.json" >/dev/null || fail "infrastructure failure classification"
jq -e '.runs | length == 4 and map(.runId) == (map(.runId) | sort) and (map(select(.candidateFailure)) | length) == 1' "$OUT/analysis-input.json" >/dev/null || fail "analysis input index"

printf 'Task 7 synthetic dry-run checks passed; artifacts: %s\n' "$OUT"
