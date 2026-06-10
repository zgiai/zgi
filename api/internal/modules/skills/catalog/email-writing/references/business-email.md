# Business Email Reference

## 使用场景

Use for general business communication, including cross-team coordination, customer-facing business updates, supplier communication, leadership updates, and formal workplace emails that do not fit a more specific reference.

Trigger examples: 商务邮件, 正式邮件, 业务沟通, 给客户写邮件, 给领导汇报, supplier email, business email, formal email.

## Payload 示例

```json
{
  "purpose": "Explain the project status and request confirmation on the next milestone.",
  "recipient_role": "customer",
  "key_points": ["Phase 1 completed", "Need confirmation before Friday", "Attach progress report"],
  "tone": "formal",
  "language": "Chinese",
  "versions": ["default"]
}
```

## 数据规则

- Include a clear subject line.
- Open with context, then state the main purpose early.
- Keep one paragraph focused on one business point.
- Convert missing non-critical details into placeholders, for example `【项目名称】`, `【截止时间】`, `【附件名称】`.
- Do not invent commercial terms, delivery dates, approvals, prices, discounts, compensation, or legal commitments.
- If the user gives exact facts, preserve them.

## 质询规则

Call `request_user_input` if the purpose or recipient relationship is unclear. Ask for tone when the recipient is external or senior and the user did not specify a tone. Ask for confirmation before writing any legal, financial, pricing, contract, refund, or compensation commitment that was not provided.
