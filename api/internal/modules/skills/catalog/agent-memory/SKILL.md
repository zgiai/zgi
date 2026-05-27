---
name: agent-memory
description: Read and update fixed memory slots for the current agent and user.
when_to_use: Use only in agent runtime when the current agent has organizer-defined memory slots that may help answer or should be updated from the current interaction. Never invent new memory keys.
provider_type: builtin
provider_id: agent-memory
supported_callers:
  - agent
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

Operation guidelines:

1. Use `read_agent_memory` before relying on or updating memory.
2. Only use keys returned by `read_agent_memory`. Never invent new keys.
3. Use `update_agent_memory` to update one existing key with concise content that fits its `max_chars`.
4. Use `clear_agent_memory` only when the user asks to remove or forget the value for an existing key.
5. Agent memory is long-term only. Do not store temporary reminders or date-bounded plans unless the configured key description explicitly asks for that kind of durable state.
6. Do not store secrets, credentials, or sensitive personal data unless the user explicitly asks and the configured key description is appropriate.
7. Do not mention memory changes unless the user asked about memory or the change is important to the answer.
