#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum --check formal.sha256
else
  shasum -a 256 --check formal.sha256
fi

migrations=()
for file in db/migrations/*.sql; do
  name="${file##*/}"
  test -f "$file" && [[ "$name" =~ ^[0-9]{6}_[a-z0-9_]+\.sql$ ]] && migrations+=("$file")
done
test "${#migrations[@]}" -gt 0 || { printf 'db/migrations contains no NNNNNN_name.sql migration\n' >&2; exit 1; }
expected="$(printf '%s\n' api/openapi.yaml db/queries/tasks.sql "${migrations[@]}" | sort)"
actual="$(while read -r digest path extra; do test -z "${extra:-}"; [[ "$digest" =~ ^[0-9a-fA-F]{64}$ ]]; printf '%s\n' "$path"; done <formal.sha256 | sort)"
test "$actual" = "$expected" || { printf 'formal.sha256 must list exactly OpenAPI, canonical SQL, and every migration\n' >&2; exit 1; }
