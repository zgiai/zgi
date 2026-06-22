# Apology And Explanation Email Reference

## 使用场景

Use for apologies, delay explanations, mistake corrections, service issue explanations, incident updates, missed deadlines, response delays, or clarifying misunderstandings.

Trigger examples: 道歉邮件, 解释邮件, 延期说明, 出错说明, 客诉回复, apology email, explanation email, delay notice, correction email.

## Payload 示例

```json
{
  "purpose": "Apologize for delayed delivery and explain the next step.",
  "recipient_role": "customer",
  "issue": "Delivery is delayed due to internal review.",
  "next_step": "Send the updated timeline tomorrow.",
  "tone": "polite",
  "language": "Chinese"
}
```

## 数据规则

- Acknowledge the issue plainly without over-explaining.
- Separate apology, factual explanation, current status, next step, and requested understanding or action.
- Use careful wording for responsibility. Do not admit legal liability unless the user explicitly asks and confirms the wording.
- Do not offer refunds, compensation, discounts, penalties, or contract changes unless the user supplied those facts.
- Use placeholders for uncertain times, case numbers, responsible teams, or next actions.

## 质询规则

Call `request_user_input` when the issue, audience, or desired stance is unclear. Ask for confirmation before including legal liability, compensation, refund, contractual commitment, or financial promise.
