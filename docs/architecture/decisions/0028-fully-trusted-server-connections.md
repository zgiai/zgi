# ADR 0028: A Server Connection Can Be Fully Trusted

- Status: Accepted
- Date: 2026-10-28
- Builds on: ADR 0026 (read-only commands without approval), ADR 0027 (scheduled runs inspect only)

## Context

Deploying or debugging is a chain of changes. Even with read-only commands running freely, every change still asks. One real deployment needed six approvals.

On a test server, or a dedicated account with limited permissions, those approvals protect little. Mature coding agents offer a mode that stops asking altogether, and leave the choice to the person. Some people also want their scheduled tasks to repair a server, not only inspect it.

## Decision

A DigitalHuman's SSH binding gets two more approval policies. Each one turns on only after the owner accepts a risk confirmation that names its consequences.

- **`trusted`**: every call runs without asking while the connection's owner is the person in a console conversation. That includes changes, shell commands, and service actions.
  - The run carries the account of the person present only for a console conversation started by an account.
  - Conversations by anyone else, channel messages, and API calls still ask.
  - Scheduled runs still follow ADR 0027.
- **`trusted_unattended`**: in addition, the owner's scheduled and background task runs make any call unattended, without a task grant. The background authorizer still restores the live task run and checks that its owner owns the connection. Only the grant consumption is skipped.

Both policies are stored as authorization modes of the binding's constraint. Like Agent direct authorization, they are pinned to the connection revision, credential version, and contract digest they were granted for. Reconnecting, rotating the credential, or a new connector contract returns the binding to asking until the owner trusts it again.

Unchanged:

- forbidden commands are refused before anything runs;
- every call records its decision and outcome;
- the connection card shows the trust while it is on.

## Consequences

- An owner can let the specialist deploy and repair without approvals, in person or on a schedule, knowing the risk.
- A trusted connection is only as safe as its SSH account. The confirmations say so and recommend a test server or a limited account.
- Channel and API users can never benefit from someone else's trust.
