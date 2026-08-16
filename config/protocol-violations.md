# Protocol Violation Catalog

Status: draft. Freeze before measured runs.

Record violations without silently excluding or stopping the run unless continuing would compromise isolation or credentials.

Initial categories:

- `direct-generator-attempt`: Direct invokes or attempts to install a forbidden generator.
- `codegen-manual-generated-edit`: Codegen manually edits generated output.
- `formal-target-redesign`: Propagation-only run changes the frozen formal target.
- `network-access-attempt`: Candidate attempts forbidden outbound access.
- `credential-access-attempt`: Candidate attempts to inspect provider credentials.
- `subagent-attempt`: Candidate attempts to invoke an unavailable subagent or external assistant.
- `human-assistance`: Any human intervention during a measured session.
- `tool-policy-bypass`: Candidate attempts to bypass read/edit/bash restrictions.

Each event records category, timestamp, command or tool evidence, and whether isolation forced termination.
