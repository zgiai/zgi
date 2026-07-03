# Personal Identifiers Reference

## 使用场景

Use for personal information redaction, including phone numbers, emails, Chinese ID cards, bank cards, addresses, names, candidate names, customer contacts, and similar PII.

Trigger examples: 手机号脱敏, 邮箱脱敏, 身份证脱敏, 银行卡脱敏, 地址脱敏, 姓名脱敏, 个人信息脱敏, PII redaction, privacy cleanup.

## Payload 示例

```json
{
  "text": "姓名：张三，手机号：13812345678，邮箱：zhangsan@example.com",
  "level": "high",
  "strategy": "auto",
  "preserve_rules": {
    "keep_last_digits": 4,
    "keep_email_domain": true,
    "keep_city": false
  }
}
```

## 数据规则

- Use `high` for external sharing, contracts, resumes, HR records, customer data, training data, or public examples.
- Do not return complete original values in field lists.
- For high-risk identifiers such as ID cards and bank cards, prefer full hiding unless the user explicitly requires limited suffix retention.
- Preserve city only when the user explicitly asks and sharing risk is acceptable.
- Mark possible misses in warnings because names and addresses can be context-dependent.

## 质询规则

Call `request_user_input` if no source text is available, if multiple text sources exist, or if the user asks to preserve personal details but does not specify which parts to preserve.
