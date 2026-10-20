# ADR 0006: DigitalHuman End-User Instances

- Status: Proposed
- Date: 2026-10-20
- Builds on: ADR 0001, ADR 0002, ADR 0004, ADR 0005

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
| `quota_policy` | Optional per-instance limits |
| `first_seen_at`, `last_active_at` | Activity, updated at run admission |

The pair (`digital_human_id`, `subject_id`) is unique. The instance is created lazily at run admission for `external_end_user` runs and is fixed on the run, like the source trust level.

### Instance memory

Instance memory is a business scope of type `instance` with `restricted` visibility, using the model from ADR 0004:

- An `external_end_user` run may write only to its own instance scope. ADR 0004's rule that external end users never write DigitalHuman memory still holds for `self` and for every other scope.
- An instance run reads `self` memory and its own instance scope. It never reads `workspace` memory, another instance, or channel scopes.
- Instance memory never flows into `self` memory automatically. An operator may promote an entry explicitly, using the existing promotion permission, after reviewing it.
- Relationship memory stays the store for facts about the person. It is linked to the instance by its existing (workspace, subject, DigitalHuman) key, so instance management covers both.

### Tasks, conversations, and files

- Tasks created in an instance are owned by the DigitalHuman with the instance recorded as their requester context, as ADR 0005 anticipated. ADR 0005's visibility rules apply within the instance. Task center managers see instance tasks and can filter by instance.
- Conversations, files, and runs gain a nullable `instance_id` so they can be listed, exported, and erased per instance. Existing subject keys stay authoritative for access.

### WebApp end users

- An authenticated WebApp end user, identified by a host-signed user token, maps to a durable instance.
- An anonymous WebApp session maps to an ephemeral instance that expires with the session and never writes instance memory. This keeps anonymous traffic from accumulating unmanageable state.

### Channels

Channel conversations keep the `channel` scope from ADR 0004. Per-member instances inside channels are outside this decision.

### Operator management

Console, for users with the DigitalHuman update permission:

- list instances with label, issuer, status, first and last activity, conversation, task, and memory counts, and usage;
- suspend and resume an instance. A suspended instance rejects new runs and pauses its scheduled tasks;
- export an instance;
- erase an instance. Erasure removes conversations, messages, files, instance memory, relationship memory, and tasks, and keeps a tombstone that records the subject identifier, time, and operator only;
- set per-instance quota limits.

External API, for server-side integrations using an API key:

- `GET /end-users/{user}` returns the instance status and usage;
- `POST /end-users/{user}/export` exports it;
- `DELETE /end-users/{user}` erases it.

These endpoints derive the subject from `user` exactly as the chat endpoints do, so a key can manage only end users it can already reach.

### Audit

Instance creation, suspension, export, erasure, quota changes, and memory promotion from an instance are audit events. They carry identifiers and enum values only.

## Consequences

- External deployments get per-end-user continuity without weakening the shared memory rules of ADR 0004.
- Operators can answer "who uses this DigitalHuman, how much, and what does it remember about them", and can fulfil export and erasure requests per end user.
- The DigitalHuman's own identity and memory stay shared, so improvements to it reach every instance immediately.
- Run admission gains one lookup or insert. Instance creation must be idempotent and safe under concurrent first requests.
- Existing data needs no backfill to keep working. Instances are created lazily, and a one-time job links existing conversations, files, and tasks to instances by subject so operators can see historical end users.
- Instance memory ships behind `ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_ENABLED`, and the management surface behind `ZGI_DIGITAL_HUMAN_INSTANCES_ENABLED`. Both default to off.
- Per-member instances in channels, cross-DigitalHuman identity for the same end user, and billing per instance are outside this decision.
