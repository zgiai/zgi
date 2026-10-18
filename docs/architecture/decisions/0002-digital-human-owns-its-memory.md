# ADR 0002: DigitalHuman Owns Its Memory

- Status: Accepted
- Date: 2026-08-18

## Context

A DigitalHuman needs continuity across channels, schedules, and collaborators. Partitioning long-term memory by the current user would fragment that continuity and make group conversations produce inconsistent identities.

## Decision

Long-term memory is owned by `digital_human_id`, within its organization and workspace. It is not partitioned by user.

Memory has two classes:

- `self`: identity, experience, decisions, open loops, and reflection owned directly by the DigitalHuman; it has no business scope.
- `business`: durable facts and experience within an explicit workspace, project, customer, campaign, or channel scope.

The initiating principal is recorded only as provenance and audit data. It is not part of the memory key or retrieval partition. Every mutation is idempotent, evidence-backed, revision-aware, and protected by configuration revision and memory epoch fences.

Business state, knowledge documents, artifacts, and credentials remain separate stores referenced by memory when necessary; they are not copied into the memory store.

## Consequences

- The same DigitalHuman presents consistent continuity in direct messages, group conversations, email, and scheduled runs.
- Authorization still determines which business scopes a run may read or write.
- User-specific personalization must be modeled explicitly outside long-term DigitalHuman memory if it is introduced later.
- Existing Agent and user-memory storage boundaries are not reused as DigitalHuman ownership keys.
