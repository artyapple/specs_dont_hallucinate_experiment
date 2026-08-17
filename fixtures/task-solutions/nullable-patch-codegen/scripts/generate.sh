#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

oapi-codegen --config oapi-codegen.yaml api/openapi.yaml >internal/httpapi/generated.gen.go
sqlc generate -f sqlc.yaml
