---
name: ticket-routing
description: Classify customer service tickets, employee issues, tenant property-service requests, and enterprise support questions by issue type, urgency, responsible department, handling team, workspace, and owner role; generate structured routing results, internal handling suggestions, and initial customer replies. Use when the user asks for 客服工单分流, 工单分类, 工单派发建议, 客户问题识别, 问题类型, 紧急程度, 处理部门, 处理团队, 工作空间分配, 物业客服, 报修, 投诉, 咨询, 财务, 法务, 招商, 物业, IT, 行政, 客服, ticket routing, ticket triage, issue classification, urgency, department routing, workspace routing, or customer reply.
when_to_use: Use this skill when an existing customer/employee issue, ticket text, chat message, or parsed complaint content needs triage into issue type, urgency, responsible department, handling team, workspace, or owner role, with internal handling suggestions and an initial customer reply. This skill produces routing recommendations only. It does not directly create, update, dispatch, close, or escalate tickets unless a separate backend tool is available and explicitly called by another workflow. If the user asks to export the routing result, produce the content first and then route it to file-generator.
runtime_type: prompt
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: tickets
  category: workflow_automation
  scenarios:
    - customer_service
    - business_operations
  label:
    en_US: Ticket Routing
    zh_Hans: 工单分流
  description:
    en_US: Designed for customer or employee issues that need triage; identifies issue type and urgency, recommends the responsible department or owner, and drafts an initial reply.
    zh_Hans: 适用于客户或员工问题的工单分流，可识别问题类型和紧急程度，推荐处理部门或负责人，并生成初步回复建议。
  when_to_use:
    en_US: Use when a customer or employee issue needs triage and routing recommendations.
    zh_Hans: 当客户或员工问题需要分类、定级和分流建议时使用。
  tags:
    en_US:
      - Ticket
      - Routing
      - Customer Service
    zh_Hans:
      - 工单
      - 分流
      - 客服
---

# Ticket Routing Skill

Use this skill to classify a customer, tenant, employee, or enterprise support issue and produce routing recommendations. This skill does not perform real ticket creation, dispatch, workspace assignment, owner notification, ticket closure, refunds, legal decisions, or financial approvals.

## Scope

Use this skill for:

- Property customer-service tickets, including repairs, complaints, payment questions, parking, access control, cleaning, noise, safety, and public-area issues.
- Enterprise internal or external support questions that need routing to finance, legal, investment leasing, property operations, IT, administration, HR, sales, customer service, or another department.
- Structured triage outputs: issue type, urgency, responsible team, suggested workspace, owner role, routing reason, internal action, missing information, risk warning, and initial customer reply.
- Department/workspace mapping suggestions when mapping rules are provided by the user or platform context.

Do not use this skill to:

- Directly create, update, dispatch, close, escalate, or assign tickets.
- Claim that a ticket has been sent to a department, workspace, or owner.
- Replace legal review, financial approval, property engineering inspection, safety incident handling, or formal complaint investigation.
- Make strong conclusions when the issue content is incomplete, ambiguous, contradictory, or lacks evidence.
- Generate Word, PDF, Markdown, TXT, CSV, XLSX, or any downloadable file directly.

## Routing Rules

- If the user provides a customer/employee issue and asks to classify, triage, route, assign, or reply, use this skill directly.
- If the user asks for real dispatch, produce a routing recommendation only unless an external workflow/tool explicitly performs dispatch and returns success.
- If department or workspace mapping rules are available, use them exactly. Do not invent workspace IDs, department owners, duty schedules, or responsible persons.
- If mapping rules are missing and the user requires a specific workspace, department owner, or dispatch target, call `request_user_input`.
- If the issue spans multiple departments, identify a primary owner and collaborating teams. If primary ownership cannot be determined, mark `需人工确认`.
- If the user asks to export the routing result, first prepare the routing content, then route that content to `file-generator`.

## Workflow

1. Confirm that issue content is available from the user message, chat context, parsed ticket text, or platform context.
2. Identify the task scenario: property service, enterprise department routing, urgency assessment, customer reply, or workspace mapping.
3. If the scenario is clear, read exactly one relevant reference before writing the result.
4. Extract facts from the issue only: customer request, location, object, time, impact scope, safety risk, payment amount, contract terms, system name, affected user, and requested outcome.
5. Classify issue type and urgency. Separate source-backed facts from inferred routing judgment.
6. Recommend responsible department/team/workspace only when the mapping is known or can be reasonably described at role level.
7. Produce an internal handling suggestion and an initial customer reply. Keep the reply service-oriented and do not overpromise resolution time unless a policy is provided.
8. If information is insufficient, state what is missing and whether the ticket should be manually confirmed.

## Clarification Workflow

Call `request_user_input` instead of writing a plain clarification when a missing decision blocks reliable routing. Ask 1-4 focused questions. Options must be concrete answers that can be used directly. A single question may contain at most 5 options.

Ask when:

- No issue content is available.
- The issue text is too vague to determine type, urgency, or responsible team.
- The user asks for specific workspace/owner dispatch but no department-to-workspace mapping is available.
- Multiple departments may be primary owner and the user requires a single target.
- Urgency depends on missing facts such as safety risk, service outage scope, payment deadline, contract deadline, or number of affected customers.
- The user asks for a customer reply but the desired tone or SLA commitment is unclear.

Example `request_user_input` payload:

```json
{
  "message": "我可以进行工单分流，但需要先确认关键分流信息。",
  "questions": [
    {
      "id": "routing_scope",
      "question": "这次需要分流到什么粒度？",
      "options": [
        { "label": "只判断部门", "description": "输出建议处理部门和依据。" },
        { "label": "部门和工作空间", "description": "按映射规则输出建议工作空间。" },
        { "label": "部门和负责人角色", "description": "输出主责角色和协作角色。" },
        { "label": "完整分流结果", "description": "包含类型、紧急度、部门、建议、回复。" }
      ]
    },
    {
      "id": "urgency_signal",
      "question": "是否存在安全、停服、合同期限或大范围影响？",
      "options": [
        { "label": "存在安全风险", "description": "优先按高紧急程度处理。" },
        { "label": "存在大范围影响", "description": "优先升级并通知协作团队。" },
        { "label": "普通咨询", "description": "按常规队列处理。" },
        { "label": "暂不确定", "description": "输出需补充确认。" }
      ]
    }
  ]
}
```

After calling `request_user_input`, stop the turn and wait for the user's answer.

## Output Rules

For Chinese output, use this structure unless the user requests another format:

```text
分流结果
- 问题类型：
- 紧急程度：
- 建议处理部门：
- 建议工作空间：
- 建议负责人角色：
- 是否需要人工确认：
- 判断依据：

内部处理建议
- 优先处理动作：
- 需要补充的信息：
- 协作团队：
- 风险提示：

客户初步回复
...
```

For English output, use consistent English labels. Do not mix Chinese and English structural field names in the same result.

## Language Rules

- Match the user's requested output language. If the user writes in Chinese or asks in Chinese, output all visible headings, field names, labels, and content in Chinese.
- If the user explicitly asks for English, output all visible headings, field names, labels, and content in English.
- Do not mix Chinese and English section headings in the final answer. Avoid English labels such as `Summary`, `Category`, `Urgency`, `Owner`, `Workspace`, `Risk`, and `Reply` in Chinese output.
- Keep department names, workspace names, system names, product names, contract identifiers, ticket IDs, and quoted customer terms in their original language when translation would change meaning.

## References

Read exactly one reference after choosing the routing scenario:

| Routing scenario | Read reference |
| --- | --- |
| Generic ticket classification, issue type, responsible team, internal suggestion | `generic-routing.md` |
| Property customer service, repairs, complaints, payment, parking, access, safety | `property-service.md` |
| Enterprise department routing, finance, legal, leasing, IT, admin, HR, customer service | `enterprise-department-routing.md` |
| Urgency and escalation level assessment | `urgency-level.md` |
| Initial customer reply drafting | `customer-reply.md` |
| Department/workspace mapping and missing mapping clarification | `workspace-mapping.md` |

If the user asks for multiple outputs, read the reference that matches the primary decision and include secondary sections only when they are directly supported by issue content and known routing rules.

## Constraints

- Do not expose internal reasoning or mention this skill.
- Do not output JSON unless the user explicitly asks for JSON.
- Do not claim a ticket was dispatched, assigned, escalated, closed, refunded, approved, or legally reviewed unless a separate backend workflow returns that result.
- Do not invent workspace IDs, department owners, schedules, SLA commitments, policy terms, refund rules, contract clauses, or responsibility boundaries.
- Do not use sensitive attributes such as gender, age, ethnicity, health, marital status, or nationality as routing or urgency evidence.
- Do not replace legal, financial, safety, engineering, or compliance review.
- If source information is insufficient, mark `信息不足，需补充确认` in Chinese output.
- Do not generate files directly. Use `file-generator` only after the routing content is prepared and the user requested export.
