# Generic Routing

## Use When

Use for general customer service, employee support, or enterprise issue triage when no specialized scenario dominates.

## Classification Rules

Identify:

- Issue type: consultation, complaint, repair, payment, contract, system fault, access request, policy question, service request, safety issue, or other.
- Request object: product, service, space, contract, invoice, account, system, facility, person, or event.
- Impact: one person, one tenant, one department, multiple customers, public area, business continuity, legal/financial exposure.
- Desired outcome: answer, repair, refund, approval, correction, escalation, investigation, assignment, or follow-up.

## Routing Output

Include:

- Suggested primary department or team.
- Suggested collaboration teams when the issue crosses boundaries.
- Urgency level and reason.
- Internal handling suggestion.
- Missing information.
- Initial customer reply.

## Data Rules

- Use source issue content only.
- If a department name is not provided by mapping rules, recommend at role level, such as finance team or property operations team.
- If ownership is unclear, mark `需人工确认`.

## Clarification Rules

Ask when the issue text lacks the problem, object, affected scope, or desired outcome.
