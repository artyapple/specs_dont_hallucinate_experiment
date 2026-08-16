# Human Review Rubric

Status: draft.

## Merge Decision

> Would you merge this patch into a production service after normal review?

Answers: `yes`, `yes-with-required-changes`, or `no`.

## Dimensions

Score each dimension from 1 to 5 using frozen anchors:

- clarity;
- idiomatic Go;
- architecture fit;
- traceability to formal requirements;
- error handling;
- candidate-authored test quality;
- reviewability;
- maintainability for the next change.

## Required Evidence

Every score of 1 or 2 and every `no` decision requires a file/line finding or a concrete missing behavior. Reviewers may record uncertainty when generated-code presence exposes the likely condition.

`TODO`: Write concrete anchors for scores 1, 3, and 5 in every dimension before review begins.
