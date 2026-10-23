# ADR 0007: DigitalHuman Per-User Connector Credentials

- Status: Accepted
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

The DigitalHuman's managers choose the mode for each binding in its Connector settings, and the choice takes effect when the version is published. Existing bindings are `shared`. Their behavior does not change.

### Per-user credentials are resolved from the requester

For a `per_user` slot, the runtime resolves the credential from the run's verified requester, never from model output or client fields:

- **Workspace member in the console.** One of the member's own Personal Connections, selected once for this DigitalHuman and slot. Existing ownership checks apply: a member never uses another member's Connection.
- **External end user.** A Connection owned by the end user's instance (ADR 0006). Connection ownership is extended from accounts to instances. An instance Connection is usable only by runs of that instance. This includes signed-in WebApp users: they are ZGI accounts, but their Personal Connections belong to their own workspace and are not used by a DigitalHuman of another workspace.
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

- **External API.** `POST /api/connect/v1/end-users/{user}/connections` (ADR 0010) lets the integrating server submit a key-based credential for an end user. `GET` lists connection health and `DELETE` removes one. The integrator's API key is the trust anchor, and the subject is derived from `user` as in the chat endpoints.
- **WebApp.** A hosted connection page lets a signed-in end user enter a key for their instance. Anonymous sessions have no instance (ADR 0006) and cannot connect.
- OAuth providers are outside this decision. The first implementation supports key-based Connectors only.

### Management and lifecycle

- Operators see, per instance, which slots are connected and their health. They never see credentials. They can force a disconnect.
- Erasing an instance deletes its Connections and makes their credentials unusable. Removing a ZGI connection does not revoke provider-side authorization, as today.
- Credentials use the existing Connector keyring, are write-only, and are never returned, logged, or copied into run records.
- End-user quotas of the Runtime Workspace (ADR 0010) apply to runs that use `per_user` operations.
- Creation, use failures, disconnection, and erasure of instance Connections are audit events with identifiers and enum values only.

### Resolved questions

| Question | Decision |
|---|---|
| May a `per_user` slot fall back to `shared`? | No. A slot has one mode. A DigitalHuman that needs both binds two slots, one per mode, so the acting identity is always stated by the slot the model chose. An external API end user's `per_user` credential always comes from their own instance; without one, the operation asks them to connect. |
| May one end user reuse a Connection across DigitalHumans? | Members: yes, a Personal Connection is theirs and can be selected for slots of several DigitalHumans. External end users: no, an instance Connection belongs to one instance, and instances are per DigitalHuman (ADR 0006). |
| When are OAuth Connectors supported? | Not in this decision. Key-based Connectors first; OAuth needs a hosted consent flow per instance and is a separate decision. |

## Implementation notes

These notes record where the first implementation refines the decision above.

- **Owner of an end user's Connection.** An end user's Connection is owned by their Runtime Workspace end-user subject. Connector rows carry `owner_kind` (`account` or `end_user`). The account foreign keys became a guard that requires the owner, and the acting principal, to be an account or such a subject. Accounts stay undeletable while Connector rows reference them. A Connection is reached only through a per-user slot of one DigitalHuman, so it still serves one DigitalHuman. Erasing the end user destroys it (ADR 0010).
- **Slots.** A per-user slot is a Connector binding row with `credential_mode: per_user`, a stable slot key as its resource id, and no Connection of its editor. When a requester runs the published version, they get their own Target Binding to the slot for the Connection they selected. Discovery and dispatch then use it like any binding the requester owns. A selection table records the Connection each requester chose per slot.
- **Shared slots act for every requester.** A shared binding delegated to a host audience now resolves at dispatch with its owner's credential, for any requester the run authorizes.
- **The requester confirms.** A confirmation is decided by the run's requester, in both modes: the console member, the API end user (through a connector-confirmation endpoint), or the channel sender.
- **Reads need no confirmation.** Trusted-low operations, such as reads, run without confirmation on DigitalHumans, as on Chat and Agents. Operations with an external effect still need a fresh confirmation each time.
- **Setup.** `GET`, `POST`, and `DELETE` on `/api/connect/v1/end-users/{user}/connections` list, connect, and disconnect an end user's per-user slots. Keys are verified with the same key setup as members, and never returned. WebApp setup is not built yet, since the WebApp is unchanged for now (ADR 0010). Console members choosing their own Connection for a slot is a follow-up.

## Consequences

- One DigitalHuman can act as itself and for each requester in the same run, with the acting identity always explicit.
- End users can bring their own accounts without becoming ZGI accounts, and their credentials live and die with their instance.
- The Connector ownership model gains a second owner type. Every credential-opening path must check owner type and instance status, which needs tests for each surface.
- Workspace members can adopt `per_user` slots before instances exist, because they already own Personal Connections.
- `per_user` slots ship behind `ZGI_DIGITAL_HUMAN_PER_USER_CONNECTORS_ENABLED`, which defaults to off.
- Every credential-opening path gains an owner-type and instance-status check, which is the main review and test burden of this change.
