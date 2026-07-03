---
name: decision-support
description: Support structured business decision analysis for product, technical, project, presales, customer solution, and management choices. Use when the user asks for 决策, 帮我选, 怎么选, 要不要做, 方案对比, 技术选型, 功能优先级, 项目优先级, 风险收益, 取舍, 权衡, 决策复盘, 客户方案, 售前方案, 管理决策, decision, choose, tradeoff, prioritization, roadmap, technical choice, option comparison, risk-benefit, ROI, should we, or decision review.
when_to_use: Use this skill when the user needs to compare multiple options, decide whether to do a product feature, choose a technical approach, rank projects or roadmap items, evaluate a customer or presales solution, make a management decision, or review a past decision. This skill produces structured decision support only. It may use available context or exposed memory signals, but it does not read, write, update, delete, or manage agent memory itself.
runtime_type: prompt
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: scale
  category: productivity
  label:
    en_US: Decision Support
    zh_Hans: 决策辅助
  description:
    en_US: Compares options, tradeoffs, risks, benefits, priorities, and decision assumptions.
    zh_Hans: 对多个方案进行权衡分析，输出风险收益、优先级、推荐方案和待确认事项。
  when_to_use:
    en_US: Use for product, technical, project, presales, customer solution, and management decisions.
    zh_Hans: 用于产品、技术、项目、售前、客户方案和管理类决策。
  tags:
    en_US:
      - Decision
      - Prioritization
      - Strategy
    zh_Hans:
      - 决策
      - 优先级
      - 方案对比
---

# Decision Support Skill

Use this skill to help the user make structured enterprise decisions. The skill compares options, clarifies assumptions, evaluates risks and benefits, and recommends a decision path for user review. It does not make binding business, legal, financial, HR, security, or contractual decisions for the user.

## Scope

Use this skill for:

- Product feature go/no-go decisions, roadmap tradeoffs, and MVP scope choices.
- Technical solution selection, architecture tradeoffs, build-vs-buy decisions, and migration choices.
- Project prioritization across impact, urgency, cost, dependency, and strategic fit.
- Customer solution or presales方案 comparison, including implementation feasibility and delivery risk.
- Management decisions involving resource allocation, process changes, organizational tradeoffs, and escalation paths.
- Decision review after an outcome is known, including what worked, what failed, and how to update future criteria.

Do not use this skill for:

- Summarizing source content as the main task.
- Writing documents, reports, slides, emails, tables, or files as the main task.
- Directly reading, updating, deleting, or storing memory.
- Directly operating project systems, CRM, ticket systems, calendars, mailboxes, databases, or workflow tools.
- Making final legal, financial, hiring, firing, medical, safety, security, compliance, or contractual decisions.

## Memory Boundary

This skill is an upper-layer decision workflow. Agent memory is a lower-layer capability.

- Use only decision preferences, risk tolerance, project rules, or historical lessons that are already available in the current context or explicitly exposed by the system.
- Treat memory-derived signals as context, not as immutable truth.
- Do not claim to have read or updated memory unless a separate memory capability explicitly did so.
- Do not write new memory, update memory, delete memory, or ask the user to store memory through this skill.
- If the user asks to remember a decision style, preference, rule, or lesson, state the memory candidate separately and route memory handling to the underlying memory capability when available.

## Workflow

1. Identify the decision scenario: product feature, technical choice, project prioritization, customer solution, management decision, or decision review.
2. Read exactly one matching reference before producing the decision analysis.
3. Extract the decision objective, options, constraints, stakeholders, time window, risk tolerance, known facts, unknowns, and success criteria.
4. If critical information is missing, call `request_user_input` instead of writing a normal Markdown clarification.
5. Choose a lightweight decision framework that fits the scenario, such as weighted criteria, risk-benefit analysis, reversible vs irreversible decision, RICE/ICE, cost-of-delay, decision matrix, or premortem.
6. Separate source-backed facts from assumptions and judgment.
7. Produce a recommendation with confidence level, reasons, tradeoffs, risks, conditions, and next verification steps.
8. If the user asks for export, first prepare the decision analysis text, then route that content to `file-generator`.

## Clarification Workflow

Call `request_user_input` when a missing decision parameter blocks reliable analysis. Ask 1-4 focused questions. Each question may contain at most 5 shortcut options, and every option must be a directly usable answer.

Ask when:

- The decision objective is unclear.
- There are no explicit options and no reasonable options can be inferred from the request.
- The most important evaluation criteria are missing.
- Risk tolerance materially affects the recommendation.
- The user asks for priority ranking but provides no candidate items.
- The user asks for a final recommendation in a high-impact decision but key constraints are missing.
- The decision depends on legal, financial, HR, compliance, security, or contractual facts that are not provided.

Example `request_user_input` payload:

```json
{
  "message": "I can help structure the decision, but need a few key inputs first.",
  "questions": [
    {
      "id": "decision_goal",
      "question": "What is the main decision goal?"
    },
    {
      "id": "options",
      "question": "Which options should be compared?"
    },
    {
      "id": "risk_tolerance",
      "question": "What risk posture should the analysis use?",
      "options": [
        { "label": "conservative", "description": "Prefer lower downside and proven paths." },
        { "label": "balanced", "description": "Balance upside, cost, and execution risk." },
        { "label": "aggressive", "description": "Prioritize upside and speed despite higher risk." }
      ]
    },
    {
      "id": "primary_criteria",
      "question": "Which criteria matter most?",
      "options": [
        { "label": "business impact", "description": "Prioritize revenue, customer value, or strategic impact." },
        { "label": "delivery speed", "description": "Prioritize the fastest viable path." },
        { "label": "cost control", "description": "Prioritize budget and resource efficiency." },
        { "label": "risk reduction", "description": "Prioritize reliability, compliance, and reversibility." },
        { "label": "team capacity", "description": "Prioritize workload and operational feasibility." }
      ]
    }
  ]
}
```

After calling `request_user_input`, stop the turn and wait for the user's answer.

## References

Read exactly one reference after choosing the decision scenario:

| Decision scenario | Read reference |
| --- | --- |
| Product feature go/no-go, roadmap scope, MVP, product priority | `product-feature-decision.md` |
| Technical solution, architecture, build-vs-buy, migration, tool choice | `technical-choice.md` |
| Project priority, roadmap ranking, resource allocation, sequencing | `project-prioritization.md` |
| Customer方案, presales方案, delivery option, solution recommendation | `customer-solution.md` |
| Management decision, organization/process/resource tradeoff, escalation | `management-decision.md` |
| Decision review,复盘, postmortem, lessons learned, decision quality | `decision-review.md` |

If multiple scenarios apply, choose the reference that matches the primary decision. Include secondary elements only when they directly affect the recommendation.

## Output Contract

Default Chinese output:

```text
决策目标
- ...

备选方案
| 方案 | 核心思路 | 适用前提 |

评价维度
| 维度 | 权重/优先级 | 判断依据 |

方案对比
| 方案 | 收益 | 风险 | 成本/复杂度 | 可逆性 | 结论 |

推荐方案
- 建议：
- 置信度：
- 推荐理由：
- 不建议方案及原因：

风险与前提
- 关键假设：
- 主要风险：
- 需要确认：

下一步动作
- ...
```

For English output, use equivalent English headings. Keep headings in one language unless the user asks for bilingual output.

## Constraints

- Do not expose internal reasoning or mention this skill.
- Do not output JSON unless the user explicitly asks for JSON.
- Do not present the recommendation as an irreversible or final decision owned by the assistant.
- Do not invent market data, customer commitments, costs, dates, team capacity, legal terms, financial numbers, security facts, or contract clauses.
- Do not replace legal, financial, HR, compliance, security, medical, or safety review.
- Do not use hidden or assumed memory as evidence. Only use memory signals visible in current context or explicitly supplied by the system.
- Do not read, write, update, delete, or manage memory through this skill.
- Mark assumptions, uncertainty, and decision conditions clearly.
- If the user needs a summary, email, file, table, slide, or report after the decision analysis is prepared, route that downstream task to the appropriate skill.
