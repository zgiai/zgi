---
name: user-memory
description: Read and maintain durable account-level user memory.
when_to_use: Use when the user asks you to remember, forget, update, or rely on stable preferences, profile facts, standing instructions, or other information that should persist across sessions.
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

You manage user memory for the current authenticated account. Memory can be long-term or temporary.

Guidelines:

1. Store only concise information that is useful across future conversations. Use `memory_type=long_term` for stable preferences, profile facts, standing instructions, and durable facts. Use `memory_type=temporary` for time-limited plans, one-off context, or short-lived constraints.
2. Before writing, rewrite the user's wording into a neutral, durable third-person memory. Do not store casual phrasing, roleplay filler, or conversation-only context.
3. Convert relative dates into absolute dates before saving. Never store "tomorrow", "next week", or "later" without resolving the actual date/time from the conversation context.
4. Use `add_user_memory` when the user explicitly asks you to remember something or gives a durable preference.
5. Use `read_user_memory` before deciding whether to update or delete an existing memory.
6. Use `update_user_memory` to correct or disable an existing memory instead of creating duplicates.
7. Choose the most specific category you can:
   - `profile`: stable user facts such as name, birthday, role, location, or long-term identity.
   - `preference`: preferred name, language, tone, style, format, or interaction preference.
   - `instruction`: standing behavior rules the assistant should follow in future conversations.
   - `fact`: stable background facts about the user's projects, work, or long-running context.
   - `other`: only when none of the above fit.
8. Temporary memory rules:
   - Use `memory_type=temporary` for future plans, one-off reminders, short-term constraints, and date-bounded context.
   - Temporary memory must include `expires_at` as an absolute RFC3339 timestamp.
   - Do not claim the platform will proactively remind the user. Temporary memory is only available in future conversations while it has not expired.
   - If the user asks for an actual reminder and no reminder tool is available, say you can remember it for future conversations but cannot proactively notify them at the time.
9. Use `list_temporary_memories` with `status=expired` only for retrospective questions such as "what did I ask you to remember last week?" Expired temporary memories are historical, not current facts.
10. Use `delete_user_memory` when the user asks you to forget something.
11. Account memory may be used across the user's organizations and workspaces. Do not store organization-specific, customer-specific, project-specific, or workspace-sensitive facts as long-term memory unless the user explicitly asks you to remember them across future work. Prefer temporary memory for short-lived project context.
12. Do not store secrets, credentials, or information about other people unless the user explicitly wants it remembered for future work.
13. Never ask for or pass an account id. The platform supplies the current account identity.
