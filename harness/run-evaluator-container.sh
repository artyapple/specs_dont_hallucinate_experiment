#!/usr/bin/env bash
set -euo pipefail

readonly HARNESS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly ROOT="${EXPERIMENT_ROOT:-$(cd "$HARNESS_DIR/.." && pwd)}"
readonly CONFIG="$ROOT/config/experiment.json"
readonly IMAGE="${EVALUATOR_IMAGE:-$(jq -er '.frozenInputs.environmentImages.evaluator' "$CONFIG")}"
: "${EXPERIMENT_RUN_ID:?}"
: "${EXPERIMENT_INSTANCE_ID:?}"
readonly CONTAINER="run-${EXPERIMENT_INSTANCE_ID}-evaluator"
[[ "$IMAGE" =~ ^sha256:[0-9a-f]{64}$ ]] || { printf 'evaluator image must be immutable\n' >&2; exit 2; }

cleanup() {
  docker stop --time 20 "$CONTAINER" >/dev/null 2>&1 || true
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
cleanup

args=("$@")
candidate=""
for ((i=0; i<${#args[@]}; i++)); do
  if test "${args[$i]}" = -candidate && test $((i + 1)) -lt "${#args[@]}"; then
    candidate="${args[$((i + 1))]}"
    args[$((i + 1))]=/candidate
  fi
done
test -n "$candidate" || { printf '%s\n' '-candidate is required' >&2; exit 2; }
candidate="$(cd "$candidate" && pwd)"
socket_gid="$(stat -f '%g' /var/run/docker.sock 2>/dev/null || stat -c '%g' /var/run/docker.sock)"

env -u OPENROUTER_API_KEY docker run --rm \
  --name "$CONTAINER" \
  --label "experiment.run-id=$EXPERIMENT_RUN_ID" \
  --label "experiment.instance-id=$EXPERIMENT_INSTANCE_ID" \
  --group-add "$socket_gid" \
  -v "$candidate:/candidate:ro" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  "$IMAGE" "${args[@]}"
