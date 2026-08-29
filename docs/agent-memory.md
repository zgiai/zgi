# Agent Memory

Agent Memory stores up to five configured values per Agent and user. Existing values remain compatible with the upgraded runtime; migration assigns legacy source metadata without changing their content, IDs, ownership, or timestamps.

## Write behavior

- The conversation's main model receives a reserved `mutate_agent_memory` runtime tool. There is no keyword router or separate memory-planner model call. Explicit remember, correct, and forget requests are validated against exact text from the current user message before an atomic write.
- When the Agent's `auto_extraction_enabled` setting and the global automatic-write fuse are both enabled, the main model may proactively save direct, stable user facts, preferences, instructions, and ongoing context into any enabled slot. It cannot proactively clear memory.
- Background extraction still runs after completed persisted turns become idle and supplements the main flow. It skips blocks already handled inline from the same messages and never scans messages older than the latest deletion cutoff or successful watermark.
- `ZGI_AGENT_MEMORY_INLINE_TOOLS_ENABLED` defaults to `true` and is an immediate fuse for synchronous memory tools. Models without verified function-calling support continue chatting normally and cannot claim that a memory change succeeded.
- `ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED` defaults to `false` and gates both proactive inline writes and background extraction. Disabling it does not affect explicit tool operations, direct edits, chat, or existing memory reads.
- Automatic writes can be undone for 24 hours if no later change has replaced their revision.

Memory is injected as untrusted user context. It cannot elevate privileges, authorize tools, override Agent or platform rules, or supersede the current request.

## End-user API

Signed-in WebApp users can view, correct, delete, export, and undo their own Agent Memory through `/console/api/webapps/{web_app_id}/memory` and its key, export, and operation subresources.

External Agent API calls expose the same operations under `/api/v1/agents/memory`. Every request must include a non-empty `user` query parameter. That identity is asserted by the integration that owns the API key; ZGI isolates data by its derived user scope but does not treat the value as a verified end-user identity.

Direct update requests may send `expected_revision`. Delete requests may send it as a query parameter. Revision conflicts return a conflict response instead of overwriting a newer value.

## Privacy and retention

Extraction jobs store message references rather than copied message text. Failed extraction jobs use bounded exponential backoff and stop after five attempts; completed, cancelled, and exhausted job records are deleted in bounded batches after 30 days. The 8K extraction input budget covers the complete model request, including slot definitions and current values. Runtime metadata and stream events record slot keys and revisions, not memory content. Audit rows retain action metadata for 180 days; automatic-write undo snapshots contain the prior value for at most 24 hours and are removed immediately on permanent deletion.

## Release gate

Keep the automatic-write fuse disabled until the bilingual evaluation reaches at least 95% automatic-write precision and 100% rejection for sensitive content, assistant-derived content, and illegal clear operations. The repository tests cover explicit remember/correct/forget operations, ordinary preferences, temporary and hypothetical statements, third-party facts, exact-evidence checks, prompt-injection boundaries, atomic rollback, idempotency, revision conflicts, deletion cutoffs, epoch invalidation, inline/background deduplication, and undo safety. Model-quality scoring must use the deployment's configured conversation and extraction models before enabling the fuse.
