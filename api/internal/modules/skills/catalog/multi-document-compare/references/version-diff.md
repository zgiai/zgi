# Version Difference

## Use When

Use for old-versus-new document comparison, contract version comparison, proposal version comparison, policy version comparison, and any request to identify additions, deletions, modifications, unchanged content, or key version changes.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese labels such as `新增`, `删除`, `修改`, `未变化`, `影响`, and `原文依据`.
- For English requests, use English headings and labels only.

## Output Shape

For Chinese output:

```md
## 版本变化摘要

...

## 新增、删除、修改

| 类型 | 位置或条目 | 旧版本 | 新版本 | 变化说明 | 影响 | 原文依据 |
| --- | --- | --- | --- | --- | --- | --- |
| 新增 | ... | 未提及 | ... | ... | ... | ... |

## 重点变化

- ...

## 风险提示

- ...

## 待确认问题

- ...
```

## Data Rules

- Use `新增` when content exists in the newer version but not in the older version.
- Use `删除` when content exists in the older version but not in the newer version.
- Use `修改` when both versions cover the same topic but wording, numbers, scope, responsibility, deadline, amount, or condition changed.
- Use `未变化` only when the source clearly supports no material change for a relevant item.
- Do not infer deletion from parser omission when source quality is incomplete; mark it as `无法判断`.
- Highlight changes that affect price, timeline, obligations, acceptance, risk allocation, scope, or compliance.

## Clarification Rules

Ask when the old and new versions are not identifiable, when there are more than two versions and ordering is unclear, or when parser output is incomplete for critical sections.
