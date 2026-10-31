# ADR 0039: Message Records Outside Message Metadata

- Status: Accepted
- Date: 2026-10-29

## Context

A review of the chat runtime tables found that a message's `metadata` column carries lists that grow while a turn runs:

- `model_invocations` holds about 22KB per message, and each call copies the full system prompt.
- Every change rewrites the whole column. 1,088 messages had 7,499 metadata updates, with no concurrency control.
- List endpoints load the column even when they only show the text.

Moving every key into its own column would need a migration for each new key. Moving the whole column into one side table would keep the whole-document rewrite.

## Decision

1. **A generic records table.** `chat_runtime_message_records` holds one row per list entry: `message_id`, `organization_id`, `kind`, `record_id`, `sequence` and a `payload` (jsonb).
   - `kind` is the metadata key the list used to live under, such as `model_invocations`.
   - `(message_id, kind, record_id)` is unique. Writing an entry again merges its payload into the stored one, so a call's start and its usage can be written separately.
   - Rows are deleted with their message.
2. **System prompts stored once.** `chat_runtime_system_prompts` keys each prompt by its SHA-256. A call record keeps only `user_system_prompt_hash`.
3. **Model calls are the first kind.** A model call is written as a record. The message metadata keeps the call's place in `runtime_timeline` and `model_invocation_count`. If the record cannot be written, the call is kept in metadata as before.
4. **Reading.** Only the runtime log paths read records back. They put them under `model_invocations` after any calls an older message still keeps in metadata, with system prompts restored. Paths that write the message back do not, so records never return to metadata.
5. **Next steps.** The plan was to move skill calls, the timeline and the audit ledgers to records the same way, update the remaining metadata per key, and remove duplicate data and indexes. ADR 0040 measured again after this step and removes only the duplicates.

## Consequences

- A message's metadata no longer grows with each model call, and a system prompt is stored once however many calls use it.
- Older messages keep their calls in metadata and still show them; there is no backfill.
- Reading a runtime log costs one more query per page of messages.
