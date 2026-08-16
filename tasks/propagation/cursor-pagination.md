# Task: Propagate the Frozen Cursor Pagination Contract

The repository's OpenAPI and canonical SQL query files already contain the frozen target change. Do not redesign or replace those formal artifacts.

Implement the supplied target so that:

- GET `/tasks` accepts optional limit and cursor parameters.
- Limit defaults to 20 and accepts values from 1 through 100.
- Ordering is stable and ascending by `(createdAt, id)`.
- The existing `{ "items": [...] }` envelope gains `nextCursor` only when another page exists.
- The final page omits `nextCursor`.
- Invalid limit or malformed cursor returns the supplied 400 Problem Details response.
- API consumers can pass server-issued cursors back without understanding their contents.
- Pages over a fixed dataset contain no gaps or duplicates, including timestamp ties.
- Existing create, get, and delete behavior continues to work.

Add meaningful tests, run visible checks, and report unresolved gaps before finishing.
