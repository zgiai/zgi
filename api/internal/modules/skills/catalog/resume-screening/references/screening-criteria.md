# Screening Criteria

## Use When

Use when the user provides explicit screening conditions such as education, years of experience, city, skills, industry experience, certificates, language ability, or project experience.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and field names only. Do not use `Criteria`, `Result`, `Evidence`, or `Gap`.
- For English requests, use English headings and labels only.

## Output Shape

For Chinese output:

```md
## 筛选条件匹配结果

| 筛选条件 | 简历体现 | 判断 | 依据 |
| --- | --- | --- | --- |
| 学历 | ... | 满足/部分满足/未体现/不满足 | ... |
| 年限 | ... | 满足/部分满足/未体现/不满足 | ... |
| 城市 | ... | 满足/部分满足/未体现/不满足 | ... |
| 技能 | ... | 满足/部分满足/未体现/不满足 | ... |
| 行业经验 | ... | 满足/部分满足/未体现/不满足 | ... |

## 亮点

- ...

## 风险点

- ...

## 待确认问题

- ...
```

## Data Rules

- Evaluate only against user-provided criteria.
- Use `未体现` when the resume does not provide enough evidence.
- Use `部分满足` when evidence supports only part of a criterion.
- Do not infer city, years, education, or certificates unless present in the resume.
- Do not include sensitive attributes as criteria even if the user provides them.
- If a provided criterion is sensitive or discriminatory, say it cannot be used and continue with lawful, job-related criteria.

## Clarification Rules

Ask when criteria are vague, when the user asks for a pass/fail decision without defining must-have requirements, or when multiple candidates need the same criteria applied but the criteria are missing.
