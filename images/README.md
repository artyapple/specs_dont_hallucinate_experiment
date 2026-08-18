# Experiment Images

These image definitions are pre-freeze build inputs. Base images and downloaded OpenCode content are digest-pinned.

## Images

- `coordinator.Dockerfile`: OpenCode coordinator plus the repository-owned bridge plugin and pinned Zod `4.1.8` runtime. It may receive provider credentials but does not execute candidate-controlled commands locally.
- `tool.Dockerfile --target direct`: credential-free candidate tool environment without generators, with the bridge server and OpenCode `1.18.18` native tool executor.
- `tool.Dockerfile --target codegen`: the same tool environment plus `oapi-codegen v2.8.0` and `sqlc v1.31.1`.
- `evaluator.Dockerfile`: the hidden evaluator binary built from pinned sources, plus the preloaded union of fixture module dependencies so candidate builds never need network access (`GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`). It runs as unprivileged uid 10001 and contains no provider credentials.

Both tool targets use Go `1.26.6`. The final Direct image does not contain generator binaries or generator build caches.

The tool image intentionally contains OpenCode but receives no provider credential. It uses only `opencode debug agent --tool` to preserve native `read`, `bash`, `apply_patch`, `edit`, and `write` behavior behind the bridge. The coordinator's plugin runtime dependency is downloaded by checksum during image build; runtime package installation is disabled by a pre-created read-only configuration directory.

## Evaluator Image Runtime

The evaluator image is defined by `evaluator.Dockerfile`. Because the evaluator executes candidate-controlled commands, its environment is hardened and credential-free:

- The candidate tree is mounted read-only at `/candidate`; Go build caches live in the container, so candidates never need in-tree writes.
- The Docker socket is mounted so Testcontainers can manage the pinned PostgreSQL sibling container. The evaluator runs as uid 10001 and receives socket access only through a supplementary group: the daemon socket group on Linux, or gid 0 on Docker Desktop where the VM mounts the socket as `root:root`. Root is never used. Testcontainers for Go detects the in-container environment and reaches the published PostgreSQL port through the bridge gateway.
- `test-evaluator-image.sh` proves credential absence, the unprivileged user, the offline module policy, the pinned Ryuk identity, and evaluates both baseline fixtures plus all six Direct and Codegen task references from the image.

```sh
make build-evaluator-image
make test-evaluator-image
```

The host Docker daemon must already hold the digest-pinned PostgreSQL and Ryuk images recorded in `config/versions.json`. The evaluator image fixes `TESTCONTAINERS_RYUK_CONTAINER_IMAGE` to that Ryuk digest, so Testcontainers never resolves a mutable cleanup-container tag.

## Local Build

From the repository root:

```sh
docker build --file images/coordinator.Dockerfile --tag specs-experiment-coordinator:1.18.18 .
docker build --file images/tool.Dockerfile --target direct --tag specs-experiment-tool-direct:go1.26.6 .
docker build --file images/tool.Dockerfile --target codegen --tag specs-experiment-tool-codegen:go1.26.6 .
docker build --file images/evaluator.Dockerfile --tag specs-experiment-evaluator:go1.26.6 .
```

Locally built image IDs are evidence for development checks, not frozen distributable image digests. Push or export final images reproducibly before replacing the remaining image TODOs.

## Freeze Distribution

The selected freeze mechanism is a reproducible OCI archive for each custom image; a public registry such as GHCR is not required. `export-oci.sh` implements the canonical `linux/amd64` build with timestamp normalization and disabled provenance/SBOM attestations.

At freeze, retain the archives together with `manifest.json`, record both each OCI image digest and archive SHA-256, import the exact archives on a separate clean machine, and rerun both harness tests. A local Docker image ID alone is not a distributable identity.

Prepare the Task 6 bundle only from a clean committed worktree and place it outside the repository and disposable cache:

```sh
make prepare-oci-bundle OUTPUT_DIR=/absolute/external/path
```

`config/independent-oci-validation.md` defines server rental, non-secret access handoff, exact-archive import, evidence preservation, and mandatory server deletion. On the clean Linux server, validation is exposed as `make validate-oci-bundle BUNDLE_DIR=/absolute/bundle EVIDENCE_DIR=/absolute/new-evidence` with the required non-secret server metadata environment variables.

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

The script fixes `SOURCE_DATE_EPOCH`, enables exporter timestamp rewriting, disables provenance/SBOM attestations, and records image digests plus archive SHA-256 values in an ignored `.cache/oci-export/<run-id>/manifest.json`. All four images (coordinator, Direct tool, Codegen tool, evaluator) are exported; the 2026-08-17 development export reproduced every image digest byte-for-byte across independent builds. These development archives do not replace `TODO_PIN_DIGEST` until the selected freeze copies are preserved outside the disposable cache and pass clean-machine verification.

The bridge, tool-network, and coordinator-egress harness tests accept optional `COORDINATOR_IMAGE` and `TOOL_IMAGE` environment overrides so imported OCI artifacts can be tested under separate tags without replacing the development images. Coordinator egress uses the repository-owned stdlib TLS relay from the already digest-pinned Go base image; it is orchestration infrastructure, not a fifth custom experiment image.
