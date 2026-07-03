# Customer Solution

Use for customer方案, presales方案, delivery option comparison, enterprise solution recommendations, implementation scope, and customer-specific tradeoffs.

## Payload Examples

```json
{
  "decision_goal": "Choose an implementation方案 for a key account",
  "options": ["standard product", "light customization", "custom project"],
  "criteria": ["customer value", "delivery risk", "margin", "timeline", "reuse potential", "contract risk"],
  "customer_context": "large enterprise, urgent launch"
}
```

## Data Rules

- Separate customer-stated requirements from inferred needs.
- Compare fit, delivery feasibility, integration complexity, timeline, acceptance criteria, margin, reuse value, support burden, and contractual exposure.
- Do not invent commitments, pricing, discounts, legal terms, SLA, delivery dates, or acceptance criteria.
- Flag when a proposal requires sales, delivery, legal, finance, or security confirmation.
- Prefer phased scope when requirements are uncertain or custom work is high risk.

## Clarification Rules

Call `request_user_input` when:

- Customer goal, must-have requirements, timeline, or budget constraints are missing.
- The user asks for a commitment-level recommendation but contract, price, or delivery scope is unknown.
- Multiple方案 exist but acceptance criteria or decision-maker priorities are unclear.

## Output Focus

Use a customer solution decision table:

-方案
- Customer value
- Delivery feasibility
- Commercial impact
- Risk
- Required confirmation
- Recommendation

Include suggested communication points and internal alignment items.
