# Management Decision

Use for management decisions involving resource allocation, process changes, operating model choices, escalation paths, organizational tradeoffs, and executive decision support.

## Payload Examples

```json
{
  "decision_goal": "Decide whether to split the support team by customer segment",
  "options": ["keep current model", "split by segment", "split by product line"],
  "criteria": ["customer response time", "team workload", "management complexity", "cost", "scalability"],
  "risk_tolerance": "balanced"
}
```

## Data Rules

- Consider stakeholder impact, execution complexity, capability gaps, communication cost, reversibility, morale risk, governance, and measurable success criteria.
- Separate decision recommendation from change-management plan.
- Do not invent headcount, budget, performance data, approval status, HR policy, legal obligations, or executive intent.
- For HR, legal, compliance, or financial matters, clearly mark that formal review is required.

## Clarification Rules

Call `request_user_input` when:

- The affected team, stakeholder group, decision owner, or desired outcome is unclear.
- The decision materially affects people, budget, compliance, or customer commitments and critical constraints are missing.
- The user asks for escalation or accountability assignment without authority or policy context.

## Output Focus

Use a management decision brief:

- Decision context
- Options
- Stakeholder impact
- Benefits
- Risks
- Implementation difficulty
- Governance or approval needs
- Recommendation and next alignment step
