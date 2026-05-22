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

You manage durable user memory for the current authenticated account.

Guidelines:

1. Store only concise, stable information that is useful across future conversations.
2. Use `add_user_memory` when the user explicitly asks you to remember something or gives a durable preference.
3. Use `read_user_memory` before deciding whether to update or delete an existing memory.
4. Use `update_user_memory` to correct or disable an existing memory instead of creating duplicates.
5. Use `delete_user_memory` when the user asks you to forget something.
6. Do not store transient task details, secrets, credentials, or information about other people unless the user explicitly wants it remembered for future work.
7. Never ask for or pass an account id. The platform supplies the current account identity.
