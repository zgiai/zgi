# User relationship memory

ZGI stores personal memory as a relationship between one user subject and one consumer. Agent slot definitions and DigitalHuman policies remain consumer-owned; saved values, settings, export, and deletion rights belong to the user relationship.

## Identity and isolation

- Signed-in calls use the account UUID as the subject.
- Agent API calls keep the request field named `user`. The trimmed, case-sensitive opaque value is converted to a deterministic UUID scoped by workspace and Agent; the raw value is not stored or included in audit events or request logs.
- API key rotation does not change the derived subject. The same key may use multiple `user` values, which remain isolated.
- External channel, scheduled DigitalHuman, and AIChat invocations cannot resolve or access relationship memory in P0.

## Runtime policy

New relationships allow explicit remember/correct/forget operations and disable automatic capture. Runtime access requires the global feature switch, an enabled published consumer contract, an active relationship, and an eligible invocation surface. Automatic capture additionally requires the global automatic-extraction switch, consumer permission, and the user's relationship setting.

Agent values must match enabled published slots. DigitalHuman values use semantic keys and one of `profile`, `preference`, `constraint`, or `relationship`; project, customer, channel, credential, and document-body data are rejected. At most 4,000 characters are injected as explicitly marked untrusted user data.

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
