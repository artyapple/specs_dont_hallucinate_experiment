# Feature: Nullable Task Deadline and Partial Update

Add a nullable task deadline and partial task updates.

## API

Add `PATCH /tasks/{taskId}`. The request may contain `title`, `dueAt`, or both.

`dueAt` has three distinct states:

- omitted: preserve the current value;
- explicit `null`: clear the current value;
- RFC3339 timestamp: set the supplied value.

## Validation

- At least one recognized field is required.
- Unknown JSON fields are rejected.
- Duplicate JSON member handling is outside the required behavior.
- Trim `title`, require 1 to 200 Unicode code points, and store the trimmed value.
- `title: null` is invalid.
- Normalize returned timestamps to UTC with exactly six fractional-second digits.

## Representation

- Existing rows and newly created rows start with `dueAt: null`.
- `POST /tasks` still accepts only `title`; supplying `dueAt` is rejected as an unknown field.
- Every Task response includes a required nullable `dueAt` member.
- An unset deadline is represented as `"dueAt": null`.

## Behavior

- Success returns `200 OK` and the updated Task.
- Unknown task returns `404 Not Found`.
- Invalid input returns RFC 9457 Problem Details with `400 Bad Request`.
- A subsequent GET returns state consistent with PATCH.
- Existing operations continue to work.

Update the formal API and database artifacts before adapting the implementation.
