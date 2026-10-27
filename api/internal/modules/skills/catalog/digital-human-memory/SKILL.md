---
name: digital-human-memory
description: Read and change the current DigitalHuman's long-term memory, and the person's own memory, as files.
when_to_use: Use when the answer may depend on a durable DigitalHuman fact, decision, preference, experience, open loop, or reflection. Do not call for ordinary questions that do not need long-term context.
provider_type: builtin
provider_id: digital_human_memory
runtime_type: tool
tools:
  - memory_view
  - memory_search
  - memory_edit
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
    en_US: Designed for recalling, searching, and reading the DigitalHuman's durable memory as files, and saving what the conversation establishes, without exposing its internal knowledge projection.
    zh_Hans: 适用于以文件形式召回、查找和阅读数字人的长期记忆，并保存对话中确认的内容，内部知识投影对用户不可见。
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

1. The files most relevant to the current message are recalled at the start of the run, with an index of the others. Read an indexed file with `memory_view` when the answer may depend on it.
2. When nothing shown fits, search with `memory_search` before saying you do not know. Build a concise query instead of copying the whole user message.
3. Change memory only with `memory_edit`, in the directory the fact is about: `/me/` for your own knowledge, `/user/notes/` for your notes on the person, `/user/` for facts about the person themselves.
4. Treat memory as fallible context. Respect its revision, and if nothing fits, do not invent a memory.
5. Never provide or request an internal knowledge base ID, DigitalHuman ID, organization ID, workspace ID, or permission filter.
