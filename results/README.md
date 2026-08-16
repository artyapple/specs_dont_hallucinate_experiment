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

Before publication:

- validate `run-result.json` against the frozen run-result schema;
- scrub credentials and provider secrets;
- preserve protocol violations and infrastructure failures;
- publish exclusion and replacement relationships;
- verify that aggregate charts can be regenerated from the cleaned artifacts.
