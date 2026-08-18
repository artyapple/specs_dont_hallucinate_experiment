#!/usr/bin/env bash
set -euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ "$#" -ne 2 ]; then
  printf 'usage: %s <absolute-bundle-directory> <absolute-new-evidence-directory>\n' "$0" >&2
  exit 2
fi

BUNDLE="$1"
EVIDENCE="$2"

for path in "$BUNDLE" "$EVIDENCE"; do
  case "$path" in
    /*) ;;
    *) printf 'bundle and evidence paths must be absolute\n' >&2; exit 2 ;;
  esac
  case "$path" in
    "$ROOT"|"$ROOT"/*)
      printf 'bundle and evidence must remain outside the experiment repository: %s\n' "$path" >&2
      exit 2
      ;;
  esac
done

test -d "$BUNDLE" || { printf 'bundle directory does not exist: %s\n' "$BUNDLE" >&2; exit 2; }
test ! -e "$EVIDENCE" || { printf 'evidence directory already exists: %s\n' "$EVIDENCE" >&2; exit 2; }
readonly EVIDENCE_PARENT="$(dirname "$EVIDENCE")"
test -d "$EVIDENCE_PARENT" || { printf 'evidence parent directory does not exist: %s\n' "$EVIDENCE_PARENT" >&2; exit 2; }
BUNDLE="$(cd "$BUNDLE" && pwd -P)"
EVIDENCE="$(cd "$EVIDENCE_PARENT" && pwd -P)/$(basename "$EVIDENCE")"
readonly BUNDLE EVIDENCE
readonly MANIFEST="$BUNDLE/manifest.json"
test -f "$MANIFEST" || { printf 'bundle manifest does not exist: %s\n' "$MANIFEST" >&2; exit 2; }

for path in "$BUNDLE" "$EVIDENCE"; do
  case "$path" in
    "$ROOT"|"$ROOT"/*) printf 'canonical path resolves inside the experiment repository: %s\n' "$path" >&2; exit 2 ;;
  esac
done

case "$EVIDENCE" in
  "$BUNDLE"|"$BUNDLE"/*) printf 'evidence directory must not modify the OCI bundle\n' >&2; exit 2 ;;
esac

for name in VALIDATION_OPERATOR SERVER_PROVIDER SERVER_REGION SERVER_HOST_ID; do
  test -n "${!name:-}" || { printf '%s must be set to non-secret validation metadata\n' "$name" >&2; exit 2; }
done

if [ "${OPENROUTER_API_KEY+x}" = x ]; then
  printf 'OPENROUTER_API_KEY must be absent from the clean-machine validation shell\n' >&2
  exit 2
fi

for command in docker git jq skopeo sha256sum uname; do
  command -v "$command" >/dev/null || { printf 'required command is unavailable: %s\n' "$command" >&2; exit 2; }
done

test "$(uname -s)" = Linux || { printf 'independent validation requires Linux\n' >&2; exit 2; }
case "$(uname -m)" in
  x86_64|amd64) ;;
  *) printf 'independent validation requires amd64, got %s\n' "$(uname -m)" >&2; exit 2 ;;
esac

mkdir -p "$EVIDENCE/logs"
cp "$MANIFEST" "$EVIDENCE/manifest.json"
readonly STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
STATUS=failed

finish() {
  local exit_code="$?"
  local finished_at
  finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  jq -n \
    --arg status "$STATUS" \
    --arg startedAt "$STARTED_AT" \
    --arg finishedAt "$finished_at" \
    --argjson exitCode "$exit_code" \
    '{status:$status,startedAt:$startedAt,finishedAt:$finishedAt,exitCode:$exitCode}' \
    >"$EVIDENCE/status.json"
  (
    cd "$EVIDENCE"
    : >evidence-sha256.txt
    for file in host.json imported-images.tsv manifest.json status.json logs/*.log; do
      if [ -f "$file" ]; then
        sha256sum "$file" >>evidence-sha256.txt
      fi
    done
  )
}
trap finish EXIT

run_check() {
  local name="$1"
  shift
  if "$@" >"$EVIDENCE/logs/$name.log" 2>&1; then
    printf 'PASS %s\n' "$name"
  else
    printf 'FAIL %s; see %s\n' "$name" "$EVIDENCE/logs/$name.log" >&2
    return 1
  fi
}

run_check manifest-contract jq -e '
  .kind == "freeze-candidate-oci-export"
  and .distributablePublication == true
  and .sourceTreeClean == true
  and .platform == "linux/amd64"
  and (.sourceRevision | test("^[0-9a-f]{40}$"))
  and (.images | length == 4)
' "$MANIFEST"

readonly SOURCE_REVISION="$(jq -er '.sourceRevision' "$MANIFEST")"
run_check source-checkout sh -c '
  test -z "$(git -C "$1" status --porcelain --untracked-files=all)"
  test "$(git -C "$1" rev-parse HEAD)" = "$2"
' sh "$ROOT" "$SOURCE_REVISION"

jq -n \
  --arg operator "$VALIDATION_OPERATOR" \
  --arg provider "$SERVER_PROVIDER" \
  --arg region "$SERVER_REGION" \
  --arg hostId "$SERVER_HOST_ID" \
  --arg os "$(. /etc/os-release && printf '%s' "$PRETTY_NAME")" \
  --arg kernel "$(uname -sr)" \
  --arg architecture "$(uname -m)" \
  --arg cpu "$(nproc)" \
  --arg memoryBytes "$(awk '/MemTotal:/ {print $2 * 1024}' /proc/meminfo)" \
  --arg diskBytes "$(df -B1 --output=size / | awk 'NR == 2 {print $1}')" \
  --arg dockerVersion "$(docker version --format '{{.Server.Version}}')" \
  --arg gitVersion "$(git --version)" \
  --arg jqVersion "$(jq --version)" \
  --arg skopeoVersion "$(skopeo --version)" \
  --arg sourceRevision "$SOURCE_REVISION" \
  '{operator:$operator,provider:$provider,region:$region,hostId:$hostId,os:$os,kernel:$kernel,architecture:$architecture,cpu:($cpu|tonumber),memoryBytes:($memoryBytes|tonumber),diskBytes:($diskBytes|tonumber),dockerVersion:$dockerVersion,gitVersion:$gitVersion,jqVersion:$jqVersion,skopeoVersion:$skopeoVersion,sourceRevision:$sourceRevision,validationSourceTreeClean:true}' \
  >"$EVIDENCE/host.json"

: >"$EVIDENCE/imported-images.tsv"
verify_and_import() {
  local key="$1" tag="$2" digest="$3" archive_name="$4" archive_sha256="$5"
  local archive observed_archive_sha256 observed_digest expected_config_digest imported_id status
  case "$archive_name" in
    */*|..|.) printf 'invalid archive name in manifest: %s\n' "$archive_name" >&2; return 1 ;;
  esac
  archive="$BUNDLE/$archive_name"
  observed_archive_sha256="missing"
  observed_digest="unread"
  expected_config_digest="unread"
  imported_id="not-imported"
  status="failed"
  if [ -f "$archive" ]; then
    observed_archive_sha256="$(sha256sum "$archive" | cut -d ' ' -f 1)"
  fi
  if [ "$observed_archive_sha256" = "$archive_sha256" ]; then
    observed_digest="$(skopeo inspect --format '{{.Digest}}' "oci-archive:$archive" 2>"$EVIDENCE/logs/digest-$key.log" || true)"
    expected_config_digest="$(skopeo inspect --raw "oci-archive:$archive" 2>>"$EVIDENCE/logs/digest-$key.log" | jq -er '.config.digest' 2>>"$EVIDENCE/logs/digest-$key.log" || true)"
  fi
  if [ "$observed_archive_sha256" != "$archive_sha256" ] || [ "$observed_digest" != "$digest" ] || [ -z "$expected_config_digest" ]; then
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$key" "$tag" "$archive_sha256" "$observed_archive_sha256" "$digest" "$observed_digest" "$expected_config_digest" "$imported_id" "$status" >>"$EVIDENCE/imported-images.tsv"
    printf 'archive identity verification failed for %s\n' "$key" >&2
    return 1
  fi
  if docker image inspect "$tag" >/dev/null 2>&1; then
    printf 'target custom image existed before import: %s\n' "$tag" >&2
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$key" "$tag" "$archive_sha256" "$observed_archive_sha256" "$digest" "$observed_digest" "$expected_config_digest" "$imported_id" "$status" >>"$EVIDENCE/imported-images.tsv"
    return 1
  fi
  if run_check "import-$key" skopeo copy "oci-archive:$archive" "docker-daemon:$tag"; then
    imported_id="$(docker image inspect "$tag" --format '{{.Id}}')"
    if [ "$imported_id" = "$expected_config_digest" ]; then
      status="passed"
    fi
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$key" "$tag" "$archive_sha256" "$observed_archive_sha256" "$digest" "$observed_digest" "$expected_config_digest" "$imported_id" "$status" >>"$EVIDENCE/imported-images.tsv"
  test "$status" = passed || { printf 'imported config identity mismatch for %s\n' "$key" >&2; return 1; }
}

while IFS=$'\t' read -r key tag digest archive_name archive_sha256; do
  verify_and_import "$key" "$tag" "$digest" "$archive_name" "$archive_sha256"
done < <(jq -r '.images | to_entries[] | [.key,.value.tag,.value.digest,.value.archive,.value.archiveSha256] | @tsv' "$MANIFEST")

readonly COORDINATOR_IMAGE="$(jq -er '.images.coordinator.tag' "$MANIFEST")"
readonly DIRECT_IMAGE="$(jq -er '.images.toolDirect.tag' "$MANIFEST")"
readonly CODEGEN_IMAGE="$(jq -er '.images.toolCodegen.tag' "$MANIFEST")"
readonly EVALUATOR_IMAGE="$(jq -er '.images.evaluator.tag' "$MANIFEST")"
readonly POSTGRES_IMAGE="$(jq -er '.frozen.postgresImage' "$ROOT/config/versions.json")"
readonly RYUK_IMAGE="$(jq -er '.frozen.ryukImage' "$ROOT/config/versions.json")"
readonly GO_IMAGE="$(jq -er '.baseImages.go' "$ROOT/config/versions.json")"

run_check pull-postgres docker pull "$POSTGRES_IMAGE"
run_check pull-ryuk docker pull "$RYUK_IMAGE"
run_check pull-egress-runtime docker pull "$GO_IMAGE"

for image in "$COORDINATOR_IMAGE" "$DIRECT_IMAGE" "$CODEGEN_IMAGE" "$EVALUATOR_IMAGE"; do
  run_check "credential-absence-$(printf '%s' "$image" | tr '/:' '--')" \
    sh -c 'docker image inspect "$1" | jq -e '\''.[0].Config.Env // [] | all(startswith("OPENROUTER_API_KEY=") | not)'\'' >/dev/null' sh "$image"
done

run_check evaluator-ryuk-pin sh -c '
  docker image inspect "$1" | jq -e --arg expected "$2" '\''.[0].Config.Env // [] | index("TESTCONTAINERS_RYUK_CONTAINER_IMAGE=" + $expected) != null'\'' >/dev/null
' sh "$EVALUATOR_IMAGE" "$RYUK_IMAGE"

run_check bridge-direct env COORDINATOR_IMAGE="$COORDINATOR_IMAGE" TOOL_IMAGE="$DIRECT_IMAGE" "$ROOT/harness/test-tool-bridge.sh"
run_check bridge-codegen env COORDINATOR_IMAGE="$COORDINATOR_IMAGE" TOOL_IMAGE="$CODEGEN_IMAGE" "$ROOT/harness/test-tool-bridge.sh"
run_check network-direct env COORDINATOR_IMAGE="$COORDINATOR_IMAGE" TOOL_IMAGE="$DIRECT_IMAGE" "$ROOT/harness/test-network-policy.sh"
run_check network-codegen env COORDINATOR_IMAGE="$COORDINATOR_IMAGE" TOOL_IMAGE="$CODEGEN_IMAGE" "$ROOT/harness/test-network-policy.sh"
run_check coordinator-egress env EGRESS_PROXY_IMAGE="$GO_IMAGE" COORDINATOR_IMAGE="$COORDINATOR_IMAGE" TOOL_IMAGE="$DIRECT_IMAGE" "$ROOT/harness/test-coordinator-egress.sh"
run_check evaluator-image env EVALUATOR_IMAGE="$EVALUATOR_IMAGE" "$ROOT/images/test-evaluator-image.sh"

STATUS=passed
printf 'Independent OCI archive checks passed. Preserve evidence before deleting the server: %s\n' "$EVIDENCE"
