# Generic Comparison

## Use When

Use when the user asks for a general comparison across two or more documents and does not specify a specialized comparison type such as contract clauses, vendor proposals, PRD changes, or policy changes.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and table fields only. Do not use `Summary`, `Comparison Table`, `Differences`, `Risks`, or `Open Questions`.
- For English requests, use English headings and table fields only.

## Output Shape

For Chinese output:

```md
## 对比摘要

...

## 共同点

- ...

## 差异清单

| 维度 | 文档A | 文档B | 差异说明 | 原文依据 |
| --- | --- | --- | --- | --- |
| ... | ... | ... | ... | ... |

## 冲突点

| 冲突项 | 涉及文档 | 冲突说明 | 风险 |
| --- | --- | --- | --- |
| ... | ... | ... | ... |

## 风险提示

- ...

## 待确认问题

- ...
```

If comparing more than two documents, add columns for each document or use `涉及文档` to keep the table readable.

## Data Rules

- Compare only content present in the source documents.
- Preserve document labels, section names, clause names, numbers, dates, amounts, and explicit limitations.
- Separate common points from differences and conflicts.
- Mark missing content as `未提及` or `无法判断`, not as a confirmed difference.
- Include original evidence with short source references such as document label, section title, clause name, or quoted short phrase.

## Clarification Rules

Ask when the available documents are not clearly labeled, when the user did not specify which documents to compare, or when the comparison dimension is too broad for the available content.
