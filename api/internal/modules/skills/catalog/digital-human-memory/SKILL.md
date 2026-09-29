---
name: digital-human-memory
description: Retrieve relevant long-term memory owned by the current DigitalHuman when the current question may depend on durable context.
when_to_use: Use when the answer may depend on a durable DigitalHuman fact, decision, preference, experience, open loop, or reflection. Do not call for ordinary questions that do not need long-term context.
provider_type: builtin
provider_id: digital_human_memory
runtime_type: tool
tools:
  - retrieve_digital_human_memory
supported_callers:
  - digital_human
max_calls_per_turn: 3
timeout_seconds: 5
display:
  icon: brain
  category: knowledge_retrieval
  scenarios:
    - general
    - business_operations
  label:
    en_US: DigitalHuman Memory
    zh_Hans: 数字人长期记忆
  description:
    en_US: Designed for retrieving relevant durable memory for the current DigitalHuman without exposing its internal knowledge projection.
    zh_Hans: 适用于根据当前问题召回数字人的相关长期记忆，内部知识投影对用户不可见。
  when_to_use:
    en_US: Use when a response may depend on durable DigitalHuman context.
    zh_Hans: 当回答可能依赖数字人的长期事实或工作上下文时使用。
  tags:
    en_US:
      - Memory
      - Retrieval
    zh_Hans:
      - 长期记忆
      - 召回
---

# DigitalHuman Memory Skill

Use this skill only when the current question may depend on durable context owned by the current DigitalHuman.

## Workflow

1. Decide from the meaning of the current question whether durable memory is relevant.
2. Build a concise query describing the fact or context needed, rather than copying the entire user message.
3. Call `retrieve_digital_human_memory` with the query. Use `top_k` only when the default result count is insufficient; keep it within the backend limit.
4. Treat results as fallible context. Respect memory scope, revision, and confidence. If results are empty or weak, do not invent a memory.
5. Never provide or request an internal knowledge base ID, DigitalHuman ID, organization ID, workspace ID, or permission filter.
6. Do not silently fall back to the most recent memories when semantic retrieval returns no result.
