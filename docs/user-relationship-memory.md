# User relationship memory

ZGI stores personal memory as a relationship between one user subject and one consumer: an Agent, a DigitalHuman, or AI chat. Memory instructions and switches remain consumer-owned; saved files, settings, export, and deletion rights belong to the user relationship.

## Identity and isolation

- Signed-in calls use the account UUID as the subject.
- Agent API calls keep the request field named `user`. The trimmed, case-sensitive opaque value is converted to a deterministic UUID scoped by workspace and Agent; the raw value is not stored or included in audit events or request logs.
- API key rotation does not change the derived subject. The same key may use multiple `user` values, which remain isolated.
- External channel and scheduled DigitalHuman invocations cannot resolve or access relationship memory.
- AI chat memory (ADR 0021) belongs to the signed-in account alone, across organizations. Its relationship has no organization or workspace and uses a fixed consumer ID. Only the main AI chat and the console's contextual assistant use it.

## Runtime policy

New relationships allow explicit remember/correct/forget operations and start with automatic capture on (ADR 0019). Runtime access requires an enabled consumer contract, an active relationship, and an eligible invocation surface. Automatic capture additionally requires the deployment fuse `ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED` (on unless set to false), the consumer's setting, and the user's relationship setting.

Memory is files the model names and organizes (ADR 0017, ADR 0019), bounded by 10,000 characters per relationship; a full memory moves older content to the person's archive. Credentials, identifiers, and other sensitive values are rejected. Memory is injected as explicitly marked untrusted user data.

DigitalHuman-owned memory and user relationship memory use separate stores and tools. Neither store promotes or copies data into the other.

## Management

Signed-in users manage account-owned relationships at `/profile/memory`. The backing `/console/api/user-memories/relationships` API supports organization-wide listing, settings, revision-aware edits, deletion, clear, export, permanent deletion, and 24-hour undo. Relationships from workspaces the user can no longer access remain exportable and permanently deletable, but cannot be edited.

The external Agent memory API keeps its existing `user` query parameter and adds per-relationship settings endpoints. Audit and telemetry use bounded enum attributes, IDs, revisions, and result codes only; they never include memory bodies or raw API `user` values.

## Rollout

Relationship memory is always on (ADR 0016). Schema migrations move any Agent memory still in the legacy Agent tables into it; the legacy tables keep their rows but are no longer written.

Automatic writes are on by default. A deployment turns them off for everyone with:

```text
ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED=false
```

Each Agent or DigitalHuman, and each person, can still turn automatic memory off for themselves. Turning memory on for an Agent turns its automatic memory on too.
