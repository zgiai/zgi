# Policy Comparison

## Use When

Use for comparing old and new制度, policy, procedure, handbook, compliance rule, approval flow, operating standard, or internal management document versions.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and table fields only. Do not use `Policy`, `Scope`, `Responsibility`, `Approval`, or `Constraint`.
- For English requests, use English headings and table fields only.

## Output Shape

For Chinese output:

```md
## 制度变化摘要

...

## 制度差异清单

| 维度 | 旧版本 | 新版本 | 变化说明 | 影响 | 原文依据 |
| --- | --- | --- | --- | --- | --- |
| 适用范围 | ... | ... | ... | ... | ... |
| 职责 | ... | ... | ... | ... | ... |
| 流程 | ... | ... | ... | ... | ... |
| 审批 | ... | ... | ... | ... | ... |
| 例外规则 | ... | ... | ... | ... | ... |
| 约束或处罚 | ... | ... | ... | ... | ... |

## 执行影响

- ...

## 风险与待确认问题

- ...
```

## Data Rules

- Compare applicability, responsibilities, procedure steps, approval authorities, exceptions, constraints, penalties, reporting duties, and effective dates.
- Do not infer policy intent beyond source text.
- State when a section is missing or cannot be compared due to incomplete parsed content.
- Distinguish wording simplification from substantive rule change.
- Highlight changes that affect responsibilities, compliance obligations, approval thresholds, or enforcement.

## Clarification Rules

Ask when the old and new versions are not identifiable, when effective dates are unclear, or when the user needs a specific department, role, or process scope.
