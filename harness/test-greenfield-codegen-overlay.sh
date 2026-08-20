#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly IMAGE="${TOOL_IMAGE:-specs-experiment-tool-codegen:go1.26.6}"
readonly WORKSPACE="$(mktemp -d)"
cleanup() { rm -rf "$WORKSPACE"; }
trap cleanup EXIT

cp -R "$ROOT/fixtures/base1/." "$WORKSPACE/"
cp -R "$ROOT/treatments/codegen/workspace/." "$WORKSPACE/"
chmod -R a+rwX "$WORKSPACE"

docker run --rm \
  --network none \
  --read-only \
  --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --user 10001:10001 \
  -v "$WORKSPACE:/workspace" \
  -w /workspace \
  --entrypoint bash \
  "$IMAGE" -c 'make generate && make verify-generate'

printf 'greenfield Codegen workspace overlay passed\n'
