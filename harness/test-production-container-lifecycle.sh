#!/usr/bin/env bash
set -euo pipefail

readonly HARNESS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly ROOT="$(cd "$HARNESS_DIR/.." && pwd)"
readonly RUN_ID="prepilot-lifecycle-$(date -u +%Y%m%dT%H%M%SZ)-$$"
readonly INSTANCE_ID="$(printf '%016x' "$RANDOM$RANDOM")"
readonly TEMP="$(mktemp -d)"
readonly WORKSPACE="$TEMP/workspace"
readonly TRANSCRIPT="$TEMP/probe.json"
readonly STDERR="$TEMP/candidate-stderr.log"
readonly COORDINATOR_IMAGE="$(jq -er '.frozenInputs.environmentImages.coordinator' "$ROOT/config/experiment.json")"
readonly TOOL_IMAGE="$(jq -er '.frozenInputs.environmentImages.toolCodegen' "$ROOT/config/experiment.json")"
readonly EVALUATOR_IMAGE="$(jq -er '.frozenInputs.environmentImages.evaluator' "$ROOT/config/experiment.json")"
readonly POSTGRES_IMAGE="$(jq -er '.frozen.postgresImage' "$ROOT/config/versions.json")"
readonly MODEL="$(jq -er '.model.id' "$ROOT/config/experiment.json")"
readonly RUN_CONFIG="$(jq -c . "$HARNESS_DIR/opencode-run.json")"

cleanup() {
  env EXPERIMENT_RUN_ID="$RUN_ID" EXPERIMENT_INSTANCE_ID="$INSTANCE_ID" \
    bash "$HARNESS_DIR/run-candidate.sh" --cleanup >/dev/null 2>&1 || true
  rm -rf "$TEMP"
}
trap cleanup EXIT

cp -R "$ROOT/fixtures/base2-codegen" "$WORKSPACE"
chmod -R a+rwX "$WORKSPACE"
: >"$TRANSCRIPT"
: >"$STDERR"

env -u OPENROUTER_API_KEY \
  EXPERIMENT_ROOT="$ROOT" \
  EXPERIMENT_RUN_ID="$RUN_ID" \
  EXPERIMENT_INSTANCE_ID="$INSTANCE_ID" \
  CANDIDATE_WORKSPACE="$WORKSPACE" \
  CANDIDATE_TRANSCRIPT="$TRANSCRIPT" \
  CANDIDATE_STDERR="$STDERR" \
  COORDINATOR_IMAGE="$COORDINATOR_IMAGE" \
  TOOL_IMAGE="$TOOL_IMAGE" \
  EVALUATOR_IMAGE="$EVALUATOR_IMAGE" \
  POSTGRES_IMAGE="$POSTGRES_IMAGE" \
  OPENCODE_MODEL="$MODEL" \
  OPENCODE_CONFIG_CONTENT="$RUN_CONFIG" \
  CANDIDATE_TIMEOUT_SECONDS=2700 \
  bash "$HARNESS_DIR/run-candidate.sh" --model-free-probe

test "$(cat "$WORKSPACE/production-lifecycle-probe.txt")" = production-lifecycle-probe=passed
jq -e '.result.metadata.exit == 0 and (.result.output | contains("all modules verified"))' "$TRANSCRIPT" >/dev/null

env -u OPENROUTER_API_KEY \
  EXPERIMENT_ROOT="$ROOT" \
  EXPERIMENT_RUN_ID="$RUN_ID" \
  EXPERIMENT_INSTANCE_ID="$INSTANCE_ID" \
  EVALUATOR_IMAGE="$EVALUATOR_IMAGE" \
  bash "$HARNESS_DIR/run-evaluator-container.sh" -task baseline-service -candidate "$WORKSPACE" >"$TEMP/evaluation.json"
jq -e '.completeSuccess == true and .setup.postgres == true and .setup.build == true and .setup.migrations == true and .setup.serviceReady == true' \
  "$TEMP/evaluation.json" >/dev/null

test -z "$(docker ps -aq --filter "label=experiment.run-id=$RUN_ID" --filter "label=experiment.instance-id=$INSTANCE_ID")"
test -z "$(docker network ls -q --filter "label=experiment.run-id=$RUN_ID" --filter "label=experiment.instance-id=$INSTANCE_ID")"
test -z "$(docker volume ls -q --filter "label=experiment.run-id=$RUN_ID" --filter "label=experiment.instance-id=$INSTANCE_ID")"

printf 'production container lifecycle probe passed\n'
