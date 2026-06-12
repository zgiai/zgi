# Clarification Rules

## Use When

Use when the request is too vague to produce a reliable professional prompt, or when the target tool type is unclear.

## Ask First

Call `request_user_input` before final output when:

- Target tool or prompt type is unknown.
- Image request lacks subject or style.
- Video request lacks action or duration.
- Architecture diagram request lacks modules or diagram type.
- Data visualization request lacks fields or analysis goal.
- The user asks for a "professional prompt" but gives no use case.
- The request is very short and defaults would materially change the result.

## Question Limits

- Ask at most 3 questions in one turn.
- Prefer choices with directly usable labels.
- Each question can have at most 5 options.
- Do not ask about minor taste details unless they change the output.
- Do not continue asking for more than two clarification rounds.

## Default Assumptions

Use defaults when:

- The user says not to ask.
- Missing fields are minor.
- The user asks for a template.
- The target type is clear enough and a conservative default is safe.

Always label defaults under `默认假设`.

## Good Questions

- "这个提示词主要给哪类工具使用？"
- "最重要的生成目标是什么？"
- "希望输出中文、英文还是中英双版？"
- "架构图需要展示模块关系、部署架构、业务流程，还是时序调用？"
- "数据可视化的核心指标和维度是什么？"

## Bad Questions

Avoid:

- Asking for too many style preferences at once.
- Asking questions already answered by the user.
- Asking for hidden internal details when a public-facing prompt is enough.
- Asking for exact data values when only a prompt template is requested.
