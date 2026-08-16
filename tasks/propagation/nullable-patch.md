# Task: Propagate the Frozen Nullable PATCH Contract

The repository's OpenAPI, migration, and canonical SQL query files already contain the frozen target change. Do not redesign or replace those formal artifacts.

Implement the supplied target so that:

- PATCH accepts `title`, `dueAt`, or both and rejects an empty object or unknown fields.
- Omitted `dueAt` preserves the stored value, null clears it, and a timestamp sets it.
- Existing and newly created tasks begin with `dueAt: null`.
- POST still accepts only title and rejects a supplied `dueAt`.
- Every Task response includes the required nullable `dueAt` member.
- `title: null` is invalid; valid titles are trimmed to 1–200 Unicode code points.
- Timestamps are returned in UTC with exactly six fractional-second digits.
- Success returns the updated Task; invalid input and unknown tasks follow the supplied Problem Details contract.
- Existing operations continue to work.

Add meaningful tests, run visible checks, and report unresolved gaps before finishing.
