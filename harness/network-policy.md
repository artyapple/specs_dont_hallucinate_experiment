# Network and Credential Policy

Status: draft. This policy must be implemented and negatively tested before pilots.

Development evidence as of August 18, 2026: `test-network-policy.sh` passes against both locally built Direct and Codegen tool images, and `test-tool-bridge.sh` passes against the coordinator and Direct tool image. The checks prove internal PostgreSQL reachability; blocked external DNS, HTTPS, `wget`, Go proxy access, subprocess access, and redirect escape; absence of provider credentials from environment and process arguments; absence of the Docker socket; non-root execution; read-only system paths; exact Go `1.26.6`; remote native-tool execution; workspace path confinement; and command termination on timeout/abort. This is not freeze validation because the custom images do not yet have final transferred archive identities and the final dual-network coordinator orchestration has not been frozen.

## Threat Model

Candidate-controlled bash commands must not be able to:

- read model-provider credentials;
- contact the public internet;
- download packages or documentation;
- access the Docker socket;
- create child containers with broader network access;
- bypass policy through DNS, redirects, proxies, or alternate protocols.

## Target Architecture

- The OpenCode coordinator runs outside the candidate tool container and owns model-provider credentials.
- Candidate read, edit, and bash operations execute inside a separate pinned tool container.
- The tool container receives no model-provider credentials.
- The tool container has no Docker socket.
- The tool container has default-deny external egress.
- Visible PostgreSQL is a harness-managed sidecar on an isolated internal network.
- Dependency caches are prepared before runs and mounted read-only.
- Hidden evaluator containers use a separate network and are never visible during the agent session.

## Required Negative Tests

- Provider credentials are absent from environment, files, process arguments, and command output.
- Arbitrary HTTPS requests fail.
- DNS resolution for external names fails or is blocked consistently.
- `curl`, `wget`, package downloads, and `go env GOPROXY` access cannot reach external services.
- Redirects cannot escape an allowlist or proxy policy.
- The Docker socket and host container runtime are inaccessible.
- A visible-check subprocess receives the same restrictions as the parent shell.

## Freeze Requirement

`config/experiment.json` may move to `status: frozen` only after every negative test passes in the exact run image.
