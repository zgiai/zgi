# ADR 0020: No Background Memory Consolidation for Now

- Status: Accepted
- Date: 2026-10-27
- Builds on: ADR 0013, ADR 0017, ADR 0019
- Amends: ADR 0013 (consolidation)

## Context

ADR 0013 gave Agents a second way to remember automatically. After a conversation went idle, a background job:
- read the recent turns and the person's memory files;
- asked a model which files to add to or rewrite;
- applied the edits.

The builder's "Preview" ran the same pass over a sample conversation.

It costs one more model call per idle conversation, on top of what the conversation itself uses. Its value over the in-turn proactive writes has not been measured since ADR 0017 changed the memory model. DigitalHumans never had it (ADR 0019).

## Decision

Remove background consolidation of a person's memory, for now:
- the scheduling after a completed turn;
- the `agentmemoryworker` package, which held the sweep, the runner, the consolidation prompt, and the preview;
- the builder's memory preview and its `POST /agents/:agent_id/memory/preview` route;
- `ZGI_MEMORY_CONSOLIDATION_MODEL`.

Automatic memory now means only proactive writes within a turn, for Agents and DigitalHumans alike. They are still gated by the consumer's own setting, the person's switch, and `ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED`.

The periodic cleanup of expired undo records, old memory events, and expired history stays, as `relationshipmemory.CleanupTask`. It also drops rows the retired consolidation queue left behind, after 30 days. The `user_memory_extraction_jobs` table stays for existing data.

A DigitalHuman's own memory (`/me/`, `/user/notes/`) still has its background extraction, which is not about the person's own memory.

## Consequences

- Automatic memory costs no extra model calls outside the conversation.
- What a person says in passing is remembered only if the model chooses to save it in that turn.
- Bringing consolidation back is a later decision, measured with the automatic suites in `api/evals/memory`.
