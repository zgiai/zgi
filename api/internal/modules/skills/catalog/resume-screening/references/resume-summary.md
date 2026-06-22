# Resume Summary

## Use When

Use when the user provides resume content and asks for candidate summary, resume extraction, structured resume information, or general capability extraction without a JD.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and field names only. Do not use `Summary`, `Keywords`, `Experience`, `Skills`, or `Notes`.
- For English requests, use English headings and labels only.

## Output Shape

For Chinese output:

```md
## 候选人摘要

...

## 结构化信息

| 项目 | 内容 |
| --- | --- |
| 姓名 | 简历未体现 |
| 联系方式 | 简历未体现 |
| 城市 | 简历未体现 |
| 学历 | 简历未体现 |
| 工作年限 | 简历未体现 |

## 核心经历

| 公司 | 岗位 | 时间 | 职责或成果 |
| --- | --- | --- | --- |
| ... | ... | ... | ... |

## 技能标签

- ...

## 亮点

- ...

## 风险点

- ...

## 疑问点

- ...
```

Add this note when no JD is provided: `说明：未提供岗位JD或筛选条件，本次不做岗位匹配度判断。`

## Data Rules

- Extract only facts present in the resume content.
- Use `简历未体现` for missing name, contact, city, education, years, dates, skills, certificates, or other fields.
- Do not infer working years if dates are absent or unclear; mark as `简历未体现` or `需确认`.
- Group experience by company, role, project, industry, responsibility, and achievement when available.
- Keep technical stacks, tools, languages, and certificates as source terms when appropriate.
- Do not use sensitive attributes in summary or evaluation.

## Clarification Rules

Ask when no resume content is available, or when multiple resumes exist and the user did not specify which candidate to summarize.
