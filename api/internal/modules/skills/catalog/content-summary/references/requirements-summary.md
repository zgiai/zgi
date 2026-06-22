# Requirements Summary

## Use When

Use when the user asks to summarize requirements, distill requirement points, extract product scope, summarize PRD/MRD content, identify acceptance points, or organize already extracted requirement source content.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and table fields only. Do not use `Requirements Summary`, `Requirement Points`, `Priority`, `Acceptance Signal`, `Notes`, `Scope`, or `Open Questions`.
- For English requests, use English headings and table fields only.

## Output Shape

For Chinese output:

```md
## 需求摘要

...

## 需求点

| 编号 | 需求 | 优先级 | 验收信号 | 备注 |
| --- | --- | --- | --- | --- |
| R1 | ... | 未说明 | ... | ... |

## 范围

- 范围内：...
- 范围外：...

## 风险与待确认问题

- ...
```

If the user explicitly asks for English, translate all headings and field names consistently into English.

## Data Rules

- Use only source content that has already been pasted, parsed from an attachment, or provided by another extraction result.
- Do not directly parse uploaded requirement files.
- Do not invent priorities, acceptance criteria, or scope boundaries.
- Preserve requirement qualifiers such as must, should, optional, excluded, dependency, and constraint.
- Mark conflicts, missing details, and ambiguous product decisions.

## Clarification Rules

Ask when the user asks to summarize requirement points but the available source includes multiple products, versions, modules, or audiences and the requested scope is unclear.
