# Measured Result Artifacts

Measured run directories are ignored by default because they may be large or contain unsanitized transcripts.

Every run directory must contain:

```text
metadata.json
transcript.jsonl
commands.json
final.patch
evaluation.json
workspace-manifest.json
run-result.json
```

Artifact ownership:

- The run driver owns `metadata.json` (draft contract in `harness/README.md`), `transcript.jsonl`, `final.patch`, and the preserved `workspace/`.
- The run-result assembler (`harness/runresult/`) owns `commands.json` with per-event captures under `commands/`, `evaluation.json`, `workspace-manifest.json`, and `run-result.json`, and validates the result against the run-result schema.

Before publication:

- validate `run-result.json` against the frozen run-result schema;
- scrub credentials and provider secrets;
- preserve protocol violations and infrastructure failures;
- publish exclusion and replacement relationships;
- verify that aggregate charts can be regenerated from the cleaned artifacts.
