# Network and Credential Policy

Status: validated in development and implemented by the production orchestration. Exact freeze-image validation remains governed by the global freeze.

The model-free bridge, Direct and Codegen network-policy, and coordinator-egress checks passed again against the exact imported OCI archives on the Task 6 clean machine. That evidence validates internal PostgreSQL reachability; blocked external DNS, HTTPS, package access, subprocess access, redirects, and direct-address bypass; credential and Docker-socket absence from the tool; non-root/read-only execution; remote native-tool confinement and cancellation; exact network membership; and the fixed-destination provider relay. The experiment and policy documents remain draft until the global freeze, but network enforcement status is `validated`.

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
- Candidate orchestration starts a clean pinned visible PostgreSQL sidecar on the internal tool network and passes only its `DATABASE_URL` to the tool.
- Dependency caches are prepared before runs and mounted read-only.
- Hidden evaluator containers use a separate network and are never visible during the agent session.

`restricted-egress.sh` owns this topology and is shared by the model-free test, `smoke-openrouter.sh`, and production `run-candidate.sh`. Production gives both internal networks, every container, and its read-only module-cache volume the scheduled run label plus a unique instance label; cleanup filters on both labels. The cache is populated offline from the validated evaluator image before the tool starts. The relay is a standard-library Go program under `egressproxy/`, run as uid 10001 from the digest-pinned Go base image with a read-only root and no provider credential. Only the coordinator maps `openrouter.ai` to the relay; both coordinator networks are Docker-internal, so direct destination-IP bypass has no external route. This does not depend on application proxy support.

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
