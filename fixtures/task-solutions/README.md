# Canonical Task Solutions

Status: draft canonical references. Automated validation is complete; independent human review and global freeze remain pending.

Each Part 2 task has behaviorally equivalent Direct and Codegen references:

- `nullable-patch-direct` and `nullable-patch-codegen`;
- `optimistic-locking-direct` and `optimistic-locking-codegen`;
- `cursor-pagination-direct` and `cursor-pagination-codegen`.

These repositories validate task formal targets and the external evaluator. They are not pilot or measured-run artifacts. Part 2 agent runs start from clean canonical Base 2 revisions, not from these completed solutions.

Within each task pair:

- OpenAPI, migrations, and canonical SQL are byte-identical;
- Direct keeps handwritten HTTP and database boundaries and has no generator configuration or generated outputs;
- Codegen keeps pinned `oapi-codegen` and `sqlc` configuration and canonical generated outputs;
- both variants pass the same applicable external evaluator cases.

From the experiment root:

```text
make validate-task-targets
make verify-task-solutions
make evaluate-task-solutions
```

The corresponding content-addressed formal patches and manifests live under `tasks/propagation/<task>/`.
