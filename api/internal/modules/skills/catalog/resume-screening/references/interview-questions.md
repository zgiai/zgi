# Interview Questions

## Use When

Use when the user asks for interview questions based on a resume, JD, screening risks, candidate highlights, project depth, skill validation, or business fit.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and labels only. Do not use `Interview Questions`, `Risk`, `Skill`, or `Follow-up`.
- For English requests, use English headings and labels only.

## Output Shape

For Chinese output:

```md
## 面试建议问题

### 经历核验

- ...

### 项目深度

- ...

### 技能掌握

- ...

### 业务理解

- ...

### 风险点追问

- ...

## 不建议提问的问题

- 涉及年龄、性别、婚育、民族、宗教、健康等敏感属性的问题。
```

## Data Rules

- Base questions on resume evidence, JD requirements, stated risks, or unclear points.
- Prefer behavioral and evidence-seeking questions.
- Include follow-up probes for achievements, project ownership, technical depth, collaboration, tradeoffs, and measurable outcomes.
- Do not ask questions about gender, age, marital or childbearing status, ethnicity, religion, health, disability, or other sensitive attributes.
- Do not assume fraud or exaggeration; phrase questions neutrally.

## Clarification Rules

Ask when the interview type is unclear and it affects question design, such as HR screen, technical interview, business interview, leadership interview, or final interview.
