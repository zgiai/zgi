# Polish Existing Draft Reference

## 使用场景

Use when the user provides an existing email draft and asks to polish, rewrite, shorten, formalize, translate, make it friendlier, make it stronger, or produce alternate versions.

Trigger examples: 润色邮件, 改写邮件, 优化邮件, 邮件语气更正式, 邮件简洁一点, 英文邮件润色, polish email, rewrite email, make it more formal, shorten this email.

## Payload 示例

```json
{
  "draft": "Hi John, checking if you saw my last email...",
  "target_change": "Make it more polite and professional.",
  "recipient_role": "customer",
  "tone": "polite",
  "language": "English",
  "versions": ["polished", "concise"]
}
```

## 数据规则

- Preserve the original facts, intent, names, dates, amounts, constraints, and requested action.
- Improve structure, clarity, grammar, politeness, and business tone.
- Do not add new commitments, claims, evidence, deadlines, attachments, prices, or legal language.
- If information is missing but not required, keep the draft's wording or add placeholders.
- If the user asks for multiple versions, label each version clearly.

## 质询规则

Call `request_user_input` if the desired change is ambiguous and affects the result, such as "make it better" without a target tone or audience. Ask before changing sensitive legal, financial, pricing, contract, compensation, or complaint-handling language.
