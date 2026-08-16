# Experiment Images

These image definitions are pre-freeze build inputs. Base images and downloaded OpenCode content are digest-pinned.

## Images

- `coordinator.Dockerfile`: OpenCode coordinator plus the repository-owned bridge plugin and pinned Zod `4.1.8` runtime. It may receive provider credentials but does not execute candidate-controlled commands locally.
- `tool.Dockerfile --target direct`: credential-free candidate tool environment without generators, with the bridge server and OpenCode `1.18.18` native tool executor.
- `tool.Dockerfile --target codegen`: the same tool environment plus `oapi-codegen v2.8.0` and `sqlc v1.31.1`.

Both tool targets use Go `1.26.6`. The final Direct image does not contain generator binaries or generator build caches.

The tool image intentionally contains OpenCode but receives no provider credential. It uses only `opencode debug agent --tool` to preserve native `read`, `bash`, `apply_patch`, `edit`, and `write` behavior behind the bridge. The coordinator's plugin runtime dependency is downloaded by checksum during image build; runtime package installation is disabled by a pre-created read-only configuration directory.

The evaluator image is intentionally not defined yet. Its digest can only be meaningful after the evaluator binary and its runtime contract exist.

## Local Build

From the repository root:

```sh
docker build --file images/coordinator.Dockerfile --tag specs-experiment-coordinator:1.18.18 .
docker build --file images/tool.Dockerfile --target direct --tag specs-experiment-tool-direct:go1.26.6 .
docker build --file images/tool.Dockerfile --target codegen --tag specs-experiment-tool-codegen:go1.26.6 .
```

Locally built image IDs are evidence for development checks, not frozen distributable image digests. Push or export final images reproducibly before replacing the remaining image TODOs.

## Freeze Distribution

The selected freeze mechanism is a reproducible OCI archive for each custom image; a public registry such as GHCR is not required. `export-oci.sh` implements the canonical `linux/amd64` build with timestamp normalization and disabled provenance/SBOM attestations.

At freeze, retain the archives together with `manifest.json`, record both each OCI image digest and archive SHA-256, import the exact archives on a separate clean machine, and rerun both harness tests. A local Docker image ID alone is not a distributable identity.

Registry publication remains an optional mirror. If it is added later, use equivalent immutable builds:

```sh
docker buildx build --platform linux/amd64 --provenance=false --sbom=false \
  --file images/coordinator.Dockerfile \
  --tag REGISTRY/specs-experiment-coordinator:1.18.18 \
  --push .

docker buildx build --platform linux/amd64 --provenance=false --sbom=false \
  --file images/tool.Dockerfile --target direct \
  --tag REGISTRY/specs-experiment-tool-direct:go1.26.6 \
  --push .

docker buildx build --platform linux/amd64 --provenance=false --sbom=false \
  --file images/tool.Dockerfile --target codegen \
  --tag REGISTRY/specs-experiment-tool-codegen:go1.26.6 \
  --push .
```

Verify an optional registry mirror with `docker buildx imagetools inspect REGISTRY/IMAGE@sha256:DIGEST`; its digest must agree with the frozen image identity or be documented as a distinct distribution artifact.

## Development OCI Export

Create reproducible single-platform OCI archives locally:

```sh
./images/export-oci.sh
```

The script fixes `SOURCE_DATE_EPOCH`, enables exporter timestamp rewriting, disables provenance/SBOM attestations, and records image digests plus archive SHA-256 values in an ignored `.cache/oci-export/<run-id>/manifest.json`. These development archives do not replace `TODO_PIN_DIGEST` until the selected freeze copies are preserved outside the disposable cache and pass clean-machine verification.

Both harness test scripts accept optional `COORDINATOR_IMAGE` and `TOOL_IMAGE` environment overrides so imported OCI artifacts can be tested under separate tags without replacing the development images.
