# ADR 0040: Chat Runtime Storage Cleanup

- Status: Accepted
- Date: 2026-10-29

## Context

ADR 0039 moved model calls out of message metadata and planned more steps:

- move skill calls, the runtime timeline and the audit ledgers to message records;
- update the remaining metadata per key, with an optimistic lock on whole writes;
- remove duplicated data and indexes.

After ADR 0039, the local data was measured again. Of about 33MB of metadata, model calls had been 24.5MB, about 22KB per message. Everything else together is about 7KB per message:

| Key | Messages | Average | Total |
|---|---|---|---|
| `skill_invocations` | 819 | 2.6KB | 2.1MB |
| `presentation` | 1,161 | 1.3KB | 1.5MB |
| `runtime_timeline` | 1,164 | 1.2KB | 1.3MB |
| `tool_governance_decisions`, `connector_confirmation_events`, `workflow_runs`, `connector_action_instances` | 26–55 each | 2.8–8.8KB | 0.1–0.2MB each |

The other duplication found:

- **Transcript previews.** Every transcript preview equalled its payload: 6,324 of 6,324 rows, about 3MB. Pruning now writes the preview and the payload together, and only attachment-reader receipts differ.
- **Duplicate indexes.** Four indexes kept from the `aichat_*` tables repeat a `chat_runtime_*` index exactly.
- **Duplicate snapshot key.** The turn start context was stored twice, as `turn_start_context` and `turn_initial_context_snapshot`.
- **No foreign key.** Messages had no foreign key to their conversation.
- **One query per entry.** Loading the transcript chain for a turn took one query per entry.

## Decision

1. **Transcript previews.**
   - A preview equal to its payload is stored as NULL and read back as the payload.
   - The migration clears existing duplicates.
2. **One query for the transcript chain.** The preview chain of a turn is read with one recursive query. The stop rules are unchanged:
   - stop after a given entry;
   - stop after the boundary of the newest summary;
   - stop at the root;
   - fail on a cycle, a missing parent or a chain longer than the limit.
3. **Duplicate indexes and keys.**
   - The four `aichat_*` duplicate indexes are dropped. Migrations run in a transaction, so `DROP INDEX` takes a short lock instead of running concurrently.
   - The turn start context is written only as `turn_start_context`. Older messages are still read from `turn_initial_context_snapshot`.
4. **Messages reference their conversation.**
   - The foreign key cascades on delete. Erasure and teardown already delete messages first.
   - It is added `NOT VALID` and validated in the same migration when no message lacks its conversation. Otherwise it is left unvalidated, so an orphan does not stop the upgrade.
5. **Not done.** The rest of the ADR 0039 plan stays as it is:
   - **Skill calls, the timeline and the ledgers stay in metadata.**
     - They are about 7KB per message in all.
     - They are working state: continuations, approvals, DigitalHuman runs, the runtime log and the web chat read them directly from the message, in about forty places.
     - Moving them would add a read to each of those paths for little space.
   - **No per-key updates and no optimistic lock.**
     - While a turn runs, metadata writes are scoped to its `runtime_run_id` lease, so there is one writer.
     - Approvals and continuations write while the message waits, under status guards.
     - The connector action ledger already survives stale writes. After ADR 0039, a whole write is about 4KB instead of about 26KB.
   - **List queries still load metadata.** The runtime log, agent history and chat lists show usage, skill calls and presentation from it.
   - **Prompt calibration stays on the message.** It is 340 bytes, and it is seeded from the message or the turn it answers, which keeps branches apart.

## Consequences

- The transcript table stores each message once instead of twice.
- A turn's context is loaded in one query however long the chain is.
- Each message write maintains four fewer indexes.
- A message can no longer point at a missing conversation.
- If later work makes metadata grow again, or adds writers that run alongside a turn, the records table from ADR 0039 is the place to move a list to.
