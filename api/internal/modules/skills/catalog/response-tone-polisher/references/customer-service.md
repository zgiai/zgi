# Customer Service Tone Reference

## 使用场景

Use when the reply is for customer service, support, after-sales, complaint handling, account issues, product usage explanations, apologies, delay explanations, or service recovery.

Trigger examples: 客服回复, 售后话术, 投诉回复, 道歉说明, 安抚客户, 礼貌一点, 耐心一点, support reply, customer service tone, complaint response.

## Payload 示例

```json
{
  "original_reply": "We cannot process this now. Please wait.",
  "target_scenario": "customer service",
  "tone": "polite and patient",
  "must_preserve": ["no confirmed processing time", "user needs to wait"]
}
```

## 数据规则

- Preserve service facts, available options, limitations, escalation path, order status, dates, amounts, refund rules, and policy boundaries.
- Make the reply respectful, patient, clear, and action-oriented.
- Acknowledge inconvenience only when the original context supports it.
- Do not add refunds, compensation, replacement, account changes, approval status, deadlines, legal liability, or special handling unless present in the original text.
- Do not promise "马上处理", "一定解决", " guaranteed", or exact completion time unless already provided.
- If the original text is a refusal, keep the refusal clear while softening the wording.

## 输出建议

For Chinese output, prefer:

```md
## 优化后回复

...

## 修改说明

- ...
```

Only include `修改说明` when useful or requested. For direct rewrite requests, output just the optimized reply.

## 质询规则

Call `request_user_input` when the reply involves refund, compensation, legal liability, complaint escalation, pricing, contract terms, or official policy and the user asks for stronger certainty not supported by the original text. Ask for the allowed commitment boundary before rewriting.
