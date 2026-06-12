# Product Feature Decision

Use for product feature go/no-go decisions, roadmap scope, MVP decisions, user value tradeoffs, and whether a capability should be built now, later, or not at all.

## Payload Examples

```json
{
  "decision_goal": "Decide whether to build batch export in Q3",
  "options": ["build full export", "build CSV-only export", "defer"],
  "criteria": ["customer value", "revenue impact", "engineering cost", "support load", "strategic fit"],
  "risk_tolerance": "balanced"
}
```

## Data Rules

- Separate user demand evidence from assumptions.
- Evaluate feature value, target segment, frequency of use, willingness to pay, operational impact, delivery complexity, support risk, and strategic alignment.
- Prefer a small reversible experiment when demand or cost is uncertain.
- Do not fabricate user research, ARR, conversion lift, retention impact, roadmap commitments, or customer promises.
- If a feature is compliance-, security-, or contract-sensitive, mark required review explicitly.

## Clarification Rules

Call `request_user_input` when:

- The feature or decision goal is unclear.
- No target user, business objective, or success metric is provided.
- The user asks "要不要做" but gives no options, evidence, or constraints.
- The recommendation depends on timeline, capacity, enterprise customer commitment, or compliance requirements that are missing.

## Output Focus

Use a product decision matrix:

- User value
- Business impact
- Strategic fit
- Build cost
- Operational cost
- Risk and reversibility
- Recommended next step: build, test, defer, reject, or split scope
