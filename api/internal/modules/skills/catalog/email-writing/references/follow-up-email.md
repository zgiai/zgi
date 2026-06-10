# Follow-Up Email Reference

## 使用场景

Use for customer follow-up, sales follow-up, supplier follow-up, candidate follow-up, partner communication, post-meeting follow-up, or checking progress after a previous conversation.

Trigger examples: 客户跟进, 跟进邮件, 会后跟进, 供应商跟进, 候选人跟进, follow-up email, checking in, next step email.

## Payload 示例

```json
{
  "purpose": "Follow up after the product demo and ask whether the customer has feedback.",
  "recipient_role": "customer",
  "context": "Demo completed last Wednesday.",
  "key_points": ["Ask for feedback", "Offer a short call", "Share pricing only if user supplied it"],
  "tone": "polite",
  "language": "English"
}
```

## 数据规则

- Mention the prior interaction if provided.
- Make the ask specific: feedback, confirmation, next call, missing material, or next decision.
- Keep the CTA easy to answer.
- Use placeholders for missing dates, materials, links, or names.
- Do not imply agreement, purchase intent, approval, or commitment unless the user supplied that fact.

## 质询规则

Call `request_user_input` when the follow-up target is unclear, such as feedback vs decision vs payment vs document delivery. For sales or supplier contexts, ask before including pricing, delivery, discount, or payment language that the user did not provide.
