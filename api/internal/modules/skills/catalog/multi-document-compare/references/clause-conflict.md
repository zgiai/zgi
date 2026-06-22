# Clause Conflict

## Use When

Use for contracts, agreements, statements of work, service terms, responsibility matrices, acceptance terms, payment terms, delivery terms, breach terms, or any request to identify conflicting clauses and inconsistent descriptions.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and table fields only. Do not use `Clause`, `Conflict`, `Evidence`, `Risk`, or `Mitigation`.
- For English requests, use English headings and table fields only.

## Output Shape

For Chinese output:

```md
## 冲突摘要

...

## 冲突点

| 冲突项 | 文档A描述 | 文档B描述 | 冲突说明 | 风险 | 原文依据 |
| --- | --- | --- | --- | --- | --- |
| ... | ... | ... | ... | ... | ... |

## 不一致描述

| 维度 | 涉及文档 | 不一致内容 | 影响 |
| --- | --- | --- | --- |
| ... | ... | ... | ... |

## 风险提示

- ...

## 待确认问题

- ...
```

## Data Rules

- Focus on contradictions, incompatible obligations, conflicting time limits, inconsistent payment terms, unclear acceptance criteria, duplicated responsibilities, and inconsistent liability allocation.
- Distinguish hard conflicts from wording differences that do not materially change meaning.
- Do not provide legal advice or final legal judgment.
- Use short source-backed evidence for each conflict.
- If a clause is missing from one document, state `未提及` rather than treating it as a conflict unless the omission itself creates a source-backed risk.

## Clarification Rules

Ask when the user has not specified which contract versions or documents are authoritative, or when the parsed content lacks the relevant clauses.
