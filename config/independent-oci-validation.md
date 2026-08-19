# Independent OCI Validation Protocol

Status: executed successfully on 2026-08-19 for source revision `14b8e64d8ff9114c710629debca810f09ca6299d`; remains draft policy until the global freeze. This protocol does not authorize pilots or measured runs.

## Entry Gate

Do not rent the server until all of the following are true:

- the experiment worktree is clean and its source revision is recorded;
- `images/prepare-oci-bundle.sh <absolute-external-directory>` completed;
- the external directory contains four OCI archives and `manifest.json`;
- the bundle exists outside `.cache` and outside the Git repository;
- a second local copy or backup of the bundle exists;
- archive SHA-256 values have been checked against `manifest.json`.

At this gate, tell the experiment owner: **rent the Task 6 server now**.

## Server Requirements

- A newly created disposable `linux/amd64` VM, preferably Debian 12 or Ubuntu 24.04.
- At least 4 vCPU, 8 GiB RAM, and 50 GiB disk.
- Docker Engine with Buildx, `jq`, `git`, `skopeo`, and standard checksum tools.
- No pre-existing experiment images, source checkout, caches, or provider credentials.
- SSH access through a public key. Never exchange a password or private-key content.

The owner supplies only: provider, region, hostname/IP, SSH username, public-key access method, OS image, architecture, CPU, memory, and disk. Record the validation operator separately. Do not place these transient access details in Git.

## Transfer

Transfer the exact external OCI bundle and a clean Git checkout at `manifest.json.sourceRevision`. Do not rebuild images on the server. Do not transfer `.env`, `OPENROUTER_API_KEY`, local OpenCode configuration, `.cache`, `pilots`, or `results`.

After transfer, verify the repository is clean and its `HEAD` equals `manifest.json.sourceRevision`. Run:

```text
./images/validate-oci-bundle.sh <bundle-directory> <new-evidence-directory>
```

## Required Validation

The command must fail unless:

- the host is Linux x86-64 and `OPENROUTER_API_KEY` is absent;
- archive hashes and OCI manifest digests match the transferred manifest;
- no target custom image tag existed before import;
- the exact archives import without rebuilding;
- imported image digests agree with the OCI archives;
- coordinator/tool bridge positive and negative tests pass;
- Direct and Codegen tools reach the internal PostgreSQL service but cannot reach arbitrary HTTPS or Go module infrastructure;
- synthetic coordinator credentials are absent from both tool environments and their process state;
- the evaluator image has no provider credential, runs as uid 10001, enforces offline Go modules, uses the pinned Ryuk digest, and passes all canonical image evaluations;
- the imported coordinator reaches OpenRouter through the fixed-destination relay but cannot reach non-provider HTTPS, plain HTTP, or OpenRouter by a direct-address bypass, while the imported tool cannot reach the relay.

The Docker network tests prove the credential-free tool boundary, internal-only tool network, and domain-level coordinator filtering. The positive coordinator check is an unauthenticated OpenRouter HTTPS request, not a model request. This exact protocol passed with imported freeze-candidate images on the separate clean machine, so `networkPolicyEnforcementStatus` is `validated` while the overall experiment remains `draft`.

## Evidence

The evidence directory contains no credentials and records:

- validation timestamp and operator supplied explicitly to the script;
- cloud provider, region, host identifier, OS, kernel, architecture, CPU, memory, and disk;
- Docker, Git, jq, and skopeo versions;
- source revision and clean-worktree result;
- copied OCI manifest and verified archive hashes/digests;
- imported image identities;
- separate logs and exit status for every bridge, network, credential, and evaluator check;
- overall pass/fail status.

Copy the evidence directory off the server. Verify its generated `evidence-sha256.txt` locally and inspect it for secrets before adding any selected evidence to Git.

## Destruction Gate

Delete the server only after:

- all evidence is copied back;
- local checksums match;
- the evidence contains no credential material;
- any failed check is preserved rather than rerun silently.

At this gate, tell the experiment owner: **evidence is preserved; delete the Task 6 server now**. Record provider-side deletion confirmation in Task 6 evidence. Server deletion is required before Task 6 can be marked complete.
