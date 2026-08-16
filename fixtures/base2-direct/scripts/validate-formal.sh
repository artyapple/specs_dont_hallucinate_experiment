#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum --check formal.sha256
else
  shasum -a 256 --check formal.sha256
fi
