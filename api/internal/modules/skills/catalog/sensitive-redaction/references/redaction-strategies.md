# Redaction Strategies Reference

## 使用场景

Use when the user's main request is about redaction strength, partial preservation, label replacement, suffix retention, email domain retention, city retention, or choosing low/medium/high strategy.

Trigger examples: 保留后四位, 保留域名, 保留城市, 部分脱敏, 完全隐藏, 替换为标签, low redaction, high redaction, label replacement.

## Payload 示例

```json
{
  "text": "手机号 13812345678，邮箱 user@example.com",
  "level": "medium",
  "strategy": "partial",
  "preserve_rules": {
    "keep_last_digits": 4,
    "keep_email_domain": true,
    "keep_city": false,
    "keep_url_domain": true
  }
}
```

## 数据规则

- `low`: preserve more context, only for low-risk internal usage.
- `medium`: partial masking for internal sharing where traceability is useful.
- `high`: full hiding by default for external sharing, public examples, logs, model training, contracts, resumes, HR data, customer data, and uncertain risk.
- `partial`: preserve safe prefixes or suffixes where supported.
- `full`: replace with redacted markers.
- `label`: replace with entity labels such as `[PHONE]` or `[EMAIL]`.
- Secrets, tokens, passwords, and private keys remain fully hidden even under partial strategy.

## 质询规则

Call `request_user_input` when the user requests partial preservation but does not specify which fields or how much to preserve. Confirm low-strength redaction when the destination is external sharing or training data.
