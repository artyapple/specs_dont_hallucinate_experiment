# Propagation-Only Tasks

Each propagation-only task starts from a canonical Base 2 repository with a byte-identical target patch already applied to:

- OpenAPI;
- migrations;
- canonical SQL query files.

The agent receives the same business semantics as the corresponding full-workflow task, but must not redesign the target formal artifacts.

Canonical draft files:

```text
nullable-patch/
├── formal.patch
└── target-manifest.json

optimistic-locking/
├── formal.patch
└── target-manifest.json

cursor-pagination/
├── formal.patch
└── target-manifest.json
```

Each task has one content-addressed formal patch. The harness applies that same patch to both canonical bases. Treatment-specific implementation changes are produced only by the agent after the run starts.

The three draft patches and manifests are implemented. Run `make validate-task-targets` from the experiment root to verify patch hashes, apply every patch to the baseline formal tree, compare the patched tree byte-for-byte with both canonical task solutions, and validate every target file hash. Freeze them only during the global freeze.
