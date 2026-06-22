# Project Prioritization

Use for ranking projects, roadmap items, workstreams, initiatives, customer requests, and backlog candidates when resources are constrained.

## Payload Examples

```json
{
  "decision_goal": "Rank Q3 initiatives for a 6-person team",
  "items": ["SSO", "audit log", "mobile approval", "data export"],
  "criteria": ["impact", "urgency", "effort", "risk", "dependency", "strategic fit"],
  "capacity": "6 engineers, one quarter"
}
```

## Data Rules

- Use RICE, ICE, cost-of-delay, WSJF, or a simple weighted matrix only when inputs are sufficient.
- If precise scores are not supported, use qualitative levels: high, medium, low.
- Identify dependencies, blockers, capacity conflicts, sequencing constraints, and quick wins.
- Do not invent effort estimates, customer counts, revenue, deadlines, owners, or dependencies.
- When ranking is uncertain, recommend a first-pass priority and the data needed to refine it.

## Clarification Rules

Call `request_user_input` when:

- No candidate items are provided.
- The ranking goal is unclear, such as revenue vs retention vs compliance vs delivery speed.
- The user needs a numeric rank but no criteria or weights are given.
- Capacity, deadline, or dependency constraints materially affect the result and are missing.

## Output Focus

Use a prioritization table:

- Item
- Impact
- Urgency
- Effort
- Risk
- Dependency
- Suggested priority
- Rationale

End with recommended sequence and what to defer.
