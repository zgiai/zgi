# ADR 0006: DigitalHuman End-User Instances

- Status: Accepted
- Date: 2026-10-20
- Builds on: ADR 0001, ADR 0002, ADR 0004, ADR 0005
- Amended by: ADR 0010 (end-user management, quotas, and external access move to the Runtime Workspace)

## Context

A DigitalHuman is one subject (ADR 0001). It owns its memory (ADR 0002) and its tasks (ADR 0005), and source trust decides what each caller may change (ADR 0004).

The same DigitalHuman is also published to many external end users through the external API and the WebApp. Those deployments need two things the current model does not provide:

- Continuity per end user. ADR 0004 stops external end users from writing DigitalHuman memory, which is correct for shared memory, but it leaves the DigitalHuman with no working memory of its own for each end user. Relationship memory stores facts about the person, not the DigitalHuman's commitments, open loops, and decisions in that relationship.
- Operator control over end users. End-user data is already isolated: the external API derives a stable subject per workspace, DigitalHuman, and `user` value, and conversations, files, tasks, and relationship memory are keyed by that subject. But there is no record that represents "this DigitalHuman's relationship with this end user". Operators cannot list end users, see their usage, suspend one, export one, or erase one. Erasure is only possible conversation by conversation.

Cloning a DigitalHuman per end user would contradict ADR 0001 and fork configuration, versions, and bindings. Keeping end users only as an implicit subject key leaves them unmanageable.

## Decision

### An instance is a relationship container, not a new DigitalHuman

An end-user instance represents one DigitalHuman's relationship with one external end user. It belongs to the DigitalHuman domain and has no configuration of its own:

- identity, rules, roles, skills, and bindings always come from the DigitalHuman's published version;
- the instance only scopes the state produced in that relationship.

The DigitalHuman remains the single subject. Instances are how it keeps many relationships apart.

In code and APIs the concept is always called an end-user instance (`end_user_instance`, `EndUserInstance`), never just "instance": connector code already uses "binding instance" for a different thing.

### Record

`digital_human_end_user_instances` stores, per instance:

| Field | Meaning |
|---|---|
| `id` | Instance identifier |
| `organization_id`, `workspace_id`, `digital_human_id` | Owning DigitalHuman |
| `subject_kind`, `subject_id` | The existing user subject. The raw `user` value is never stored |
| `issuer_type`, `issuer_id` | How the end user arrived: an API key or a WebApp |
| `status` | `active`, `suspended`, or `erased` |
| `label` | Optional operator-set alias. Never derived from end-user input |
| `quota_policy` | Unused; quotas belong to the Runtime Workspace (ADR 0010) |
| `first_seen_at`, `last_active_at` | Activity, updated at run admission |

The pair (`digital_human_id`, `subject_id`) is unique. The instance is created lazily in run admission (`RuntimeService.SubmitInvocation`), in the same transaction that persists the run, for `external_end_user` runs that carry a durable user subject. It is fixed on the run, like the source trust level. Concurrent first requests resolve to one instance through the unique key.

### Instance memory

Instance memory is a business scope of type `instance` with `restricted` visibility, using the model from ADR 0004:

- An `external_end_user` run may write only to its own instance scope. ADR 0004's rule that external end users never write DigitalHuman memory still holds for `self` and for every other scope.
- An instance run reads `self` memory and its own instance scope. It never reads `workspace` memory, another instance, or channel scopes.
- Instance memory never flows into `self` memory automatically. An operator may promote an entry explicitly, using the existing promotion permission, after reviewing it.
- Relationship memory stays the store for facts about the person. It is linked to the instance by its existing (workspace, subject, DigitalHuman) key, so instance management covers both.
- Capacity and expiry are optional and off by default: an instance keeps any number of entries, and they never expire on their own.
  - `ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_MAX_ENTRIES` bounds the active entries per instance. A write beyond the limit is rejected as a policy rejection, and the model is told to update or clear an existing entry instead. Unset or `0` means no limit.
  - `ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_TTL` (a duration such as `2160h`) archives an entry that has not been written for that long. Archived entries are no longer recalled and stay visible to operators. Unset or `0` means no expiry.
  - Otherwise instance memory ends only when an operator clears it or erases the instance.

### Tasks, conversations, and files

- Tasks created in an instance are owned by the DigitalHuman with the instance recorded as their requester context, as ADR 0005 anticipated. ADR 0005's visibility rules apply within the instance. Task center managers see instance tasks and can filter by instance.
- Runs record `end_user_instance_id`. Conversations, files, and tasks are already keyed by the user subject, so they are listed, exported, and erased per instance by joining on that subject; they gain no instance column. Existing subject keys stay authoritative for access.

### WebApp end users

- A signed-in WebApp end user is a ZGI account. Their runs are `external_end_user` runs with the account's user subject, so they map to a durable instance like API end users. Signing in users of a host application with a host-signed token is outside this decision; if it is added later, it maps to an instance the same way.
- An anonymous WebApp session has no durable user subject: its identity is a random identifier in a session token that expires. It creates no instance and never writes instance memory. Its conversations keep the session's retention, and operators see anonymous traffic in aggregate only. This keeps anonymous traffic from accumulating unmanageable state.

### Channels

Channel conversations, direct or group, are a way of using the DigitalHuman itself, not end-user instances. They keep the `channel` scope from ADR 0004 and create no instance.

### Operator management

End-user management, quotas, and the external management endpoints belong to the Runtime Workspace (ADR 0010), which covers Agents and DigitalHumans alike. An instance is the DigitalHuman's panel of an end user there: its instance memory, tasks, and per-user connections. Suspending or erasing the end user suspends or erases the instance; erasure also removes its conversations, files, instance memory, relationship memory, and tasks, and keeps a tombstone that records identifiers only.

### Audit

Instance creation, suspension, erasure, and memory promotion from an instance are audit events. They carry identifiers and enum values only.

### Resolved questions

| Question | Decision |
|---|---|
| Does instance memory need a capacity limit or expiry? | Both exist and are set by environment variables; by default there is no limit and no expiry (see Instance memory). |
| Should the same end user share one identity across DigitalHumans? | Through a Runtime Workspace, yes: the end user is one subject for every bound application (ADR 0010). Each DigitalHuman still has its own instance for that subject, so one relationship never leaks into another. Through a DigitalHuman's own API key, the subject stays per DigitalHuman. |
| Do channel direct messages map to member instances? | No. A channel direct message is one way of using the DigitalHuman itself and keeps the `channel` scope from ADR 0004. |
| Is quota billing or rate limiting? | Billing in points, configured in the Runtime Workspace per end user (ADR 0010). |

### Delivery

Each step ships behind its flag and keeps existing behavior when the flag is off:

1. Instance record, creation in run admission, and `end_user_instance_id` on runs.
2. Instance memory scope and its read and write rules.
3. The DigitalHuman panel in Runtime Workspace end-user management (ADR 0010), suspension and erasure of instances, and audit events.
4. Per-user connector credentials (ADR 0007).

## Consequences

- External deployments get per-end-user continuity without weakening the shared memory rules of ADR 0004.
- Operators can answer "who uses this DigitalHuman, how much, and what does it remember about them", and can fulfil export and erasure requests per end user.
- The DigitalHuman's own identity and memory stay shared, so improvements to it reach every instance immediately.
- Run admission gains one lookup or insert. Instance creation must be idempotent and safe under concurrent first requests.
- Existing data needs no backfill to keep working. Instances are created lazily, and the migration creates instances for end users seen in earlier runs so operators can see historical end users.
- Instance memory ships behind `ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_ENABLED`, and the management surface behind `ZGI_DIGITAL_HUMAN_INSTANCES_ENABLED`. Both default to off.
- Per-member instances in channels and cross-DigitalHuman identity for the same end user are outside this decision.
