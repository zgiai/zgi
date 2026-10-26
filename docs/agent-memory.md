# Agent Memory

Agent memory keeps what an Agent should remember about each user as memory blocks: small files such as `/user/profile.md`, one per block the Agent defines. Blocks live in the user relationship memory store ([User Relationship Memory](user-relationship-memory.md)); the design is in ADR 0013 to ADR 0016.

An Agent defines up to five blocks, each with a name, a description, a character limit, and whether it is read-only or written only by hand. Templates for common scenarios (support, sales follow-up, tutoring, e-commerce after-sales, internal helpdesk) fill in blocks and memory instructions.

## Write behavior

- Saved blocks are rendered into the conversation as memory files within a share of the model's input budget. Memory is untrusted user context: it cannot raise privileges, authorize tools, override Agent or platform rules, or replace the current request.
- The conversation's main model gets the `memory_edit` and `memory_view` tools, plus searches over earlier conversations and the memory archive. Explicit remember, correct, and forget requests are checked against exact text from the current user message before an atomic write; one call with rejected edits may be followed by one corrected call.
- When the Agent's `auto_extraction_enabled` setting and the global automatic-write fuse are both on, the main model may also save direct, stable user facts, preferences, instructions, and ongoing context into blocks that allow automatic writes. It cannot proactively clear memory.
- Background consolidation runs after completed turns go idle. It reads the recent turns and the current blocks, rewrites blocks that need it, and moves older text to the archive. It follows the Agent's memory instructions, skips blocks already handled inline from the same messages, and never reads messages older than the latest deletion cutoff or watermark.
- `ZGI_AGENT_MEMORY_INLINE_TOOLS_ENABLED` defaults to `true` and is an immediate fuse for the synchronous memory tools. Models without verified function calling keep chatting and cannot claim that a memory change succeeded.
- `ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED` defaults to `false` and gates proactive inline writes and background consolidation. `ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED` is still accepted during rolling upgrades. Turning it off does not affect explicit memory requests, direct edits, chat, or memory reads.
- `ZGI_MEMORY_CONSOLIDATION_MODEL` picks the consolidation model; empty uses the conversation's model.

Every change is kept in the memory history with the content before and after, and can be reverted.

## End-user API

Signed-in WebApp users can view, correct, delete, export, and undo their own Agent memory through `/console/api/webapps/{web_app_id}/memory` and its key, export, history, archive, and operation subresources.

External Agent API calls expose the same operations under `/api/v1/agents/memory`. Every request must include a non-empty `user` query parameter. That identity is asserted by the integration that owns the API key; ZGI isolates data by its derived user scope but does not treat the value as a verified end-user identity.

Direct update requests may send `expected_revision`. Delete requests may send it as a query parameter. Revision conflicts return a conflict response instead of overwriting a newer value.

## Upgrading from slot memory

Earlier releases stored Agent memory as slots in `agent_memory_values`, behind `ZGI_RELATIONSHIP_MEMORY_ENABLED` and `ZGI_MEMORY_BLOCKS_ENABLED`. Both switches are gone. A schema migration copies anything still only in the legacy tables into the relationship store, keeping the newer copy of each value and honoring deletions; it is safe to run on a deployment that already used the relationship store. The legacy tables keep their rows for rollback but are no longer written.

## Privacy and retention

Extraction jobs store message references rather than copied message text. Failed jobs use bounded exponential backoff and stop after five attempts; completed, cancelled, and exhausted jobs are deleted in bounded batches after 30 days. Runtime metadata and stream events record block keys and revisions, not memory content.

## Release gate

Keep the automatic-write fuse off until the evaluation suite in `api/evals/memory` reaches at least 95% automatic-write precision and 100% rejection for sensitive content, assistant-derived content, and illegal clears, using the deployment's own conversation and consolidation models.
