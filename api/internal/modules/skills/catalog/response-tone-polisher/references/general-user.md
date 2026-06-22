# General User Tone Reference

## 使用场景

Use when the reply is for ordinary users, public-facing product explanations, help center answers, educational explanations, troubleshooting guidance, onboarding, or non-expert audiences.

Trigger examples: 面向普通用户, 说得像人话, 少点术语, 更好懂, 自然一点, 亲切一点, user-friendly, plain language, make it easier to understand.

## Payload 示例

```json
{
  "original_reply": "The operation failed because the credential token expired.",
  "target_scenario": "general user",
  "tone": "clear and friendly",
  "audience": "non-technical users"
}
```

## 数据规则

- Preserve what happened, why it happened, what the user can do next, and any constraints.
- Replace unnecessary jargon with plain language, but keep product names, field names, and exact error codes when useful.
- Use short sentences and concrete next steps.
- Do not remove limitations, required checks, or warning information.
- Do not add troubleshooting steps, product capabilities, account permissions, or policy claims not present in the original text.
- If a technical term must remain, explain it briefly in plain language.

## 输出建议

For Chinese output, default to a direct optimized reply without extra commentary:

```md
## 优化后回复

...
```

If the user asks for "只给结果", omit the heading and output only the rewritten reply.

## 质询规则

Call `request_user_input` if the original text has multiple possible audiences and the right level of detail is unclear, such as beginner user, advanced user, enterprise admin, developer, or customer decision-maker.
