# Talent Pool Entry

## Use When

Use when the user asks to extract structured resume information for talent pool entry, candidate database intake, CRM-like records, or recruiting pipeline notes.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and field names only. Do not use `Candidate`, `Profile`, `Tags`, or `Notes`.
- For English requests, use English headings and labels only.

## Output Shape

For Chinese output:

```md
## 人才库信息

| 字段 | 内容 |
| --- | --- |
| 姓名 | 简历未体现 |
| 联系方式 | 简历未体现 |
| 城市 | 简历未体现 |
| 最高学历 | 简历未体现 |
| 工作年限 | 简历未体现 |
| 当前或最近公司 | 简历未体现 |
| 当前或最近岗位 | 简历未体现 |
| 行业经验 | 简历未体现 |
| 技能标签 | 简历未体现 |
| 证书 | 简历未体现 |

## 经历摘要

- ...

## 标签建议

- ...

## 待补充信息

- ...
```

## Data Rules

- Extract structured fields only from resume content.
- Use `简历未体现` for missing or unclear fields.
- Do not include sensitive attributes in talent pool fields unless the user explicitly needs to preserve source text for a lawful reason; never use them for evaluation.
- Normalize skill tags conservatively. Do not add skills not supported by resume text.
- Keep contact information concise and do not invent missing channels.

## Clarification Rules

Ask when the user wants a specific talent pool schema but has not provided field names, or when multiple candidates are present in the same content.
