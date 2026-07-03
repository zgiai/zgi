---
name: agent-knowledge
description: Retrieve from knowledge bases explicitly configured on the current Agent.
when_to_use: Use this skill when a published or agent-scoped assistant needs context from its configured knowledge bases.
provider_type: builtin
provider_id: knowledge
runtime_type: tool
tools:
  - retrieve_agent_knowledge
max_calls_per_turn: 20
timeout_seconds: 30
display:
  icon: library
  category: knowledge
  label:
    en_US: Agent Knowledge
    zh_Hans: 智能体知识库
  description:
    en_US: Retrieves only from knowledge bases bound to the current Agent configuration.
    zh_Hans: 仅从当前智能体配置绑定的知识库中检索上下文。
  when_to_use:
    en_US: Use for Agent answers that need configured knowledge base retrieval.
    zh_Hans: 当智能体回复需要检索已绑定知识库时使用。
  tags:
    en_US:
      - Knowledge
      - Agent
    zh_Hans:
      - 知识库
      - 智能体
---

# Agent Knowledge Skill

Use this skill to retrieve context only from knowledge bases configured on the current Agent.
Knowledge access is authorized when the editor binds the knowledge bases to the Agent, and runtime retrieval uses that Agent binding grant.

## Workflow

1. Call `retrieve_agent_knowledge` when the answer depends on the Agent's configured knowledge, product facts, policy, documentation, or other long-lived source material.
2. Use a concise query derived from the user's intent. Do not enumerate all user-accessible knowledge bases.
3. Do not ask for, guess, or pass dataset IDs. The backend reads configured knowledge bases from the Agent config.
4. Inspect `status`, `source_summary`, `context_blocks`, and scores before answering:
   - If `status` is `success` and the retrieved blocks answer the user, answer from those blocks.
   - If results are missing, weak, or off-topic, rewrite the query once using clearer entities, synonyms, or constraints from the user question and retry.
   - After two retrieval attempts, if the relevant answer is still unclear, ask the user for clarification or say that no configured relevant knowledge was found.
5. When the user asks for original wording, definitions, synopsis text, policy clauses, exact wording, or "what does it say", quote or closely excerpt the retrieved source text first and cite the source. Summarize only when the user asks for a summary or the original text is too long.
6. When retrieved context is used, cite source names from `source_summary` or `retriever_resources` when useful.
7. Never expose internal IDs such as dataset ID, document ID, or segment ID to the user.
8. Never invent a knowledge-base answer when no relevant configured context was found.

## Tool Usage

`retrieve_agent_knowledge` accepts:

- `query`: the user question or refined search query.
- `top_k`: optional maximum retrieved chunks. Defaults to 5 and is capped at 20.
- `retrieval_mode`: optional `hybrid`, `vector`, or `graph`. Omit it for the default hybrid mode; use `graph` only for relationship or entity questions.
