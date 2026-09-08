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

Both switches are fail-closed in the example environment files:

```text
ZGI_RELATIONSHIP_MEMORY_ENABLED=false
ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED=false
```

Run the schema migration and validate the backfill before enabling the runtime switch in a test environment. The legacy Agent tables remain available as read-only rollback data after cutover. While the runtime switch is disabled, profile-management writes are rejected so the legacy and relationship-memory stores cannot silently diverge.
