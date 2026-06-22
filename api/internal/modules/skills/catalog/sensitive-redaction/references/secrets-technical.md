# Secrets And Technical Data Reference

## 使用场景

Use for logs, API traces, code snippets, HTTP requests, URLs, database connection strings, private keys, tokens, passwords, API keys, IP addresses, and sensitive URL parameters.

Trigger examples: 日志脱敏, Token 脱敏, 密钥脱敏, password redaction, API key redaction, secret cleanup, IP 脱敏, URL 参数脱敏.

## Payload 示例

```json
{
  "text": "Authorization: Bearer sk_live_1234567890abcdef\ncallback=https://example.com/cb?token=abc123&user=42",
  "level": "high",
  "strategy": "full",
  "entity_types": ["token", "secret", "password", "private_key", "ip", "url_parameter"]
}
```

## 数据规则

- Always use `high` by default for secrets, tokens, passwords, private keys, logs, and training data.
- Do not partially preserve token, password, private key, or API key values.
- Redact sensitive URL query parameter values while preserving non-sensitive URL structure when useful.
- Do not execute, validate, or call any secret, token, URL, IP, or credential.
- Warn the user to rotate leaked credentials if the source appears to contain real secrets.

## 质询规则

Call `request_user_input` if the user asks to keep parts of secrets, tokens, passwords, or private keys. Confirm the risk before proceeding.
