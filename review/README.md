# Optional Human Review

Human review runs only after automated evaluation and only if the deadline permits.

## Sample

- Runs 2 and 4 from every cell.
- 28 candidates total before infrastructure replacements.
- Both reviewers independently review every eligible candidate.
- Presentation order is randomized separately for each reviewer with a frozen seed.

If a selected run is excluded for infrastructure failure, review its measured replacement. The excluded original remains in the published artifact set but is not part of the comparative review sample.

## Visible Materials

Reviewers receive:

- task business semantics required to understand the change;
- contract diff;
- handwritten diff;
- generated diff in a separate section;
- candidate-authored tests.

Reviewers do not receive:

- condition label or expected hypothesis;
- model run ID;
- tokens or duration;
- hidden-test result;
- protocol-violation result.

The presence of generated code may reveal process signals. Those unavoidable signals are documented rather than claimed to be blinded.

## Procedure

- Independent review before discussion.
- Production merge decision plus anchored rubric.
- Concrete findings required for low or rejecting scores.
- Disagreements preserved.
- Adjudication adds a shared interpretation but does not erase original ratings.

`TODO`: Freeze reviewer identities, assignment seed, material renderer, and adjudication owner before generating review packets.
