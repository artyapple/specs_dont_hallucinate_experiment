#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cd "$ROOT_DIR"

for track in nullable-patch optimistic-locking cursor-pagination; do
  target="$TMP_DIR/$track"
  mkdir -p "$target/api" "$target/db"
  cp -R fixtures/base2-direct/api/. "$target/api/"
  cp -R fixtures/base2-direct/db/. "$target/db/"
  patch --silent -p1 -d "$target" <"tasks/propagation/$track/formal.patch"
  diff -ru "$target/api" "fixtures/task-solutions/$track-direct/api"
  diff -ru "$target/db" "fixtures/task-solutions/$track-direct/db"
  diff -ru "$target/api" "fixtures/task-solutions/$track-codegen/api"
  diff -ru "$target/db" "fixtures/task-solutions/$track-codegen/db"
done

go run ./tasks/propagation/validate-targets.go
