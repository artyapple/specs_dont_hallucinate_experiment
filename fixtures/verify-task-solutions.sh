#!/usr/bin/env bash
set -euo pipefail

readonly FIXTURES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly CODEGEN_IMAGE="${CODEGEN_IMAGE:-specs-export/tool-codegen:go1.26.6}"

for candidate in "$FIXTURES_DIR"/task-solutions/*-direct "$FIXTURES_DIR"/task-solutions/*-codegen; do
  (
    cd "$candidate"
    go mod verify
    go test ./...
    go vet ./...
  )
done

for candidate in "$FIXTURES_DIR"/task-solutions/*-codegen; do
  docker run --rm \
    --read-only \
    --tmpfs /tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001 \
    --cap-drop ALL \
    --security-opt no-new-privileges \
    --entrypoint bash \
    -v "$candidate:/workspace:ro" \
    "$CODEGEN_IMAGE" -c 'make verify-generate'
done

printf 'canonical task solutions verified\n'
