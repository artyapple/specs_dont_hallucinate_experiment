# Task: Propagate the Frozen Optimistic Locking Contract

The repository's OpenAPI, migration, and canonical SQL query files already contain the frozen target change. Do not redesign or replace those formal artifacts.

Implement the supplied target so that:

- Task JSON exposes an integer version starting at 1.
- POST, GET by ID, and successful PUT return the current strong ETag.
- PUT accepts only title and applies baseline title normalization.
- If-Match accepts one quoted positive decimal integer without signs or leading zeros and within signed 64-bit range.
- Missing If-Match returns 428; zero, signs, leading zeros, overflow, weak tags, wildcards, lists, and other malformed syntax return 400.
- A valid tag for an unknown task returns 404; a stale tag for an existing task returns 412.
- A successful update atomically matches version, updates title, increments version, and returns the new ETag.
- Two concurrent updates using the same current ETag produce exactly one success and one 412.
- Existing operations continue to work.

Add meaningful tests, run visible checks, and report unresolved gaps before finishing.
