# Feature: Cursor Pagination

Add cursor pagination to `GET /tasks`.

## Request

- Optional `limit`, default 20, accepted range 1 to 100.
- Optional opaque `cursor` returned by an earlier page.

## Ordering

- Stable ascending order by `(createdAt, id)`.
- Tasks with identical timestamps are disambiguated by UUID.
- Pages must not contain duplicates or skip existing rows in the fixed test dataset.

## Response

- Keep the existing `{ "items": [...] }` envelope.
- Add `nextCursor` when another page exists.
- Omit `nextCursor` on the final page.

## Errors

Invalid limits and malformed cursors return RFC 9457 Problem Details with `400 Bad Request`.

The cursor is opaque to API consumers. The canonical implementation uses unpadded Base64URL JSON containing `createdAt` and `id`.

Update the formal API and database artifacts before adapting the implementation.
