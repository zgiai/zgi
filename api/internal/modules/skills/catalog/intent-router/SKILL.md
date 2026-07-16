---
name: intent-router
description: Identify the user's real intent from the current message, conversation context, uploaded files, and available capabilities, then classify the request into a standard task type with confidence, recommended action, routing hints, missing information, and evidence. Use for intent recognition, task routing, request classification, workflow dispatch, database routing, knowledge retrieval routing, file export routing, chart or visualization routing, report routing, schedule planning routing, calculation routing, clarification planning, 意图识别, 任务路由, 请求分类, 工作流分发, 数据库查询路由, 知识库检索路由, 文件生成, 图表生成, 可视化, 报告生成.
when_to_use: Use this skill when an agent must decide what kind of task the user is asking for before choosing a skill, tool, workflow, database query, knowledge retrieval, file generation, chart generation, report generation, schedule plan, calculation, or structured clarification path.
provider_type: builtin
provider_id: intent_router
runtime_type: hybrid
tools:
  - route_intent
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: git-fork
  category: workflow_automation
  scenarios:
    - business_operations
    - customer_service
    - technical_development
  label:
    en_US: Intent Router
    zh_Hans: 意图路由
  description:
    en_US: Designed for ambiguous or compound requests; identifies the task type and recommends the right Skill, tool, workflow, database, or knowledge-retrieval path.
    zh_Hans: 适用于意图不清或包含多项任务的请求，可识别任务类型并推荐合适的 Skill、工具、工作流、数据库或知识检索路径。
  when_to_use:
    en_US: Use before dispatching ambiguous or capability-dependent requests to skills, tools, workflows, databases, or knowledge retrieval.
  tags:
    en_US:
      - Intent
      - Routing
      - Classification
---

# Intent Router Skill

Use this skill to classify the user's current request into a stable routing result. The model performs the semantic judgment; the `route_intent` builtin tool validates and normalizes the structured result so downstream agent logic can consume it reliably.

## Workflow

1. Read `taxonomy.md` before choosing `task_type`, `intent_id`, `recommended_action`, or target skill/tool names.
2. Inspect the current user message, recent conversation context, uploaded file metadata, and any known available skills, workflows, databases, or knowledge bases.
3. Decide whether the request can be routed directly or whether required information is missing.
4. Read `routing-rules.md` when the request could map to a skill, tool, workflow, database, or knowledge retrieval path.
5. Read `clarification-rules.md` when the request is ambiguous, incomplete, risky, or would trigger database mutation or workflow execution.
6. Read `payload-examples.md` when building a new or unfamiliar `route_intent` payload.
7. Call `route_intent` with a complete structured classification result.
8. Return or use the normalized tool result. Do not invent a downstream tool result; this skill only classifies and recommends next action.

## Required Judgments

Always decide:

- `task_type`: one value from the taxonomy.
- `intent_id`: stable dotted identifier in the form `<task_type>.<subtype>` when a subtype is known.
- `confidence`: numeric value from 0 to 1.
- `recommended_action`: one value from the action taxonomy.
- `normalized_request`: concise restatement of what the user is actually asking.
- `evidence`: short facts from the message/context that support the classification.
- `missing_info`: required fields that block reliable execution, if any.

## Clarification Behavior

This skill usually records ambiguity in `missing_info`; it does not automatically ask the user.

Use `request_user_input` instead of `route_intent` only when a clarification is necessary before a reliable route can be produced, or when the next action would be high impact and the route is not clear enough. Each question must be structured, concrete, and limited to the blocking decision.

## References

Read these files as needed:

| Need | Read reference |
| --- | --- |
| Task types, action values, intent ID rules, confidence scale | `taxonomy.md` |
| Skill/tool/workflow/database/knowledge routing rules | `routing-rules.md` |
| Missing information and structured clarification policy | `clarification-rules.md` |
| Valid `route_intent` payload examples | `payload-examples.md` |

## Constraints

- Do not call downstream business tools from this skill. It only classifies and recommends.
- Do not silently convert an ambiguous request into a specific execution route.
- Do not mark `recommended_action` as `run_workflow`, `query_database`, or `call_tool` unless the required target is known or listed in `missing_info`.
- Do not use `confidence` above `0.85` when multiple materially different task types remain plausible.
- Do not include sensitive raw file contents in `evidence`; use filenames, MIME types, summaries, or attachment roles.
- Keep `evidence` short and grounded in the user's message or supplied context.
- Keep output field names stable. Use the `route_intent` tool result as the source of truth.
