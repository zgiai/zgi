# Requirements Difference

## Use When

Use for PRD comparison, MRD comparison, product requirements comparison, requirement version changes, feature scope changes, acceptance criteria changes, process changes, role changes, and dependency changes.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and table fields only. Do not use `Requirement`, `Priority`, `Acceptance`, `Scope`, or `Open Questions`.
- For English requests, use English headings and table fields only.

## Output Shape

For Chinese output:

```md
## 需求变化摘要

...

## 需求差异清单

| 模块或功能 | 旧版本 | 新版本 | 变化类型 | 影响 | 原文依据 |
| --- | --- | --- | --- | --- | --- |
| ... | ... | ... | 新增/删除/修改 | ... | ... |

## 范围变化

- 范围内变化：...
- 范围外变化：...

## 验收与依赖变化

| 项目 | 旧版本 | 新版本 | 影响 |
| --- | --- | --- | --- |
| ... | ... | ... | ... |

## 风险与待确认问题

- ...
```

## Data Rules

- Compare feature scope, business rules, user roles, workflows, data fields, permissions, acceptance criteria, priorities, dependencies, and non-functional requirements.
- Do not invent priorities, acceptance criteria, or product decisions.
- Mark changes as `新增`, `删除`, or `修改` only when source evidence supports the classification.
- Identify ambiguous requirements and missing acceptance details.
- Preserve requirement IDs, module names, version labels, and original terminology.

## Clarification Rules

Ask when multiple product versions exist but ordering is unclear, when the user did not specify modules to compare, or when parser output lacks the relevant requirement sections.
