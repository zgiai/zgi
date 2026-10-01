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
tool_governance:
  retrieve_agent_knowledge:
    tool_id: knowledge.retrieve_agent
    skill_id: agent-knowledge
    domain: knowledge
    effect: read
    asset_type: knowledge_base
    risk_level: low
    requires_asset_resolution: false
    reversible: false
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - knowledge:read
    default_approval_policy: auto_by_permission_tier
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
display:
  icon: book-open-check
  category: knowledge_retrieval
  scenarios:
    - knowledge_research
    - technical_development
  label:
    en_US: Agent Knowledge
    zh_Hans: 智能体知识库
  description:
    en_US: Designed for Agent answers that need internal sources; retrieves relevant facts and context only from knowledge bases bound to the current Agent.
    zh_Hans: 适用于智能体基于内部资料回答问题，可从当前智能体绑定的知识库中检索相关事实和来源上下文。
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

### 1. Decide whether retrieval is necessary

Retrieve only when the answer depends on information that may exist in the Agent's
configured knowledge bases and is not already available in the conversation. Typical
cases include internal policies, product facts, procedures, historical records,
definitions, exact clauses, and source-grounded wording.

Do not retrieve for general knowledge, casual conversation, or a request that only
transforms content already supplied by the user. Do not retrieve again when evidence
already collected in the current user turn is sufficient.

### 2. Identify the evidence needed

Before the first call, identify the key claim or relationship that requires evidence.
For a simple question, use one query. For a complex or multi-part question, decompose
the task into a short internal checklist of independent claims, entities, time ranges,
or relationships. Order the sub-questions by dependency and importance, then retrieve
one focused query per unresolved item. Do not split a simple question into artificial
sub-questions. Do not enumerate knowledge bases, ask for dataset IDs, guess dataset
IDs, or pass dataset IDs.

For example, a request to explain Apple's relationship with Intel, including its
history, processor supply, and current status, can become three focused evidence
items: historical cooperation, processor-supply facts, and current relationship
status. Retrieve those items separately and synthesize them only after checking the
evidence for each item.

### 3. Write a standalone query

Preserve the user's intent, named entities, relationships, time range, scope, and
conditions. Prefer the user's language and any source-specific terminology. You may
add a useful synonym or paraphrase, but do not change the meaning or reduce a
relationship question to unrelated keywords.

Good: `business and processor-supply relationship between Apple and Intel`
Bad: `Apple Intel`

Each query should express one coherent information need and remain understandable
without the surrounding conversation. Do not repeat a query already used in the
current user turn. Distinguish decomposition from rewriting: decomposition creates a
new query for a different evidence item, while rewriting changes wording for the same
unresolved item.

### 4. Choose the smallest sufficient parameters

- `top_k`: omit it to use the default value of 5. When supplied, it must be an
  integer from 1 to 10. Use a value above 5 only when the answer is likely to require
  evidence from several chunks or sources. Never request more than 10.
- `retrieval_mode`: omit it to use the default `hybrid` mode. Use
  `vectorAndBM25` for textual facts, passages, definitions, or exact wording; use
  `graph` for entity relationships, dependencies, or topology; use `hybrid` when the
  question needs both textual evidence and relationship evidence, or when the best
  route is uncertain.

The modes have these fixed meanings:

- `vectorAndBM25`: Vector and BM25 recall, followed by RRF and textual reranking.
- `graph`: graph retrieval.
- `hybrid`: `vectorAndBM25` plus `graph`.

### 5. Evaluate the returned evidence

Inspect `status`, `result_count`, `source_summary`, `context_blocks`, scores,
`warnings`, and `control` when present. A high score alone does not make the evidence
sufficient. Check whether the returned content covers the key claim, is relevant to
the requested scope and time, and conflicts with another source.

- `status = no_config`: stop; the Agent has no configured knowledge bases.
- `status = no_results`: retry only if a materially different query or retrieval mode
  can target a still-valid evidence gap.
- `status = blocked`: obey `control`; for `duplicate_query`, `query_too_long`, or
  `query_required`, make one corrected, materially different call only when
  `control.continuation_allowed` is true. Otherwise answer from evidence already
  collected.
- `control.continuation_allowed = false`: stop immediately.
- A rerank or retrieval degradation warning is not, by itself, a reason to repeat the
  same query. Evaluate the fallback results that were returned.

### 6. Choose the next action

| Evidence state | Next action |
|---|---|
| The key question is covered | Stop retrieving and answer with citations |
| A specific part is missing | Query only that missing part |
| Results are off-topic | Clarify the entities, relationship, scope, or time constraint |
| No results were found | Try one materially different wording or a suitable mode |
| Sources conflict | Run one focused query for the disputed claim |
| No new evidence was added | Stop and report the remaining uncertainty |

Every additional call must have a distinct evidence goal. Reformulate at most once
for the same evidence gap. The maximum is twenty retrieval tool calls in one Agent
execution, including calls issued together in the same model step. A normal Agent
execution corresponds to one user turn; this is not a conversation-wide quota.

### 7. Stop and answer

Stop retrieving when any of the following is true:

- The key claims are supported by the collected evidence.
- No distinct, useful next query remains.
- A retry did not add relevant evidence.
- Twenty retrieval calls have been used in the current Agent execution.
- `control.continuation_allowed` is false or `control.stop_reason` indicates a hard
  stop such as `retrieval_round_limit`, `no_new_evidence`, `rate_limited`,
  `context_budget_exhausted`, or `no_config`.

Answer from the evidence collected across the current user turn. Cite source names
from `source_summary` or `retriever_resources`, distinguish confirmed information
from unresolved or inferred points, and preserve exact wording when the user asks for
it. Never expose dataset, document, or segment IDs. If no relevant evidence was found,
say so instead of inventing an answer.

## Tool Usage

`retrieve_agent_knowledge` accepts:

- `query`: the user question or refined search query.
- `top_k`: optional maximum retrieved chunks. Defaults to 5 and must be between 1 and 10.
- `retrieval_mode`: optional `hybrid`, `vectorAndBM25`, or `graph`. Omit it for the
  default `hybrid`; use `vectorAndBM25` for the textual Vector + BM25 route, `graph`
  for relationship/entity questions, and `hybrid` for `vectorAndBM25 + graph`.
