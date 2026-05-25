---
name: agent-knowledge
description: Retrieve from knowledge bases explicitly configured on the current Agent.
when_to_use: Use this skill when a published or agent-scoped assistant needs context from its configured knowledge bases.
provider_type: builtin
provider_id: knowledge
runtime_type: tool
tools:
  - retrieve_agent_knowledge
max_calls_per_turn: 8
timeout_seconds: 30
display:
  icon: library
  category: knowledge
  label:
    en_US: Agent Knowledge
    zh_Hans: Agent Knowledge
  description:
    en_US: Retrieves only from knowledge bases bound to the current Agent configuration.
    zh_Hans: Retrieves only from knowledge bases bound to the current Agent configuration.
  when_to_use:
    en_US: Use for Agent answers that need configured knowledge base retrieval.
    zh_Hans: Use for Agent answers that need configured knowledge base retrieval.
  tags:
    en_US:
      - Knowledge
      - Agent
    zh_Hans:
      - Knowledge
      - Agent
---

# Agent Knowledge Skill

Use this skill to retrieve context only from knowledge bases configured on the current Agent.

## Workflow

1. Call `retrieve_agent_knowledge` directly with the user query.
2. Do not enumerate all user-accessible knowledge bases.
3. Do not ask for, guess, or pass dataset IDs. The backend reads configured knowledge bases from the Agent config.
4. If the Agent has no configured knowledge bases or no relevant results, answer clearly that no configured relevant knowledge was found.
5. When retrieved context is used, cite source names from `retriever_resources` when useful.

## Tool Usage

`retrieve_agent_knowledge` accepts:

- `query`: the user question or search query.
- `top_k`: optional maximum retrieved chunks.
- `retrieval_mode`: optional `hybrid`, `vector`, or `graph`.
