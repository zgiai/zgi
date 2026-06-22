# Technical Choice

Use for technical solution selection, architecture choices, tool selection, build-vs-buy decisions, migration plans, integration design, and implementation tradeoffs.

## Payload Examples

```json
{
  "decision_goal": "Choose a search backend for enterprise knowledge retrieval",
  "options": ["Postgres full-text", "Elasticsearch", "managed vector search"],
  "criteria": ["latency", "relevance", "operational cost", "team familiarity", "scalability", "security"],
  "risk_tolerance": "conservative"
}
```

## Data Rules

- Compare options against concrete non-functional requirements: latency, scale, reliability, security, cost, observability, maintainability, migration effort, team skill, vendor lock-in, and rollback path.
- Distinguish one-way-door choices from reversible choices.
- Prefer incremental migration when risk is high and current system is still viable.
- Do not invent benchmark numbers, cloud pricing, security certifications, SLAs, or compatibility claims.
- If production safety is involved, recommend proof-of-concept, load testing, rollback plan, and security review.

## Clarification Rules

Call `request_user_input` when:

- The options are missing.
- The workload, scale, reliability target, or security requirements are unknown.
- The user asks for a firm recommendation but critical constraints are missing.
- Buy-vs-build depends on budget or procurement limits not supplied.

## Output Focus

Use a technical decision record shape:

- Context
- Options
- Evaluation dimensions
- Tradeoff table
- Recommendation
- Reversibility and migration path
- Risks, validation plan, and rollback plan
