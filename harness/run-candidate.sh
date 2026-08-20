#!/usr/bin/env bash
set -euo pipefail

readonly HARNESS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=restricted-egress.sh
. "$HARNESS_DIR/restricted-egress.sh"

: "${EXPERIMENT_RUN_ID:?}"
: "${EXPERIMENT_INSTANCE_ID:?}"
readonly LABEL_RUN="experiment.run-id=$EXPERIMENT_RUN_ID"
readonly LABEL_INSTANCE="experiment.instance-id=$EXPERIMENT_INSTANCE_ID"
readonly PREFIX="run-${EXPERIMENT_INSTANCE_ID}"
readonly TOOL="${PREFIX}-tool"
readonly COORDINATOR="${PREFIX}-coordinator"
readonly POSTGRES="${PREFIX}-postgres"
readonly MODULE_CACHE="${PREFIX}-module-cache"
restricted_egress_names "$PREFIX"

cleanup() {
  restricted_egress_stop
  restricted_egress_stop_labels "$EXPERIMENT_RUN_ID" "$EXPERIMENT_INSTANCE_ID"
}
on_exit() {
  local code=$?
  cleanup
  trap - EXIT
  case "$code" in
    0|70|71|124|130) exit "$code" ;;
    *) exit 71 ;;
  esac
}
if test "${1:-}" = --cleanup; then
  cleanup
  exit 0
fi

: "${CANDIDATE_WORKSPACE:?}"
: "${CANDIDATE_PROMPT_FILE:?}"
: "${CANDIDATE_TRANSCRIPT:?}"
: "${CANDIDATE_STDERR:?}"
: "${COORDINATOR_IMAGE:?}"
: "${TOOL_IMAGE:?}"
: "${EVALUATOR_IMAGE:?}"
: "${POSTGRES_IMAGE:?}"
: "${OPENCODE_MODEL:?}"
: "${OPENCODE_CONFIG_CONTENT:?}"
: "${OPENROUTER_API_KEY:?}"
exec 2>>"$CANDIDATE_STDERR"
test "${CANDIDATE_TIMEOUT_SECONDS:?}" = 2700
[[ "$COORDINATOR_IMAGE" =~ ^sha256:[0-9a-f]{64}$ ]] || exit 71
[[ "$TOOL_IMAGE" =~ ^sha256:[0-9a-f]{64}$ ]] || exit 71
[[ "$EVALUATOR_IMAGE" =~ ^sha256:[0-9a-f]{64}$ ]] || exit 71
[[ "$POSTGRES_IMAGE" =~ ^[^@]+@sha256:[0-9a-f]{64}$ ]] || exit 71

on_signal() {
  cleanup
  trap - EXIT INT TERM
  exit 130
}
trap on_exit EXIT
trap on_signal INT TERM
cleanup
restricted_egress_start "$LABEL_RUN" "$LABEL_INSTANCE"

# Seed the candidate's read-only module cache from the independently validated
# evaluator image. This performs no network access and receives no credential.
docker volume create --label "$LABEL_RUN" --label "$LABEL_INSTANCE" "$MODULE_CACHE" >/dev/null || exit 71
env -u OPENROUTER_API_KEY docker run --rm \
  --label "$LABEL_RUN" \
  --label "$LABEL_INSTANCE" \
  --network none \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --user 0:0 \
  -v "$MODULE_CACHE:/cache" \
  --entrypoint sh \
  "$EVALUATOR_IMAGE" -c 'cp -R /go/pkg/mod/. /cache/' >/dev/null || exit 71

docker run -d \
  --name "$POSTGRES" \
  --label "$LABEL_RUN" \
  --label "$LABEL_INSTANCE" \
  --hostname postgres \
  --network "$TOOL_NETWORK" \
  --network-alias postgres \
  --tmpfs /var/lib/postgresql/data:rw,nosuid,nodev \
  --security-opt no-new-privileges \
  -e POSTGRES_DB=tasks \
  -e POSTGRES_PASSWORD=visible-check \
  -e POSTGRES_USER=postgres \
  "$POSTGRES_IMAGE" >/dev/null || exit 71
for _ in $(seq 1 60); do
  if docker exec "$POSTGRES" pg_isready -U postgres -d tasks >/dev/null 2>&1; then break; fi
  if test "$(docker inspect --format '{{.State.Running}}' "$POSTGRES" 2>/dev/null || true)" != true; then exit 71; fi
  sleep 1
done
docker exec "$POSTGRES" pg_isready -U postgres -d tasks >/dev/null 2>&1 || exit 71

docker run -d \
  --name "$TOOL" \
  --label "$LABEL_RUN" \
  --label "$LABEL_INSTANCE" \
  --init \
  --hostname tool-executor \
  --network "$TOOL_NETWORK" \
  --network-alias tool \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --tmpfs /home/candidate/.cache/go-build:rw,nosuid,nodev,uid=10001,gid=10001 \
  --user 10001:10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  -e 'DATABASE_URL=postgres://postgres:visible-check@postgres:5432/tasks?sslmode=disable' \
  -v "$MODULE_CACHE:/go/pkg/mod:ro" \
  -v "$CANDIDATE_WORKSPACE:/workspace" \
  "$TOOL_IMAGE" >/dev/null || exit 71

healthy=false
for _ in $(seq 1 30); do
  if docker exec "$TOOL" curl --fail --silent --max-time 2 http://127.0.0.1:4096/healthz >/dev/null 2>&1; then
    healthy=true
    break
  fi
  if test "$(docker inspect --format '{{.State.Running}}' "$TOOL" 2>/dev/null || true)" != true; then
    exit 71
  fi
  sleep 1
done
test "$healthy" = true || exit 71

docker create \
  --name "$COORDINATOR" \
  --label "$LABEL_RUN" \
  --label "$LABEL_INSTANCE" \
  --hostname coordinator \
  --network "$TOOL_NETWORK" \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --user 10001:10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --add-host "openrouter.ai:$EGRESS_PROXY_IP" \
  -v "$CANDIDATE_WORKSPACE:/workspace:ro" \
  -e OPENROUTER_API_KEY \
  -e OPENCODE_MODEL \
  -e OPENCODE_CONFIG_CONTENT \
  -e TOOL_BRIDGE_URL=http://tool:4096 \
  -e XDG_DATA_HOME=/tmp/opencode-data \
  -e XDG_CACHE_HOME=/tmp/opencode-cache \
  -e XDG_STATE_HOME=/tmp/opencode-state \
  "$COORDINATOR_IMAGE" run --agent experiment --format json --dir /workspace "$(<"$CANDIDATE_PROMPT_FILE")" >/dev/null || exit 71
docker network connect "$PROVIDER_NETWORK" "$COORDINATOR" || exit 71
docker start "$COORDINATOR" >/dev/null || exit 71

deadline=$((SECONDS + CANDIDATE_TIMEOUT_SECONDS))
result=0
while test "$(docker inspect --format '{{.State.Running}}' "$COORDINATOR" 2>/dev/null || true)" = true; do
  if test "$(docker inspect --format '{{.State.Running}}' "$TOOL" 2>/dev/null || true)" != true; then
    result=71
    docker stop --time 5 "$COORDINATOR" >/dev/null 2>&1 || true
    break
  fi
  if test "$SECONDS" -ge "$deadline"; then
    result=124
    docker stop --time 5 "$COORDINATOR" >/dev/null 2>&1 || true
    break
  fi
  sleep 1
done
docker logs "$COORDINATOR" >"$CANDIDATE_TRANSCRIPT" 2>>"$CANDIDATE_STDERR" || true
if test "$result" = 0; then
  if test "$(docker inspect --format '{{.State.Running}}' "$TOOL" 2>/dev/null || true)" != true; then result=71; fi
  coordinator_exit="$(docker inspect --format '{{.State.ExitCode}}' "$COORDINATOR" 2>/dev/null || printf 125)"
  if test "$result" = 0 && test "$coordinator_exit" != 0; then result=70; fi
fi
exit "$result"
