#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly TMP_DIR="$(mktemp -d)"

cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

mkdir -p "$TMP_DIR/internal/httpapi" "$TMP_DIR/internal/repository/generated"
cp -R "$ROOT_DIR/api" "$ROOT_DIR/db" "$TMP_DIR/"
cp "$ROOT_DIR/oapi-codegen.yaml" "$ROOT_DIR/sqlc.yaml" "$TMP_DIR/"

cd "$TMP_DIR"
oapi-codegen --config oapi-codegen.yaml api/openapi.yaml >internal/httpapi/generated.gen.go
sqlc generate -f sqlc.yaml

cmp internal/httpapi/generated.gen.go "$ROOT_DIR/internal/httpapi/generated.gen.go"
for file in db.go models.go querier.go tasks.sql.go; do
  cmp "internal/repository/generated/$file" "$ROOT_DIR/internal/repository/generated/$file"
done

printf 'generated files are canonical\n'
