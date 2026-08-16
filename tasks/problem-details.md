# Problem Details Catalog

Status: fixed for fixture and evaluator implementation. It remains draft experiment input until the global freeze.

All errors use `application/problem+json` and include `type`, `title`, `status`, and `detail`.

The five categories have exact machine-readable values. `type`, `title`, `status`, and `detail` are required and must equal the values below; implementations must not expose internal error text through `detail`.

| Type | Title | Status | Detail | Category |
|---|---|---:|---|---|
| `urn:problem:validation` | `Validation failed` | 400 | `The request is invalid.` | Invalid body, parameter, header, or cursor |
| `urn:problem:not-found` | `Task not found` | 404 | `The requested task does not exist.` | Task does not exist |
| `urn:problem:precondition-required` | `Precondition required` | 428 | `A valid If-Match header is required.` | Required `If-Match` is missing |
| `urn:problem:precondition-failed` | `Precondition failed` | 412 | `The supplied task version is stale.` | Valid `If-Match` is stale |
| `urn:problem:internal` | `Internal server error` | 500 | `The server could not complete the request.` | Unexpected server failure |

Every error response has `Content-Type: application/problem+json`. Additional members are forbidden in the experiment contract so candidates cannot rely on treatment-specific error extensions.

Example validation response:

```json
{
  "type": "urn:problem:validation",
  "title": "Validation failed",
  "status": 400,
  "detail": "The request is invalid."
}
```

Example not-found response:

```json
{
  "type": "urn:problem:not-found",
  "title": "Task not found",
  "status": 404,
  "detail": "The requested task does not exist."
}
```
