---
name: agent-memory
description: Deprecated Agent memory skill. Agent chat now uses native runtime memory injection plus native update/clear tools.
when_to_use: Deprecated. Do not load this skill from Agent chat; Agent memory is managed by native runtime capability.
provider_type: builtin
provider_id: agent-memory
supported_callers:
  - deprecated
tools:
  - read_agent_memory
  - update_agent_memory
  - clear_agent_memory
runtime_type: hybrid
max_calls_per_turn: 4
timeout_seconds: 5
display:
  icon: brain
  category: system
  label:
    en_US: Agent Memory
    zh_Hans: 智能体固定记忆
  description:
    en_US: Agent-scoped fixed-slot memory.
    zh_Hans: 读取和更新当前智能体为用户配置的固定槽位记忆。
  when_to_use:
    en_US: Read or update configured memory keys for this agent.
    zh_Hans: 当需要读取或更新当前智能体已配置的用户记忆槽位时使用。
  tags:
    en_US:
      - system
      - memory
      - agent
    zh_Hans:
      - 系统
      - 记忆
      - 智能体
---

You manage memory for the current agent and current user. Memory keys are fixed by the agent organizer.

Deprecated: Agent chat no longer uses this hidden skill. Runtime reads memory directly into the system prompt and exposes native `update_agent_memory` / `clear_agent_memory` tools outside the skill loop. This document is kept only for compatibility with older traces or deployments.

Operation guidelines:

1. Saved Agent memory may already be provided in the system context. Use those saved values proactively when answering; do not wait for the user to explicitly remind you.
2. Use `read_agent_memory` before updating or clearing memory, or when saved memory is not shown but current values are needed.
3. Only use configured memory keys. Never invent new keys.
4. Use `update_agent_memory` to update one existing key with concise content that fits its `max_chars`.
5. Required memory write triggers: profile facts include the user's own name, preferred address, job role, team role, or stable identity changes; preferences include durable answer style, language, examples, length, layout, tone, or voice; standing instructions include durable interaction rules, how you should address the user, what the user calls you, agent persona/roleplay instructions, phrases such as "以后", "每次", "当我让你...时", or durable workflows like "先...再..."; project context includes ongoing projects, goals, responsibilities, or workstreams.
6. When a required write trigger appears and the matching key exists, call `read_agent_memory` then `update_agent_memory` before your final answer.
7. If you call `read_agent_memory` because of a required write trigger, a read-only result is incomplete. Continue with `update_agent_memory` before answering.
8. When the user provides stable profile facts, response preferences, standing instructions, or ongoing project context, update the matching key in the same turn even if the user did not explicitly ask you to remember it. This is required when a matching key exists.
9. Treat statements such as "I am responsible for X", "I am working on X", or "the goal is Y" as durable project context when a project/context key exists, and call `update_agent_memory` in the same turn.
10. Use `clear_agent_memory` when the user asks to remove or forget the value for an existing key. It is okay to call it after reading even if the current value is already empty.
11. Do not store agent identity, assistant persona, roleplay style, or what the user calls you in `profile`. Store those in `standing_instructions` when they are durable interaction rules, or `preferences` when they are only tone/style preferences.
12. Do not copy profile facts such as the user's real name, preferred name, job, or team into `standing_instructions`. If `standing_instructions` contains an addressing rule, keep it as the rule itself, not as a duplicate profile fact.
13. When the user changes their name, preferred address, job, or role, update `profile` only. Do not rewrite `standing_instructions` unless the user explicitly changes the collaboration rule, assistant persona, or addressing rule.
14. Agent memory is long-term only. Do not store temporary small talk, one-off events, or date-bounded plans unless the configured key description explicitly asks for that kind of durable state.
15. Do not store secrets, credentials, payment data, government IDs, or other sensitive personal data. If the user asks you to save sensitive data, politely decline without calling `update_agent_memory`.
16. Never tell the user that you remembered, recorded, updated, saved, cleared, or forgot memory unless the corresponding `update_agent_memory` or `clear_agent_memory` call succeeded in this turn.
17. Never say memory was updated after only `read_agent_memory`. Reading memory does not save changes.
18. Do not say that you will remember something later. Either update memory successfully in this turn, or answer without claiming it was saved.
19. Do not mention internal memory operations such as loading this skill, reading memory, or calling tools. After a successful memory change, confirm naturally and briefly only when useful.
