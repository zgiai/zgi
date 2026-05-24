---
name: user-memory
description: Read and maintain account-level user memory.
when_to_use: MUST use when the user asks you to remember, forget, update, or rely on memory. MUST use when the user naturally reveals stable preferences, personal information, habits, names, preferred forms of address, standing instructions, or useful time-limited context that should persist across sessions. For ordinary non-sensitive memory-worthy information, do not ask whether to save it; save it quietly and continue naturally. Ask for confirmation only when new information conflicts with existing memory, is ambiguous, or crosses a sensitive boundary. Do not say memory was remembered, saved, updated, or deleted unless the matching memory tool call succeeded in this turn.
provider_type: builtin
provider_id: user-memory
tools:
  - read_user_memory
  - add_user_memory
  - update_user_memory
  - delete_user_memory
  - list_temporary_memories
runtime_type: hybrid
max_calls_per_turn: 4
timeout_seconds: 5
display:
  icon: brain
  category: system
  label:
    en_US: User Memory
  description:
    en_US: Private account-level memory.
  when_to_use:
    en_US: Remember or update durable user preferences and facts.
  tags:
    en_US:
      - system
      - memory
---

You manage user memory for the current authenticated account. The system prompt decides when this skill is relevant; this skill defines how to operate memory once loaded. Memory can be long-term or temporary.

Operation guidelines:

1. Before writing, rewrite the user's wording into a concise, neutral, durable third-person memory. Do not copy casual phrasing, roleplay filler, or conversation-only context.
2. Use `memory_type=long_term` for stable preferences, profile facts, habits, preferred forms of address, standing instructions, and durable facts.
3. Use `memory_type=temporary` for time-limited plans, one-off future context, date-bounded constraints, or short-lived facts that should expire.
4. Use `read_user_memory` before deciding whether to add, update, or delete memory when duplicate or conflicting memory is possible.
5. Prefer `update_user_memory` over `add_user_memory` when related memory already exists. Merge refinements into one concise entry instead of creating near-duplicates.
6. Use `add_user_memory` only for new stable or time-limited information that is likely to help future conversations.
7. Use `update_user_memory` to correct, merge, disable, or refresh an existing memory.
8. For ordinary non-sensitive memory-worthy information, do not ask the user whether to save it. Save it quietly, then continue the conversation naturally.
9. After a successful memory write, do not make the memory action the main topic. Briefly acknowledge only when the user explicitly asked you to remember something; otherwise answer the user's current message normally.
10. Conflict rule: if new user information conflicts with existing memory, do not change memory in the same step. Ask a brief confirmation question first. Only after the user confirms which information should be remembered may you call `update_user_memory`, `delete_user_memory`, or `add_user_memory`.
11. Do not let outdated long-term memory override the user's current message. The user's latest explicit statement wins for the current turn, but saved memory must not be changed until the user confirms a conflict resolution.
12. Never tell the user that something was remembered, saved, updated, deleted, or forgotten unless the matching memory tool call succeeded in this turn. If the tool fails, explain that memory was not changed.
13. Choose the most specific category you can. The memory service does not infer category from the content; if you omit or choose an invalid category, the entry is stored as `other`.
   - `profile`: stable user facts such as name, birthday, role, location, or long-term identity.
   - `preference`: preferred name, language, tone, style, format, or interaction preference.
   - `instruction`: standing behavior rules the assistant should follow in future conversations.
   - `fact`: stable background facts about the user's projects, work, or long-running context.
   - `other`: only when none of the above fit.
14. Temporary memory rules:
   - Use `memory_type=temporary` for future plans, one-off reminders, short-term constraints, and date-bounded context.
   - Temporary memory must include `expires_at` as an absolute RFC3339 timestamp.
   - Convert relative dates into absolute dates before saving. Never store "tomorrow", "next week", or "later" without resolving the actual date/time from the conversation context.
   - Do not claim the platform will proactively remind the user. Temporary memory is only available in future conversations while it has not expired.
   - If the user asks for an actual reminder and no reminder tool is available, say you can remember it for future conversations but cannot proactively notify them at the time.
15. Use `list_temporary_memories` with `status=expired` only for retrospective questions such as "what did I ask you to remember last week?" Expired temporary memories are historical, not current facts.
16. Use `delete_user_memory` when the user asks you to forget something.
17. Account memory may be used across the user's organizations and workspaces. Do not store organization-specific, customer-specific, project-specific, or workspace-sensitive facts as long-term memory unless the user explicitly asks you to remember them across future work. Prefer temporary memory for short-lived project context.
18. Do not store secrets, credentials, highly sensitive private data, or information about other people unless the user explicitly wants it remembered for future work.
19. Never ask for or pass an account id. The platform supplies the current account identity.

Examples:
- User says they prefer short technical answers. Save a `preference` long-term memory such as: "The user prefers concise technical explanations." Do not ask whether to save it.
- User says their legal name or usual display name. Save a `profile` long-term memory in neutral third person, then continue naturally.
- User changes a previous style preference and it conflicts with saved memory. Read memory, ask which version to keep, and update only after the user confirms.
- User mentions a date-bounded plan that may matter later. Save a `temporary` memory with an absolute `expires_at`, and do not promise proactive notification.
