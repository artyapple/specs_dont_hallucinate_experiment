#!/usr/bin/env bash
set -euo pipefail

readonly NETWORK="specs-bridge-$RANDOM-$$"
readonly TOOL="${NETWORK}-tool"
readonly COORDINATOR="${NETWORK}-coordinator"
readonly SECRET="synthetic-provider-secret-$RANDOM-$$"
readonly HARNESS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly MODEL="$(jq -r '.model.id' "$HARNESS_DIR/../config/experiment.json")"
readonly TOOL_IMAGE="${TOOL_IMAGE:-specs-experiment-tool-direct:go1.26.6}"
readonly COORDINATOR_IMAGE="${COORDINATOR_IMAGE:-specs-experiment-coordinator:1.18.18}"
readonly WORKSPACE="$(mktemp -d)"
readonly CONFIG="$(jq -c '
  .agent.experiment.permission.bridge_test_read = "allow"
  | .agent.experiment.permission.bridge_test_edit = "allow"
  | .agent.experiment.permission.bridge_test_write = "allow"
  | .agent.experiment.permission.bridge_test_bash = "allow"
  | .agent.experiment.permission.bridge_test_apply_patch = "allow"
' "$HARNESS_DIR/opencode-run.json")"
chmod a+rwx "$WORKSPACE"

cleanup() {
  docker rm -f "$TOOL" "$COORDINATOR" >/dev/null 2>&1 || true
  docker network rm "$NETWORK" >/dev/null 2>&1 || true
  rm -rf "$WORKSPACE"
}
trap cleanup EXIT

fail() {
  printf 'bridge test failed: %s\n' "$1" >&2
  exit 1
}

run_tool() {
  local tool="$1"
  local params="$2"
  docker run --rm \
    --network "$NETWORK" \
    --read-only \
    --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
    --cap-drop ALL \
    --security-opt no-new-privileges \
    -v "$WORKSPACE:/workspace:ro" \
    -e OPENROUTER_API_KEY="$SECRET" \
    -e OPENCODE_MODEL="$MODEL" \
    -e OPENCODE_CONFIG_CONTENT="$CONFIG" \
    -e TOOL_BRIDGE_URL="http://tool:4096" \
    -e TOOL_BRIDGE_TEST_ALIASES=1 \
    -e XDG_DATA_HOME=/tmp/opencode-data \
    -e XDG_CACHE_HOME=/tmp/opencode-cache \
    -e XDG_STATE_HOME=/tmp/opencode-state \
    "$COORDINATOR_IMAGE" debug agent experiment --tool "$tool" --params "$params"
}

docker network create --internal "$NETWORK" >/dev/null

docker run -d \
  --name "$TOOL" \
  --init \
  --hostname tool-executor \
  --network "$NETWORK" \
  --network-alias tool \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --tmpfs /home/candidate/.cache/go-build:rw,nosuid,nodev,uid=10001,gid=10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  -v "$WORKSPACE:/workspace" \
  "$TOOL_IMAGE" >/dev/null

docker run -d \
  --name "$COORDINATOR" \
  --hostname coordinator \
  --network "$NETWORK" \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  -v "$WORKSPACE:/workspace:ro" \
  -e OPENROUTER_API_KEY="$SECRET" \
  -e OPENCODE_MODEL="$MODEL" \
  -e OPENCODE_CONFIG_CONTENT="$CONFIG" \
  -e TOOL_BRIDGE_URL="http://tool:4096" \
  -e TOOL_BRIDGE_TEST_ALIASES=1 \
  -e XDG_DATA_HOME=/tmp/opencode-data \
  -e XDG_CACHE_HOME=/tmp/opencode-cache \
  -e XDG_STATE_HOME=/tmp/opencode-state \
  "$COORDINATOR_IMAGE" serve --hostname 0.0.0.0 --port 4097 >/dev/null

for _ in $(seq 1 30); do
  if docker exec "$TOOL" curl --fail --silent --max-time 2 http://127.0.0.1:4096/healthz >/dev/null 2>&1 \
    && docker exec "$COORDINATOR" curl --fail --silent --max-time 2 http://127.0.0.1:4097/global/health >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

docker inspect --format '{{.State.Running}}' "$COORDINATOR" | grep -Fx true >/dev/null \
  || fail "coordinator failed during startup"

BASH_ARGS="$(jq -nc --arg command \
  'hostname; id -u; pwd; ! env | grep -F synthetic-provider-secret; ! tr "\0" "\n" </proc/1/environ | grep -F synthetic-provider-secret; ! grep -R -F synthetic-provider-secret /workspace /home/candidate 2>/dev/null; printf "alpha\nbeta\n" > evidence.txt; printf "stdout-line\n"; printf "stderr-line\n" >&2; exit 7' \
  '{command:$command}')"
BASH_RESULT="$(run_tool bridge_test_bash "$BASH_ARGS")"
test "$(jq -r '.result.metadata.exit' <<<"$BASH_RESULT")" = "7" || fail "bash exit code was not preserved"
jq -e '.result.output | contains("tool-executor") and contains("10001") and contains("/workspace") and contains("stdout-line") and contains("stderr-line")' \
  >/dev/null <<<"$BASH_RESULT" || fail "bash output or execution identity was not preserved"
! grep -F "$SECRET" <<<"$BASH_RESULT" >/dev/null || fail "coordinator credential leaked into bash output"

READ_RESULT="$(run_tool bridge_test_read '{"filePath":"/workspace/evidence.txt"}')"
jq -e '.result.output | contains("1: alpha") and contains("2: beta")' >/dev/null <<<"$READ_RESULT" \
  || fail "read did not return native line-numbered output"

WRITE_RESULT="$(run_tool bridge_test_write '{"filePath":"/workspace/deepseek.txt","content":"before\n"}')"
jq -e '.result.output | contains("Wrote file successfully")' >/dev/null <<<"$WRITE_RESULT" \
  || fail "write did not preserve native output"
EDIT_RESULT="$(run_tool bridge_test_edit '{"filePath":"/workspace/deepseek.txt","oldString":"before","newString":"after"}')"
jq -e '.result.output | contains("Edit applied successfully")' >/dev/null <<<"$EDIT_RESULT" \
  || fail "edit did not preserve native output"
DEEPSEEK_READ="$(run_tool bridge_test_read '{"filePath":"/workspace/deepseek.txt"}')"
jq -e '.result.output | contains("1: after")' >/dev/null <<<"$DEEPSEEK_READ" \
  || fail "edit/write changes were not visible in the tool workspace"

PATCH_ARGS="$(jq -nc --arg patch $'*** Begin Patch\n*** Update File: evidence.txt\n@@\n-alpha\n+patched\n beta\n*** End Patch' '{patchText:$patch}')"
PATCH_RESULT="$(run_tool bridge_test_apply_patch "$PATCH_ARGS")"
jq -e '.result.output | contains("Success. Updated the following files:") and contains("evidence.txt")' \
  >/dev/null <<<"$PATCH_RESULT" || fail "apply_patch did not preserve native output"
docker exec "$TOOL" bash -c 'read -r line </workspace/evidence.txt; test "$line" = patched' \
  || fail "apply_patch did not update the tool workspace"

if run_tool bridge_test_read '{"filePath":"/etc/passwd"}' >/dev/null 2>&1; then
  fail "read accepted an external path"
fi
if run_tool bridge_test_read '{"filePath":"/workspace/../etc/passwd"}' >/dev/null 2>&1; then
  fail "read accepted path traversal"
fi
if run_tool bridge_test_write '{"filePath":"/workspace/../tmp/escaped.txt","content":"escape"}' >/dev/null 2>&1; then
  fail "write accepted path traversal"
fi
if run_tool bridge_test_edit '{"filePath":"/etc/passwd","oldString":"root","newString":"escape"}' >/dev/null 2>&1; then
  fail "edit accepted an external path"
fi
docker exec "$TOOL" ln -s /etc /workspace/external-link
if run_tool bridge_test_read '{"filePath":"/workspace/external-link/passwd"}' >/dev/null 2>&1; then
  fail "read followed a symlink outside the workspace"
fi
TRAVERSAL_PATCH="$(jq -nc --arg patch $'*** Begin Patch\n*** Add File: ../escaped.txt\n+escape\n*** End Patch' '{patchText:$patch}')"
if run_tool bridge_test_apply_patch "$TRAVERSAL_PATCH" >/dev/null 2>&1; then
  fail "apply_patch accepted path traversal"
fi

TIMEOUT_ARGS="$(jq -nc --arg command 'sleep 30; touch /workspace/timeout-failed' '{command:$command,timeout:300}')"
TIMEOUT_RESULT="$(run_tool bridge_test_bash "$TIMEOUT_ARGS")"
test "$(jq -r '.result.metadata.exit' <<<"$TIMEOUT_RESULT")" = "null" || fail "timeout exit was not null"
jq -e '.result.output | contains("exceeding timeout 300 ms")' >/dev/null <<<"$TIMEOUT_RESULT" \
  || fail "timeout metadata was absent"
sleep 1
docker exec "$TOOL" test ! -e /workspace/timeout-failed || fail "timed-out command survived"

ABORT_BODY="$(jq -nc --arg command 'sleep 30; touch /workspace/abort-failed' \
  '{tool:"bash",args:{command:$command}}')"
docker exec "$COORDINATOR" curl --silent --max-time 1 \
  -H 'content-type: application/json' -d "$ABORT_BODY" http://tool:4096/execute >/dev/null 2>&1 || true
sleep 1
docker exec "$TOOL" test ! -e /workspace/abort-failed || fail "aborted command survived"
docker exec "$TOOL" bash -c 'for process in /proc/[0-9]*/comm; do read -r name <"$process" || true; test "$name" != sleep || exit 1; done' \
  || fail "aborted command process remained alive"

docker exec "$TOOL" bash -c '! env | grep -F synthetic-provider-secret'
docker exec "$TOOL" bash -c '! tr "\0" "\n" </proc/1/environ | grep -F synthetic-provider-secret'
docker exec "$TOOL" test ! -S /var/run/docker.sock

printf 'tool bridge integration test passed\n'
