#!/usr/bin/env bash
set -euo pipefail

readonly HARNESS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly ROOT_DIR="$(cd "$HARNESS_DIR/.." && pwd)"
# shellcheck source=restricted-egress.sh
. "$HARNESS_DIR/restricted-egress.sh"
readonly ENV_FILE="$ROOT_DIR/.env"
readonly CONFIG_FILE="$ROOT_DIR/config/experiment.json"
readonly RUN_CONFIG_FILE="$HARNESS_DIR/opencode-run.json"
readonly TOOL_IMAGE="${TOOL_IMAGE:-specs-experiment-tool-direct:go1.26.6}"
readonly COORDINATOR_IMAGE="${COORDINATOR_IMAGE:-specs-experiment-coordinator:1.18.18}"
readonly RUN_ID="openrouter-smoke-$(date -u +%Y%m%dT%H%M%SZ)-$$"
readonly TOOL="$RUN_ID-tool"
readonly COORDINATOR="$RUN_ID-coordinator"
readonly WORKSPACE="$(mktemp -d)"
readonly ARTIFACT_DIR="$ROOT_DIR/.cache/openrouter-smoke/$RUN_ID"
readonly MODEL="$(jq -er '.model.id | select(type == "string" and length > 0)' "$CONFIG_FILE")"
readonly RUN_CONFIG="$(jq -c . "$RUN_CONFIG_FILE")"
restricted_egress_names "$RUN_ID"
readonly PROMPT='Perform this exact smoke scenario using tools, in order. Do not skip or simulate any step.
1. Use read on /workspace/input.txt and confirm it contains alpha-17.
2. Use bash with workdir /workspace to run exactly: set -eu; test -z "${OPENROUTER_API_KEY+x}"; ! tr "\0" "\n" </proc/1/environ | grep -q "^OPENROUTER_API_KEY="; printf "bash=ok\ncredential_absent=true\nuid=%s\n" "$(id -u)" > /workspace/bash-evidence.txt
3. Use write to create /workspace/result.txt with exactly these three lines: status=pending, input=alpha-17, bash=ok.
4. Use read on /workspace/result.txt.
5. Use edit to replace status=pending with status=complete in /workspace/result.txt.
6. Use read on /workspace/result.txt again and verify the final content.
After all six tool steps succeed, reply with exactly SMOKE_OK.'

cleanup() {
  docker rm -f "$TOOL" "$COORDINATOR" >/dev/null 2>&1 || true
  restricted_egress_stop
  rm -rf "$WORKSPACE"
}
trap cleanup EXIT

fail() {
  printf 'OpenRouter smoke failed: %s\nArtifacts: %s\n' "$1" "$ARTIFACT_DIR" >&2
  exit 1
}

artifact_contains_key() {
  local file line
  while IFS= read -r -d '' file; do
    while IFS= read -r line || [ -n "$line" ]; do
      case "$line" in
        *"$OPENROUTER_API_KEY"*) return 0 ;;
      esac
    done <"$file"
  done < <(find "$ARTIFACT_DIR" -type f -print0)
  return 1
}

test -f "$ENV_FILE" || fail "$ENV_FILE does not exist"
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a
: "${OPENROUTER_API_KEY:?set OPENROUTER_API_KEY in $ENV_FILE}"

mkdir -p "$ARTIFACT_DIR/workspace"
printf 'alpha-17\n' >"$WORKSPACE/input.txt"
chmod -R a+rwX "$WORKSPACE"

restricted_egress_start
docker run -d \
  --name "$TOOL" \
  --init \
  --hostname tool-executor \
  --network "$TOOL_NETWORK" \
  --network-alias tool \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --tmpfs /home/candidate/.cache/go-build:rw,nosuid,nodev,uid=10001,gid=10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  -v "$WORKSPACE:/workspace" \
  "$TOOL_IMAGE" >/dev/null

for _ in $(seq 1 30); do
  if docker exec "$TOOL" curl --fail --silent --max-time 2 http://127.0.0.1:4096/healthz >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "$TOOL" curl --fail --silent --max-time 2 http://127.0.0.1:4096/healthz >/dev/null \
  || fail "tool bridge did not become healthy"

docker exec "$TOOL" bash -c 'test -z "${OPENROUTER_API_KEY+x}"' \
  || fail "OPENROUTER_API_KEY exists in the tool environment"
docker exec "$TOOL" bash -c '! tr "\0" "\n" </proc/1/environ | grep -q "^OPENROUTER_API_KEY="' \
  || fail "OPENROUTER_API_KEY exists in the tool init environment"

docker create \
  --name "$COORDINATOR" \
  --hostname coordinator \
  --network "$TOOL_NETWORK" \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --add-host "openrouter.ai:$EGRESS_PROXY_IP" \
  -v "$WORKSPACE:/workspace:ro" \
  -e OPENROUTER_API_KEY \
  -e OPENCODE_MODEL="$MODEL" \
  -e OPENCODE_CONFIG_CONTENT="$RUN_CONFIG" \
  -e TOOL_BRIDGE_URL="http://tool:4096" \
  -e XDG_DATA_HOME=/tmp/opencode-data \
  -e XDG_CACHE_HOME=/tmp/opencode-cache \
  -e XDG_STATE_HOME=/tmp/opencode-state \
  "$COORDINATOR_IMAGE" run --agent experiment --format json --dir /workspace "$PROMPT" >/dev/null
docker network connect "$PROVIDER_NETWORK" "$COORDINATOR"
docker start "$COORDINATOR" >/dev/null

deadline=$((SECONDS + 600))
while [ "$(docker inspect --format '{{.State.Running}}' "$COORDINATOR")" = true ]; do
  if [ "$SECONDS" -ge "$deadline" ]; then
    docker stop --time 5 "$COORDINATOR" >/dev/null || true
    docker logs "$COORDINATOR" >"$ARTIFACT_DIR/transcript.jsonl" 2>"$ARTIFACT_DIR/stderr.log" || true
    fail "coordinator exceeded the 600-second smoke timeout"
  fi
  sleep 1
done

docker logs "$COORDINATOR" >"$ARTIFACT_DIR/transcript.jsonl" 2>"$ARTIFACT_DIR/stderr.log" || true
readonly EXIT_CODE="$(docker inspect --format '{{.State.ExitCode}}' "$COORDINATOR")"
cp -R "$WORKSPACE/." "$ARTIFACT_DIR/workspace/"

readonly TOOL_IMAGE_ID="$(docker image inspect "$TOOL_IMAGE" --format '{{.Id}}')"
readonly COORDINATOR_IMAGE_ID="$(docker image inspect "$COORDINATOR_IMAGE" --format '{{.Id}}')"
jq -n \
  --arg runId "$RUN_ID" \
  --arg model "$MODEL" \
  --arg coordinatorImage "$COORDINATOR_IMAGE" \
  --arg coordinatorImageId "$COORDINATOR_IMAGE_ID" \
  --arg toolImage "$TOOL_IMAGE" \
  --arg toolImageId "$TOOL_IMAGE_ID" \
  --argjson exitCode "$EXIT_CODE" \
  '{
    runId: $runId,
    kind: "development-openrouter-smoke",
    countedAsPilot: false,
    countedAsMeasuredRun: false,
    model: $model,
    coordinator: {image: $coordinatorImage, imageId: $coordinatorImageId, networks: ["provider-egress", "internal-tool"]},
    tool: {image: $toolImage, imageId: $toolImageId, networks: ["internal-tool"], uid: 10001},
    providerCredentialInToolEnvironment: false,
    providerCredentialInToolInitEnvironment: false,
    coordinatorExitCode: $exitCode
  }' >"$ARTIFACT_DIR/metadata.json"

if artifact_contains_key; then
  rm -rf "$ARTIFACT_DIR"
  fail "provider credential appeared in artifacts; artifacts were deleted"
fi
unset OPENROUTER_API_KEY

test "$EXIT_CODE" = 0 || fail "coordinator exited with code $EXIT_CODE"
test "$(cat "$WORKSPACE/result.txt")" = $'status=complete\ninput=alpha-17\nbash=ok' \
  || fail "result.txt does not contain the expected final content"
grep -Fx 'credential_absent=true' "$WORKSPACE/bash-evidence.txt" >/dev/null \
  || fail "bash did not record credential isolation"
grep -Fx 'uid=10001' "$WORKSPACE/bash-evidence.txt" >/dev/null \
  || fail "bash did not run as tool UID 10001"
for tool in read bash write edit; do
  jq -e --arg tool "$tool" '.. | objects | select(.tool? == $tool)' "$ARTIFACT_DIR/transcript.jsonl" >/dev/null \
    || fail "$tool tool event is absent from the transcript"
done
grep -F 'SMOKE_OK' "$ARTIFACT_DIR/transcript.jsonl" >/dev/null \
  || fail "model did not return SMOKE_OK"

printf 'OpenRouter smoke passed\nModel: %s\nArtifacts: %s\n' "$MODEL" "$ARTIFACT_DIR"
