#!/usr/bin/env bash
# Runs the frozen evaluator checks from the evaluator OCI image against
# canonical fixtures: baseline and all three task-specific families. The image
# executes candidate code, so the checks also prove provider credentials are
# absent from the image environment. Requires Docker with the pinned
# PostgreSQL image and testcontainers/ryuk available to the daemon.
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly IMAGE="${EVALUATOR_IMAGE:-specs-experiment-evaluator:go1.26.6}"
readonly SOCKET="${DOCKER_SOCKET:-/var/run/docker.sock}"

# The evaluator runs as unprivileged uid 10001. Access to the Docker socket is
# granted through a supplementary group only: the socket group on the daemon
# host (Linux) or root:root on Docker Desktop, where the VM mounts the socket
# with gid 0. Root itself is never used.
case "$(uname -s)" in
  Darwin) SOCKET_GID="${SOCKET_GID:-0}" ;;
  *)      SOCKET_GID="${SOCKET_GID:-$(stat -c %g "$SOCKET")}" ;;
esac
readonly SOCKET_GID

fail() {
  printf 'evaluator image test FAILURE: %s\n' "$1" >&2
  exit 1
}

# 1. The evaluator image must never carry provider credentials.
docker run --rm --entrypoint sh "$IMAGE" -c 'test -z "${OPENROUTER_API_KEY+x}"' \
  || fail "image environment contains OPENROUTER_API_KEY"
printf 'ok: no provider credentials in image environment\n'

# 2. The evaluator runs as an unprivileged user.
uid="$(docker run --rm --entrypoint id "$IMAGE" -u)"
[ "$uid" = "10001" ] || fail "evaluator runs as uid $uid, want 10001"
printf 'ok: unprivileged user (uid %s)\n' "$uid"

# 3. Candidate builds are hermetic: no module downloads are possible.
docker run --rm --entrypoint sh "$IMAGE" -c 'test "$GOPROXY" = off && test "$GOTOOLCHAIN" = local' \
  || fail "module downloads are not disabled in the image"
printf 'ok: offline module policy\n'

# 4. Baseline and task-specific checks from the image.
run_case() { # $1 task, $2 candidate source
  local task="$1" src="$2" dir
  dir="$(mktemp -d)"
  trap 'rm -rf "$dir"' RETURN
  cp -R "$src" "$dir/candidate"
  chmod -R a+rX "$dir"
  if ! docker run --rm \
    --group-add "$SOCKET_GID" \
    -v "$SOCKET":/var/run/docker.sock \
    -v "$dir/candidate":/candidate:ro \
    "$IMAGE" -task "$task" -candidate /candidate >"$dir/result.json"; then
    fail "evaluator run failed for $task ($src)"
  fi
  jq -e '.completeSuccess == true' "$dir/result.json" >/dev/null \
    || fail "completeSuccess is not true for $task ($src)"
  printf 'ok: %s (%s)\n' "$task" "$(basename "$src")"
}

run_case baseline-service "$ROOT/fixtures/base2-direct"
run_case baseline-service "$ROOT/fixtures/base2-codegen"
run_case nullable-patch "$ROOT/fixtures/task-solutions/nullable-patch-direct"
run_case optimistic-locking "$ROOT/fixtures/task-solutions/optimistic-locking-direct"
run_case cursor-pagination "$ROOT/fixtures/task-solutions/cursor-pagination-direct"

printf 'evaluator image checks passed\n'
