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

- `nullable.omitted-preserves`: Omitted `dueAt` preserves the previous value.
- `nullable.null-clears`: Explicit null clears the previous value.
- `nullable.value-sets`: Timestamp sets the value and returns UTC with exactly six fractional-second digits.
- `nullable.initial-null`: Existing and newly created tasks expose `dueAt: null` before a deadline is set.
- `nullable.post-rejects-due-at`: POST rejects a supplied `dueAt` field.
- `nullable.title-null-rejected`: `title: null` is rejected.
- `nullable.title-only`: `title` can be changed without changing omitted `dueAt`.
- `nullable.both-fields`: Both fields can be changed together.
- `nullable.empty-rejected`: Empty patch is rejected.
- `nullable.unknown-field-rejected`: Unknown fields are rejected.
- `nullable.invalid-title`: Invalid blank and overlong titles are rejected.
- `nullable.invalid-timestamp`: Invalid timestamps are rejected.
- `nullable.unknown-task`: Unknown task returns `404`.
- `nullable.get-consistent`: GET after PATCH returns consistent state.
- Baseline create, get, list, and delete behavior remains valid.

## Optimistic Locking

- `locking.initial-version`: New task starts at version 1, returns ETag `"1"`, and appears with an integer version in the collection.
- `locking.get-etag`: GET by ID returns the current strong ETag matching the body version.
- `locking.unknown-field`: PUT rejects unknown fields without mutation or version increment.
- `locking.invalid-title`: PUT rejects blank and overlong titles without mutation or version increment.
- `locking.put-success`: Correct `If-Match` normalizes the title, increments version, and returns the new ETag.
- `locking.missing-if-match`: Missing `If-Match` returns `428`.
- `locking.malformed-if-match`: Zero, signed value, leading-zero value, overflow, unquoted value, weak tag, wildcard, and list return `400`.
- `locking.stale-if-match`: Stale ETag returns `412` without changing state.
- `locking.unknown-task`: Unknown task with a valid ETag returns `404`.
- `locking.concurrent-single-winner`: Exactly one of two concurrent updates using the same ETag succeeds and the loser returns `412`.
- Baseline operations remain valid with the versioned representation.

## Cursor Pagination

- `pagination.default-limit`: Missing limit uses 20.
- `pagination.limit-bounds`: Limits 1 and 100 are accepted.
- `pagination.invalid-limit`: Limit 0, negative values, values above 100, and non-integers return `400`.
- `pagination.malformed-cursor`: A clearly malformed cursor string returns `400`.
- `pagination.empty`: Empty dataset returns an empty page without `nextCursor`.
- `pagination.single-page`: One page returns all items without `nextCursor`.
- `pagination.multiple-pages`: Multiple pages return every fixed row exactly once in order.
- `pagination.timestamp-tie`: Identical timestamps are ordered by UUID without duplicates or gaps.
- `pagination.final-page`: Final page omits `nextCursor`.
- `pagination.cursor-after-delete`: A cursor obtained from the API still yields a valid empty final page if all later fixed rows are deleted before reuse.
- Baseline create, get, and delete behavior remains valid.

## Contract Conformance

- `contract.openapi-conformance`: Content types, success and error status codes, strict request decoding, and response shapes match OpenAPI.
- `contract.problem-details`: Problem Details contain frozen `type`, `title`, `status`, and `detail` semantics.
- Response bodies reject missing required fields and unexpected shape changes.
- Header contracts, including ETag, match OpenAPI.
