# Risks And Conclusions

## Use When

Use when the user asks for risks, issues, concerns, conclusions, decisions, tradeoffs, blockers, mitigations, or a judgment-oriented synthesis.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and table fields only. Do not use `Conclusions`, `Risks`, `Evidence`, `Impact`, `Mitigation`, or `Open Questions`.
- For English requests, use English headings and table fields only.

## Output Shape

For Chinese output:

```md
## 结论

- ...

## 风险

| 风险 | 依据 | 影响 | 缓解措施 |
| --- | --- | --- | --- |
| ... | ... | ... | 未说明 |

## 决策与待确认问题

- ...
```

If the user explicitly asks for English, translate all headings and field names consistently into English.

## Data Rules

- Separate evidence from inference.
- Mark inferred risks as inference when the source does not explicitly name them.
- Do not exaggerate severity beyond what the source supports.
- Preserve contradictions, unresolved questions, and missing assumptions.
- Include mitigation only if stated or clearly requested as a recommendation.

## Clarification Rules

Ask when the user requests risks but does not specify whether they want only source-stated risks or inferred risks as well, and that distinction materially changes the answer.
