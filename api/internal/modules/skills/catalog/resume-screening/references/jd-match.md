# JD Match

## Use When

Use when resume content and a JD or role requirements are available, and the user asks whether the candidate matches the role, whether to enter the next round, or what the match evidence and gaps are.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and field names only. Do not use `Fit`, `Match`, `Pros`, `Cons`, `Risk`, or `Next Round`.
- For English requests, use English headings and labels only.

## Output Shape

For Chinese output:

```md
## 候选人摘要

...

## 匹配度判断

匹配度：高/中/低

## 匹配依据

| 岗位要求 | 简历证据 | 判断 |
| --- | --- | --- |
| ... | ... | 匹配/部分匹配/未体现 |

## 不匹配或待确认原因

- ...

## 亮点

- ...

## 风险点

- ...

## 面试建议问题

- ...

## 是否建议进入下一轮

建议：是/否/需补充确认

依据：...
```

## Data Rules

- Compare against JD must-have requirements first, then preferred requirements.
- Use `匹配`, `部分匹配`, `未体现`, or `不匹配` for each requirement.
- Use `高`, `中`, or `低` only when the JD or explicit criteria are available.
- Do not produce a final hiring or rejection decision.
- Do not infer missing skills from job titles alone; require resume evidence.
- Do not use sensitive attributes as match evidence.
- Keep next-round recommendation preliminary and evidence-based.

## Clarification Rules

Ask when the user requests suitability or next-round judgment but no JD or criteria are available, or when there are multiple resumes and JDs without a clear mapping.
