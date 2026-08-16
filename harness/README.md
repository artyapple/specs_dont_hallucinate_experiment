# Harness

The harness creates isolated agent runs and preserves all artifacts required for independent evaluation.

## Run Lifecycle

1. Resolve the frozen cell, fixture revision, treatment overlay, task revision, and environment image.
2. Create a clean isolated workspace and container namespace.
3. Apply the treatment overlay and, for propagation-only cells, the frozen target patch.
4. Start a fresh OpenCode session with the frozen task text.
5. Allow only read, edit, and bash capabilities.
6. Stop on final response or after 45 minutes.
7. Preserve transcript, tool events, timestamps, usage, final workspace, and patch.
8. Run the hidden evaluator once without returning feedback to the agent.
9. Store evaluator detail in `evaluation.json`, assemble `run-result.json`, and validate the complete run result against the result schema.
10. Destroy the run environment.

## OpenCode CLI

The selected runner is the official OpenCode CLI `1.18.18`, not the locally installed OpenCode Desktop `1.18.7` Electron launcher. The official Darwin x64 CLI archive and its verified SHA-256 are recorded in `../config/versions.json`; the Linux CLI must be copied into the pinned agent image and verified from the matching official release before that image is frozen.

Create the local credential file from `.env.example` and fill in the OpenRouter key:

```sh
cp .env.example .env
```

```dotenv
OPENROUTER_API_KEY=your-key
```

`.env` is ignored by Git. The single model setting is `model.id` in `config/experiment.json`; it currently selects `openrouter/deepseek/deepseek-v4-flash-0731`. The harness passes that value to OpenCode through `OPENCODE_MODEL`. Change this one field before the experiment freeze if a different model is selected.

The coordinator launches a non-interactive run with the repository-owned `opencode-run.json` injected after user and project configuration. The bridge plugin is baked into the coordinator image at the path recorded in that configuration, so `--pure` must not be used:

```sh
set -a
. ./.env
set +a
: "${OPENROUTER_API_KEY:?set OPENROUTER_API_KEY in .env}"
export OPENCODE_MODEL="$(jq -r '.model.id' config/experiment.json)"

OPENCODE_CONFIG_CONTENT="$(jq -c . harness/opencode-run.json)" \
OPENCODE_DISABLE_DEFAULT_PLUGINS=1 \
OPENCODE_DISABLE_EXTERNAL_SKILLS=1 \
OPENCODE_DISABLE_CLAUDE_CODE=1 \
OPENCODE_DISABLE_LSP_DOWNLOAD=1 \
/usr/local/bin/opencode run \
  --agent experiment \
  --format json \
  --dir "$WORKSPACE" \
  "$TASK_TEXT"
```

The harness, not OpenCode, enforces the 45-minute wall-clock timeout. `opencode-run.json` defaults every capability to `deny` and allows only `read`, `edit`, and `bash`. For `gpt-5.6-sol`, OpenCode exposes `apply_patch` in place of `edit`/`write`; other models such as DeepSeek expose `edit` and `write`. The plugin overrides `read`, `bash`, `apply_patch`, `edit`, and `write` with the same IDs and compatible schemas.

Before pilots, `smoke-openrouter.sh` runs one disposable development smoke against the real provider. It resolves the model only from `config/experiment.json`, attaches the coordinator to provider egress and the internal tool network, and leaves the credential-free tool container only on the internal network. It exercises `read`, `bash`, `write`, and `edit` without using any experiment fixture or pilot directory:

```sh
./harness/smoke-openrouter.sh
```

Credential-free development evidence, the JSONL transcript, and the final smoke workspace are stored under ignored `.cache/openrouter-smoke/`. This smoke is neither a pilot nor a measured run.

## Tool Bridge

The coordinator owns provider credentials and loads `tool-bridge.ts`. The plugin sends only the tool ID and JSON arguments over the isolated run network to `tool-bridge` in the credential-free tool container. The server validates workspace paths, then invokes the same OpenCode `1.18.18` native tool through `opencode debug agent --tool`. This preserves native read formatting, edit/write behavior, shell output/exit metadata, timeout behavior, and the OpenCode patch parser without shell interpolation.

The tool container is the only writable owner of `/workspace`. The coordinator mounts that path read-only. The tool container has an internal network, no provider environment, no Docker socket, a read-only root filesystem, and runs as UID 10001. It is started with `--init` so descendants killed on abort are reaped.

OpenCode `v1.18.18` source establishes the override behavior:

- `packages/opencode/src/tool/registry.ts:194-199,251-254` appends plugin tools after built-ins;
- `packages/opencode/src/session/tools.ts:92-100` assigns definitions by ID in order, so the later plugin definition wins;
- `packages/opencode/src/tool/registry.ts:286-298` selects `apply_patch` for eligible `gpt-*` models;
- `packages/plugin/src/tool.ts:3-20` supplies the plugin `AbortSignal`;
- `packages/opencode/src/tool/shell.ts:533-594` defines native abort, timeout, output, and exit semantics.

`test-tool-bridge.sh` is a model-free integration test. The official experimental tool-list endpoint proves that all five plugin definitions load after the matching built-ins in GPT and DeepSeek tool modes and applies the same last-write reduction as session resolution. OpenCode's debug command selects the first duplicate definition rather than the runtime winner, so test-only alias IDs invoke the same plugin definitions to exercise transport and native tools without a model call. The aliases exist only when `TOOL_BRIDGE_TEST_ALIASES=1` and are absent from experiment runs.

## Isolation

- Pinned run image.
- Unique workspace per run.
- Unique Docker labels, networks, and resource names.
- At most two concurrent runs.
- Model-provider access only; no agent web tools or package downloads.
- Prepared read-only dependency caches.
- No credentials copied into candidate repositories or published artifacts.

## Scheduling

Generate a blocked randomized schedule that balances treatments within every task and mode over execution time.

The schedule and seed are frozen before measured runs. Pilot and measured schedules are separate.

## Completion and Failures

- Early final response ends the run.
- Timeout preserves and evaluates the partial candidate.
- Protocol violations are recorded and do not silently remove a run.
- Frozen infrastructure-failure rules determine replacement eligibility.
- Replaced artifacts remain preserved and published.

## TODO

- Install and checksum the selected OpenCode Linux CLI in the run image, then pin the image digest.
- Freeze orchestration that gives the coordinator provider egress while leaving the tool container on its internal-only network.
- Publish or reproducibly export the custom images and replace local image IDs with distributable digests.
- Implement deterministic command-event parsing and token extraction.
- Implement transcript secret scrubbing.
