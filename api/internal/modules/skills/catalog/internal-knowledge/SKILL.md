---
name: internal-knowledge
description: Discover and retrieve from knowledge bases the current AIChat user can access.
when_to_use: Use this skill when an internal AIChat answer needs factual context from workspace knowledge bases.
provider_type: builtin
provider_id: knowledge
runtime_type: tool
tools:
  - list_accessible_knowledge_bases
  - retrieve_knowledge
max_calls_per_turn: 10
timeout_seconds: 30
display:
  icon: library
  category: knowledge
  label:
    en_US: Internal Knowledge
    zh_Hans: Internal Knowledge
  description:
    en_US: Finds accessible workspace knowledge bases and retrieves selected context.
    zh_Hans: Finds accessible workspace knowledge bases and retrieves selected context.
  when_to_use:
    en_US: Use for internal AIChat questions that need knowledge base retrieval.
    zh_Hans: Use for internal AIChat questions that need knowledge base retrieval.
  tags:
    en_US:
      - Knowledge
      - Retrieval
    zh_Hans:
      - Knowledge
      - Retrieval
---

# Internal Knowledge Skill

Use this skill to answer internal AIChat questions with context from knowledge bases the current user can access.

## Workflow

1. First call `list_accessible_knowledge_bases` with a short query derived from the user request.
2. Select only the most relevant returned `dataset_id` values.
3. Call `retrieve_knowledge` with the user query and the selected `dataset_ids`.
4. Do not invent, guess, or use dataset IDs that were not returned by the list tool.
5. If no relevant accessible knowledge base is listed, answer from available conversation context and say no relevant accessible knowledge base was found.
6. When retrieved context is used, cite source names from `retriever_resources` when useful.

## Tool Usage

`list_accessible_knowledge_bases` accepts:

- `query`: optional search text for narrowing candidate knowledge bases.
- `limit`: optional maximum number of candidates.

`retrieve_knowledge` accepts:

- `query`: the user question or search query.
- `dataset_ids`: required selected knowledge base IDs returned by the list tool.
- `top_k`: optional maximum retrieved chunks.
- `retrieval_mode`: optional `hybrid`, `vector`, or `graph`.
