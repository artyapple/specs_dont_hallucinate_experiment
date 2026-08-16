# Pilot Artifacts

Pilot results are unmeasured and never enter primary analysis.

Create one ignored directory per pilot run containing:

```text
metadata.json
transcript.jsonl
commands.json
final.patch
evaluation.json
workspace-manifest.json
run-result.json
```

Any task, treatment, evaluator, or analysis change after pilots requires a new pilot phase and a new freeze.
