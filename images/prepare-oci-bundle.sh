#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ "$#" -ne 1 ]; then
  printf 'usage: %s <absolute-external-output-directory>\n' "$0" >&2
  exit 2
fi

OUTPUT="$1"
case "$OUTPUT" in
  /*) ;;
  *) printf 'output directory must be absolute\n' >&2; exit 2 ;;
esac

readonly OUTPUT_PARENT="$(dirname "$OUTPUT")"
test -d "$OUTPUT_PARENT" || { printf 'output parent directory does not exist: %s\n' "$OUTPUT_PARENT" >&2; exit 2; }
OUTPUT="$(cd "$OUTPUT_PARENT" && pwd -P)/$(basename "$OUTPUT")"
readonly OUTPUT

case "$OUTPUT" in
  "$ROOT"|"$ROOT"/*)
    printf 'freeze-candidate bundle must be outside the experiment repository: %s\n' "$OUTPUT" >&2
    exit 2
    ;;
esac

if [ -e "$OUTPUT" ]; then
  printf 'output directory already exists: %s\n' "$OUTPUT" >&2
  exit 2
fi

if [ -n "$(git -C "$ROOT" status --porcelain --untracked-files=all)" ]; then
  printf 'refusing freeze-candidate export from a dirty worktree\n' >&2
  exit 2
fi

OCI_EXPORT_KIND=freeze-candidate-oci-export "$ROOT/images/export-oci.sh" "$OUTPUT"

jq -e '
  .kind == "freeze-candidate-oci-export"
  and .distributablePublication == true
  and .sourceTreeClean == true
  and (.sourceRevision | test("^[0-9a-f]{40}$"))
  and (.images | length == 4)
' "$OUTPUT/manifest.json" >/dev/null

printf 'Freeze-candidate OCI bundle prepared outside the repository.\n'
printf 'Do not rent the clean-machine server until this directory is backed up: %s\n' "$OUTPUT"
