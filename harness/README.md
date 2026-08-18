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

The scheduler forms seven strata: Greenfield and each task/mode combination. A measured schedule contains five 14-run blocks. Each block shuffles the seven strata and independently randomizes treatment order, then places the Direct and Codegen runs for each stratum adjacently. This guarantees local treatment balance throughout execution as well as five observations per cell. Pilot schedules use one separate 14-run block.

Schedule manifests identify this algorithm as `sha256-fisher-yates-v1`. `harness/freezecheck` uses the architecture-independent SHA-256 counter stream and rejection-sampled Fisher-Yates algorithm specified in `freezecheck/schedule.go`. Generation requires an explicit seed, config revision, timestamp, and output path; it never chooses them implicitly and never overwrites a manifest. The schedule and seed are frozen before measured runs. Pilot and measured schedules are separate.

```text
make generate-schedule PHASE=measured SEED=<seed> REVISION=<commit> GENERATED_AT=<rfc3339> OUTPUT=<path>
make validate-schedule PHASE=measured SCHEDULE=<path>
```

## Completion and Failures

- Early final response ends the run.
- Timeout preserves and evaluates the partial candidate.
- Protocol violations are recorded and do not silently remove a run.
- Frozen infrastructure-failure rules determine replacement eligibility.
- Replaced artifacts remain preserved and published.

## Run Result Assembly

`bin/runresult` (source `runresult/`, built by `make build-runresult`) converts a preserved run directory into a validated `run-result.json`. It is the only component that merges hidden evaluator output into the complete run result. Run drivers invoke it after final submission or timeout; it never feeds information back to the agent.

### Driver-Owned Inputs

Before assembly the run directory must contain:

- `metadata.json` — run identity and lifecycle state (draft contract below);
- `transcript.jsonl` — OpenCode `--format json` output, possibly empty;
- `final.patch` — candidate diff, possibly empty;
- `workspace/` — preserved candidate workspace (required for `submitted` and `timed-out`).

Draft `metadata.json` fields: `runId`, `cellId` (must exist in `config/experiment.json`), `repeatIndex` 1–5, `phase` (`pilot` or `measured`), `status` (`submitted`, `timed-out`, `infrastructure-failure`, or `harness-failure`), `startedAt` and `finishedAt` (RFC 3339; for timed-out runs `finishedAt` is the enforced deadline so wall clock never exceeds 2,700,000 ms), optional `workspace` directory name (default `workspace`), `protocolViolations` (pass-through from driver evidence), `infrastructure` with `category` and `evidence` (required for the two failure statuses), and optional `replacesRunId` replacement linkage. Infrastructure categories follow `config/infrastructure-failure-policy.md`: `model-provider-outage`, `harness-process-crash`, `host-container-runtime-failure`, `evaluator-infrastructure-failure`, `artifact-storage-failure`.

### Assembly Behavior

1. Resolves stage, task, mode, and treatment from the cell in `config/experiment.json`.
2. For `submitted` and `timed-out`, invokes `bin/evaluator -task <task> -candidate <workspace>` and preserves its raw output as `evaluation.json`. For the two failure statuses, skips evaluation and writes `evaluation.json` containing `null`; no behavior outcomes are fabricated.
3. Failure classification:
   - evaluator exit 2, missing or unparseable result JSON, spawn failure, assembly deadline, or case-roster mismatch → `harness-failure` with category `harness-process-crash`;
   - evaluator `setup.postgres=false` → `infrastructure-failure` with category `evaluator-infrastructure-failure`;
   - build, migration, or service-start setup failures and any case failures → candidate failure: status stays `submitted` or `timed-out` and the evaluation embeds the failed gates and cases;
   - Codegen health container failure → `infrastructure-failure` with category `host-container-runtime-failure`.
   The assembler never sets `exclusionEligible`, `excluded`, or `replacementRunId`; exclusion remains a later reviewed decision under the infrastructure-failure policy.
4. Draft common-gate derivation: `build`, `migrations`, and `service-start` come from evaluator setup gates; `baseline-behavior` covers the seven `baseline.*` cases; `task-behavior` covers all applicable task-specific cases and mirrors `baseline-behavior` for `baseline-service`; `regressions` covers every common (`task: all`) case, that is baseline plus contract; `api-conformance` is `contract.openapi-conformance` and `contract.problem-details`; `database-consistency` is `contract.database-consistency`; `formal-inputs` requires propagation-only candidates to hash-match every file in `tasks/propagation/<task>/target-manifest.json` byte-for-byte, and otherwise checks presence and basic validity of `api/openapi.yaml`, `db/queries/tasks.sql`, and at least one `NNNNNN_name.sql` migration.
5. `completeSuccess` is every common gate true and every applicable required behavior case passed. `residualFailures` lists the sorted IDs of failed applicable cases. `candidateTests` counts workspace `*_test.go` files as a quality finding only.
6. Codegen health (Codegen treatment only) regenerates derived code from the candidate's own formal inputs inside the pinned codegen tool image using harness-owned canonical commands; candidate scripts are never invoked. `canonical` requires the first regeneration to byte-match the committed generated files, `idempotent` requires a second regeneration to be byte-identical, and `manualEditDetected` is `generationSucceeded` and not `canonical`, which conflates manual edits with stale regeneration until human review.
7. Draft transcript extraction: `usage.turns` counts `step_finish` events, `usage.toolCalls` counts `tool_use` events, and token sums use the reported `tokens.input` and `tokens.output` only; cache and reasoning tokens remain available in the transcript artifact. `commands.json` holds one event per tool call with a draft category classification (`generate`, `migration`, `build`, `test`, `service`, `read`, `edit`, or `other`; `network-attempt` is reserved for network-layer evidence). `process.repairIterations` counts failed build or test command events, `filesTouched` counts distinct edit, write, and apply_patch paths, diff lines classify `api/` and `db/` as contract, the frozen generated paths as generated (Codegen only), and the rest as handwritten, and `compilerEvents` codes Go `file:line:col` diagnostics from failed build or test output with category `generated-type` or `handwritten`; `followedByRelevantRepair` stays null until analysis coding rules freeze.
8. Writes `commands.json` with per-event output captures under `commands/`, `workspace-manifest.json` with SHA-256 and size per workspace file, and `run-result.json`. The result is validated against `schemas/run-result.schema.json` (draft 2020-12 with format assertions) before it is written.

### Exit Codes

- `0`: a schema-valid `run-result.json` was produced, including candidate-failure results;
- `2`: no honest result could be assembled (driver contract violation, unreadable inputs, or schema rejection). This is always a harness defect and must stop the pipeline loudly.

### Tests

- `make test-runresult` — unit tests for gate derivation, failure classification, transcript and diff parsing, formal-input checks, and schema acceptance and rejection for all four statuses;
- `make test-runresult-integration` — end-to-end assembly against canonical fixtures as synthetic preserved workspaces: greenfield Direct pass, greenfield Codegen pass with healthy codegen metrics, a candidate migration failure, and both driver-classified failure paths. Artifacts land in ignored `results/runresult-tests/`. No agent run, pilot, or measured run is exercised.

## Remaining Work

- Freeze orchestration that gives the coordinator provider egress while leaving the tool container on its internal-only network.
- Publish or reproducibly export the custom images and replace local image IDs with distributable digests.
- Implement the run driver that creates workspaces, runs the agent, finalizes `metadata.json`, and invokes run-result assembly (roadmap Task 7).
- Implement transcript secret scrubbing.
