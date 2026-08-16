#!/usr/bin/env bash
set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly RUN_ID="oci-export-$(date -u +%Y%m%dT%H%M%SZ)-$$"
readonly OUTPUT_DIR="${1:-$ROOT_DIR/.cache/oci-export/$RUN_ID}"
readonly SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-0}"

mkdir -p "$OUTPUT_DIR"

build_image() {
  local name="$1"
  local dockerfile="$2"
  local target="$3"
  local tag="$4"
  local archive="$OUTPUT_DIR/$name.oci.tar"
  local metadata="$OUTPUT_DIR/$name.build-metadata.json"
  if [ -n "$target" ]; then
    docker buildx build \
      --platform linux/amd64 \
      --provenance=false \
      --sbom=false \
      --build-arg "SOURCE_DATE_EPOCH=$SOURCE_DATE_EPOCH" \
      --file "$ROOT_DIR/$dockerfile" \
      --target "$target" \
      --tag "$tag" \
      --metadata-file "$metadata" \
      --output "type=oci,dest=$archive,rewrite-timestamp=true" \
      "$ROOT_DIR"
    return
  fi

  docker buildx build \
    --platform linux/amd64 \
    --provenance=false \
    --sbom=false \
    --build-arg "SOURCE_DATE_EPOCH=$SOURCE_DATE_EPOCH" \
    --file "$ROOT_DIR/$dockerfile" \
    --tag "$tag" \
    --metadata-file "$metadata" \
    --output "type=oci,dest=$archive,rewrite-timestamp=true" \
    "$ROOT_DIR"
}

build_image coordinator images/coordinator.Dockerfile "" specs-export/coordinator:1.18.18
build_image tool-direct images/tool.Dockerfile direct specs-export/tool-direct:go1.26.6
build_image tool-codegen images/tool.Dockerfile codegen specs-export/tool-codegen:go1.26.6

jq -n \
  --arg runId "$RUN_ID" \
  --arg createdAt "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --argjson sourceDateEpoch "$SOURCE_DATE_EPOCH" \
  --arg coordinatorDigest "$(jq -er '."containerimage.digest"' "$OUTPUT_DIR/coordinator.build-metadata.json")" \
  --arg coordinatorArchiveSha256 "$(shasum -a 256 "$OUTPUT_DIR/coordinator.oci.tar" | cut -d ' ' -f 1)" \
  --arg directDigest "$(jq -er '."containerimage.digest"' "$OUTPUT_DIR/tool-direct.build-metadata.json")" \
  --arg directArchiveSha256 "$(shasum -a 256 "$OUTPUT_DIR/tool-direct.oci.tar" | cut -d ' ' -f 1)" \
  --arg codegenDigest "$(jq -er '."containerimage.digest"' "$OUTPUT_DIR/tool-codegen.build-metadata.json")" \
  --arg codegenArchiveSha256 "$(shasum -a 256 "$OUTPUT_DIR/tool-codegen.oci.tar" | cut -d ' ' -f 1)" \
  '{
    runId: $runId,
    kind: "development-oci-export",
    distributablePublication: false,
    platform: "linux/amd64",
    provenance: false,
    sbom: false,
    sourceDateEpoch: $sourceDateEpoch,
    createdAt: $createdAt,
    images: {
      coordinator: {tag: "specs-export/coordinator:1.18.18", digest: $coordinatorDigest, archive: "coordinator.oci.tar", archiveSha256: $coordinatorArchiveSha256},
      toolDirect: {tag: "specs-export/tool-direct:go1.26.6", digest: $directDigest, archive: "tool-direct.oci.tar", archiveSha256: $directArchiveSha256},
      toolCodegen: {tag: "specs-export/tool-codegen:go1.26.6", digest: $codegenDigest, archive: "tool-codegen.oci.tar", archiveSha256: $codegenArchiveSha256}
    }
  }' >"$OUTPUT_DIR/manifest.json"

printf 'OCI export complete\nArtifacts: %s\n' "$OUTPUT_DIR"
jq -r '.images | to_entries[] | "\(.key): \(.value.digest)"' "$OUTPUT_DIR/manifest.json"
