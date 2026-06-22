# Decision Review

Use for decision review,复盘, postmortem, lessons learned, whether a past decision was good, and how to improve the decision framework next time.

## Payload Examples

```json
{
  "decision_goal": "Review why the CRM migration was delayed",
  "original_options": ["big-bang migration", "phased migration"],
  "chosen_option": "big-bang migration",
  "outcome": "launch delayed by one month",
  "review_goal": "improve future migration decisions"
}
```

## Data Rules

- Separate decision quality from outcome quality. A good decision can have a bad outcome if uncertainty was real.
- Reconstruct original context only from provided information.
- Identify assumptions, missing signals, decision process gaps, execution gaps, and monitoring gaps.
- Do not assign blame without evidence.
- Do not rewrite history by using information that was unavailable at the time unless clearly labeled as hindsight.

## Clarification Rules

Call `request_user_input` when:

- The original decision, selected option, outcome, or review goal is unclear.
- The user asks for root cause but provides only a conclusion.
- The review affects personnel accountability, legal exposure, financial loss, or customer commitments and evidence is missing.

## Output Focus

Use a decision review structure:

- Original decision and context
- What was known then
- Key assumptions
- Outcome
- Decision quality assessment
- Process gaps
- Lessons learned
- Updated decision rule for next time
