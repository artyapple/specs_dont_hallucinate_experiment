# Network and Credential Policy

Status: draft. This policy must be implemented and negatively tested before pilots.

Development evidence as of August 18, 2026: `test-network-policy.sh` passes against both locally built Direct and Codegen tool images, `test-tool-bridge.sh` passes against the coordinator and Direct tool image, and model-free `test-coordinator-egress.sh` passes with no provider credential. The checks prove internal PostgreSQL reachability; blocked external DNS, HTTPS, `wget`, Go proxy access, subprocess access, and redirect escape; absence of provider credentials from tool environment and process arguments; absence of the Docker socket; non-root execution; read-only system paths; exact Go `1.26.6`; remote native-tool execution; workspace path confinement; command termination on timeout/abort; exact coordinator network membership; positive unauthenticated OpenRouter HTTPS reachability; blocked non-provider, plain-HTTP, direct-bypass, and redirect traffic; and absence of the provider relay from the tool network. This is not freeze validation because the custom images do not yet have final transferred archive identities and the same checks have not passed on the clean machine.

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
- The coordinator joins only the internal tool and provider networks and has no direct external route.
- A repository-owned TCP relay joins the internal provider network and the external bridge and always forwards to exactly `openrouter.ai:443`.
- Candidate read, edit, and bash operations execute inside a separate pinned tool container.
- The tool container receives no model-provider credentials.
- The tool container has no Docker socket.
- The tool container has default-deny external egress.
- Visible PostgreSQL is a harness-managed sidecar on an isolated internal network.
- Dependency caches are prepared before runs and mounted read-only.
- Hidden evaluator containers use a separate network and are never visible during the agent session.

`restricted-egress.sh` owns this topology and is shared by the model-free test and `smoke-openrouter.sh`. The relay is a standard-library Go program under `egressproxy/`, run as uid 10001 from the digest-pinned Go base image with a read-only root and no provider credential. Only the coordinator maps `openrouter.ai` to the relay; both coordinator networks are Docker-internal, so direct destination-IP bypass has no external route. This does not depend on application proxy support.

## Required Negative Tests

- Provider credentials are absent from environment, files, process arguments, and command output.
- Arbitrary HTTPS requests fail.
- DNS resolution for external names fails or is blocked consistently.
- `curl`, `wget`, package downloads, and `go env GOPROXY` access cannot reach external services.
- Redirects cannot escape an allowlist or proxy policy.
- The Docker socket and host container runtime are inaccessible.
- A visible-check subprocess receives the same restrictions as the parent shell.
- Coordinator requests to non-provider HTTPS, plain HTTP, and a directly resolved OpenRouter address fail.
- Coordinator redirects cannot escape the exact provider allowlist.
- The tool cannot resolve or reach the provider relay.

`make test-coordinator-egress` performs no agent or model API call. It makes one unauthenticated request to the public OpenRouter HTTPS endpoint to prove live routing. The real-provider development smoke remains the separate application-level proof that OpenCode honors the same restricted topology.

## Freeze Requirement

`config/experiment.json` may move to `status: frozen` only after every negative test passes in the exact run image.
