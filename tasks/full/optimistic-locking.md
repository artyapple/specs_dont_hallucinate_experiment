# Feature: Optimistic Task Updates

Add optimistic locking and `PUT /tasks/{taskId}` for title replacement.

## Versioning

- Add an integer task version starting at 1.
- Include the integer `version` in every Task JSON representation.
- `POST`, `GET /tasks/{taskId}`, and successful `PUT` return a strong ETag such as `"3"`.
- Collection GET returns versioned Task objects but does not require a collection ETag.
- A successful update increments the version and returns the new ETag.

## PUT request

- Accept exactly `{ "title": "..." }`.
- Reject unknown fields.
- Trim title, require 1 to 200 Unicode code points, and store the trimmed value.

## Preconditions

- `PUT` requires exactly one strong integer ETag in `If-Match`.
- After optional surrounding HTTP whitespace, the value must match `"[1-9][0-9]*"` and fit signed 64-bit range.
- Zero, signs, leading zeros, overflow, weak tags, wildcards, and lists are invalid.
- Response ETags use canonical quoted decimal form without leading zeros.
- Missing `If-Match` returns `428 Precondition Required`.
- Malformed, weak, wildcard, or list-valued `If-Match` returns `400 Bad Request`.
- A stale version returns `412 Precondition Failed`.
- An unknown task returns `404 Not Found`.
- Apply error precedence in this order: missing header, malformed header, unknown task, stale version.

## Atomicity

The database update must atomically match the expected version, update the trimmed title, and increment the version. For two concurrent updates with the same current ETag, exactly one succeeds and the other returns `412 Precondition Failed`.

Errors use the repository's RFC 9457 Problem Details contract. Existing operations continue to work.

Update the formal API and database artifacts before adapting the implementation.
