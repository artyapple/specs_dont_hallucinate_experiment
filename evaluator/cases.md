# Hidden Behavior Case Catalog

Status: draft. Freeze before measured runs.

## Baseline

- `baseline.create-valid`: Create a valid task and verify `201`, body, UUIDv4, trimmed title, and UTC timestamp with six fractional digits.
- `baseline.create-invalid-title`: Reject blank and 201-code-point titles with RFC 9457 Problem Details.
- `baseline.get-existing`: Get an existing task.
- `baseline.get-not-found`: Return Problem Details for an unknown task.
- `baseline.list-ordered`: List tasks in stable ascending `(createdAt, id)` order using the `{items}` envelope.
- `baseline.delete-existing`: Delete an existing task with `204` and an empty body.
- `baseline.delete-again-not-found`: Return `404` when deleting the same task again.
- `contract.database-consistency`: Verify persisted database state after HTTP create and delete operations.

## Nullable PATCH

- Omitted `dueAt` preserves the previous value.
- Explicit null clears the previous value.
- Timestamp sets the value and returns UTC with exactly six fractional-second digits.
- Existing and newly created tasks expose `dueAt: null` before a deadline is set.
- POST rejects a supplied `dueAt` field.
- `title: null` is rejected.
- `title` can be changed without changing omitted `dueAt`.
- Both fields can be changed together.
- Empty patch is rejected.
- Unknown fields are rejected.
- Invalid title and timestamp are rejected.
- Unknown task returns `404`.
- GET after PATCH returns consistent state.
- Baseline create, get, list, and delete behavior remains valid.

## Optimistic Locking

- New task starts at version 1 and returns ETag `"1"`.
- Task JSON includes the current integer version.
- GET by ID returns the current strong ETag.
- Collection GET returns versioned Task objects without requiring a collection ETag.
- PUT rejects unknown fields and applies baseline title normalization.
- Correct `If-Match` updates title, increments version, and returns the new ETag.
- Missing `If-Match` returns `428`.
- Zero, signed value, leading-zero value, overflow, unquoted value, weak tag, wildcard, and list return `400`.
- Stale ETag returns `412` without changing state.
- Unknown task returns `404`.
- Two concurrent updates using the same ETag cannot both succeed.
- Exactly one concurrent update succeeds and the losing request returns `412`.
- Baseline operations remain valid with the versioned representation.

## Cursor Pagination

- Missing limit uses 20.
- Limits 1 and 100 are accepted.
- Limit 0, negative values, and values above 100 return `400`.
- A clearly malformed cursor string returns `400`.
- Empty dataset returns an empty page without `nextCursor`.
- One page returns all items without `nextCursor`.
- Multiple pages return every fixed row exactly once in order.
- Identical timestamps are ordered by UUID without duplicates or gaps.
- Final page omits `nextCursor`.
- A cursor obtained from the API still yields a valid empty final page if all later fixed rows are deleted before reuse.
- Baseline create, get, and delete behavior remains valid.

## Contract Conformance

- `contract.openapi-conformance`: Content types, success and error status codes, strict request decoding, and response shapes match OpenAPI.
- `contract.problem-details`: Problem Details contain frozen `type`, `title`, `status`, and `detail` semantics.
- Response bodies reject missing required fields and unexpected shape changes.
- Header contracts, including ETag, match OpenAPI.
