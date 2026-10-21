# ADR 0007: DigitalHuman Per-User Connector Credentials

- Status: Proposed
- Date: 2026-10-20
- Builds on: ADR 0004, ADR 0005, ADR 0006

## Context

Connections are personal. Each Connection belongs to one ZGI account, and there are no organization-shared credentials. A DigitalHuman uses a Connector when an editor binds one of their own Connections to it and, at publication, delegates that binding to host audiences (`workspace`, `api`, `channel`). At run time the delegated resolver authorizes each use against the run and opens the binding owner's credential with the binding's allowed operations.

As a result, every caller of a DigitalHuman acts through the editor's account. That is right when the DigitalHuman acts as itself, for example with a company mailbox. It does not cover a DigitalHuman that acts for each requester with the requester's own account: their mailbox, their repository, their calendar. External end users cannot hold Connections at all, because they are not ZGI accounts.

## Decision

### A binding declares a slot and a credential mode

A DigitalHuman Connector binding becomes a slot: a Connector, its allowed operations, and a credential mode. The mode is part of the published version.

| Mode | Credential | Acts as |
|---|---|---|
| `shared` | The editor's delegated Connection, as today | The DigitalHuman |
| `per_user` | A Connection supplied by the requester of the run | The requester |

Existing bindings are `shared`. Their behavior does not change.

### Per-user credentials are resolved from the requester

For a `per_user` slot, the runtime resolves the credential from the run's verified requester, never from model output or client fields:

- **Workspace member in the console.** One of the member's own Personal Connections, selected once for this DigitalHuman and slot. Existing ownership checks apply: a member never uses another member's Connection.
- **External end user.** A Connection owned by the end user's instance (ADR 0006). Connection ownership is extended from accounts to instances. An instance Connection is usable only by runs of that instance.
- **Runs without a requester.** The DigitalHuman's own background runs never resolve a `per_user` slot.
- **Channel participants.** Not supported in this decision. The slot is unavailable in channel runs.

When no credential is resolved, the slot's operations are presented to the model as needing a connection, together with a setup action for the requester. The runtime never falls back to a `shared` credential or to another Connection, because that would change who the action is performed as.

### Identity is explicit

Tool descriptions and approval cards state whether an operation acts as the requester or as the DigitalHuman. Confirmation rules for the requester's own account apply as for the requester's direct use.

### Data stays with the credential

Data read through a `per_user` credential is treated as the requester's data:

- in an instance run, it may be written only to that instance's memory scope;
- in a console run, it may be written only to memory the member could write without it under ADR 0004, and never to `workspace` memory automatically;
- it is never written to `self` memory without explicit promotion.

### Scheduled work

A task created in an instance or by a member runs with that requester's credential. If the credential is removed, or the instance is suspended or erased, the task's runs fail closed with a reason that asks the requester to reconnect.

### Setup for end users

- **External API.** `POST /end-users/{user}/connections` lets the integrating server submit a key-based credential for an end user. `GET` lists connection health and `DELETE` removes one. The integrator's API key is the trust anchor, and the subject is derived from `user` as in the chat endpoints.
- **WebApp.** A hosted connection page lets an authenticated end user enter a key. Anonymous sessions cannot connect.
- OAuth providers are outside this decision. The first implementation supports key-based Connectors only.

### Management and lifecycle

- Operators see, per instance, which slots are connected and their health. They never see credentials. They can force a disconnect.
- Erasing an instance deletes its Connections and makes their credentials unusable. Removing a ZGI connection does not revoke provider-side authorization, as today.
- Credentials use the existing Connector keyring, are write-only, and are never returned, logged, or copied into run records.
- Per-instance rate limits apply to `per_user` operations.
- Creation, use failures, disconnection, and erasure of instance Connections are audit events with identifiers and enum values only.

## Consequences

- One DigitalHuman can act as itself and for each requester in the same run, with the acting identity always explicit.
- End users can bring their own accounts without becoming ZGI accounts, and their credentials live and die with their instance.
- The Connector ownership model gains a second owner type. Every credential-opening path must check owner type and instance status, which needs tests for each surface.
- Workspace members can adopt `per_user` slots before instances exist, because they already own Personal Connections.
- `per_user` slots ship behind `ZGI_DIGITAL_HUMAN_PER_USER_CONNECTORS_ENABLED`, which defaults to off.
- Open for review: whether a `per_user` slot may declare an explicit, user-visible `shared` fallback, and whether one end user may reuse a Connection across DigitalHumans.
