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
max_calls_per_turn: 20
timeout_seconds: 30
display:
  icon: library
  category: knowledge
  label:
    en_US: Internal Knowledge
    zh_Hans: 内部知识库
  description:
    en_US: Finds knowledge bases accessible to the current AIChat user and retrieves relevant context.
    zh_Hans: 查找当前 AIChat 用户可访问的知识库，并检索相关上下文。
  when_to_use:
    en_US: Use when an AIChat answer needs facts or source context from accessible knowledge bases.
    zh_Hans: 当 AIChat 回复需要引用可访问知识库中的事实或来源上下文时使用。
  tags:
    en_US:
      - Knowledge
      - Retrieval
    zh_Hans:
      - 知识库
      - 检索
---

# Internal Knowledge Skill

Use this skill to answer internal AIChat questions with context from knowledge bases the current user can access.

## Workflow

1. First call `list_accessible_knowledge_bases` with a short query derived from the user's topic, business domain, or explicitly named knowledge base.
2. Inspect `status`, `fallback_used`, and returned knowledge base names/descriptions before selecting datasets:
   - If `status` is `success`, select only the most relevant returned `dataset_id` values.
   - If `status` is `fallback`, treat the candidates as weak matches. Select a dataset only when its name or description is clearly related; otherwise refine the list query once or ask the user which knowledge base/domain to use.
   - If `status` is `no_results`, answer from conversation context if possible and say no relevant accessible knowledge base was found.
3. Call `retrieve_knowledge` with the selected `dataset_ids` and a concise query. Do not use dataset IDs that were not returned by the list tool.
4. Inspect `status`, `source_summary`, `context_blocks`, and scores before answering:
   - If retrieved blocks answer the user, answer from those blocks.
   - If the blocks are missing, weak, or off-topic, rewrite the query once using clearer entities, synonyms, or constraints and retry with the same relevant datasets.
   - After two retrieval attempts, if the relevant answer is still unclear, ask the user for clarification or say that no relevant accessible knowledge was found.
5. When the user asks for original wording, definitions, synopsis text, policy clauses, exact wording, or "what does it say", quote or closely excerpt the retrieved source text first and cite the source. Summarize only when the user asks for a summary or the original text is too long.
6. When retrieved context is used, cite source names from `source_summary` or `retriever_resources` when useful.
7. Never expose internal IDs such as dataset ID, document ID, or segment ID to the user.
8. Never invent a knowledge-base answer when no relevant accessible context was found.

## Tool Usage

`list_accessible_knowledge_bases` accepts:

- `query`: optional search text for narrowing candidate knowledge bases.
- `limit`: optional maximum number of candidates. Defaults to 20 and is capped at 100.

`retrieve_knowledge` accepts:

- `query`: the user question or refined search query.
- `dataset_ids`: required selected knowledge base IDs returned by the list tool.
- `top_k`: optional maximum retrieved chunks. Defaults to 5 and is capped at 20.
- `retrieval_mode`: optional `hybrid`, `vector`, or `graph`. Omit it for the default hybrid mode; use `graph` only for relationship or entity questions.
