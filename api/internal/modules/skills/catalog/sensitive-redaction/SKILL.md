---
name: sensitive-redaction
description: Detect and redact sensitive information from existing text or already parsed document content before sharing, archiving, model training, logging, ticket handoff, contract sharing, resume sharing, customer chat export, or external delivery. Use when the user asks for 脱敏, 敏感信息隐藏, 隐私清洗, 去标识化, 匿名化, 手机号脱敏, 邮箱脱敏, 身份证脱敏, 银行卡脱敏, 地址脱敏, 姓名脱敏, 公司名脱敏, 客户名脱敏, 订单号脱敏, 合同号脱敏, 日志脱敏, Token 脱敏, 密钥脱敏, password redaction, PII redaction, anonymization, de-identification, privacy cleanup, redact sensitive data, or mask secrets.
when_to_use: Use this skill only to redact sensitive information from text already available in the conversation, pasted by the user, or supplied by the platform attachment parser. Do not directly read, parse, OCR, or inspect uploaded files or images. If the user asks to export the redacted result as Word, PDF, Markdown, TXT, CSV, or another file, redact the content first and then route the redacted content to file-generator.
provider_type: builtin
provider_id: sensitive_redaction
runtime_type: hybrid
tools:
  - redact_text
max_calls_per_turn: 5
timeout_seconds: 10
display:
  icon: shield-alert
  category: security
  label:
    en_US: Sensitive Redaction
    zh_Hans: 敏感信息脱敏
  description:
    en_US: Detects and hides sensitive text before sharing, archiving, training, or export.
    zh_Hans: 在分享、归档、训练或导出前识别并隐藏敏感文本。
  when_to_use:
    en_US: Use when existing text or parsed document content needs PII, business identifier, or secret redaction.
    zh_Hans: 当已有文本或解析后的文档内容需要个人信息、业务标识或密钥脱敏时使用。
  tags:
    en_US:
      - Privacy
      - Security
      - Redaction
    zh_Hans:
      - 隐私
      - 安全
      - 脱敏
---

# Sensitive Redaction Skill

Use this skill to redact sensitive information from existing text. This skill does not read files, parse file formats, OCR images, generate files directly, or store original sensitive data.

## Scope

Use this skill for:

- Customer chat logs, support tickets, call notes, and service records before external sharing.
- Contracts, resumes, business records, and parsed document content before forwarding or archiving.
- Application logs, API traces, error reports, and debugging snippets before issue handoff.
- Business data cleanup before model training, evaluation, demos, or public examples.
- Text that contains phones, emails, ID cards, bank cards, names, companies, customers, orders, contracts, addresses, secrets, tokens, passwords, IPs, or sensitive URL parameters.

Do not use this skill to:

- Directly read, parse, OCR, or inspect uploaded PDF, Word, spreadsheet, image, screenshot, or other file bytes.
- Preserve the original exact file layout or edit binary documents in place.
- Guarantee 100% detection of every sensitive field.
- Store or repeat complete original sensitive values.
- Generate a downloadable file directly.

## Routing Rules

- If the user pasted text or the platform already supplied parsed document content, use this skill directly.
- If the user uploads a document and asks for redaction, use this skill only after the platform attachment parser has supplied source text in the conversation.
- If the user uploads an image or screenshot and asks for redaction, do not use this skill unless OCR or parsed text is already available.
- If the user asks to export the redacted result, first call `redact_text`, then pass only the redacted content to `file-generator`.
- If the user asks to summarize, classify, or transform the content after redaction, complete redaction first, then route the redacted content to the appropriate downstream skill.

## Workflow

1. Confirm source text is available in the conversation or parsed attachment content.
2. Identify the redaction scenario and read exactly one reference before calling `redact_text`.
3. Choose a redaction level:
   - Use `high` for external sharing, public examples, training data, logs, contracts, resumes, customer data, legal/financial/HR data, or when unsure.
   - Use `medium` for internal sharing where partial traceability is acceptable.
   - Use `low` only when the user explicitly asks to preserve more context.
4. Choose a strategy:
   - Use `auto` unless the user explicitly requests partial retention, full hiding, or label replacement.
   - Use `full` for secrets, tokens, passwords, private keys, and other high-risk credentials.
   - Use `label` when the user wants clean placeholders such as `[PHONE]` or `[EMAIL]`.
5. Call `redact_text` with the source text, level, strategy, preserve rules, and optional entity type filter.
6. Return the redacted text, sensitive type statistics, redacted field list, and risk warnings.
7. Do not show complete original sensitive values in the final answer.
8. If export is requested, pass the redacted text to `file-generator`; never pass unredacted source content to file generation.

## Clarification Workflow

Call `request_user_input` instead of writing a plain clarification when missing information blocks reliable redaction. Ask 1-4 focused questions. Options must be concrete answers that can be used directly. A single question may contain at most 5 options.

Ask when:

- No source text or parsed content is available.
- Multiple source texts exist and the target is unclear.
- The user asks for a custom preserve rule but does not specify what to preserve.
- The user asks for document export but the output format is missing.
- The user explicitly wants low-strength or partial redaction for high-risk external sharing; confirm the risk before proceeding.

Example `request_user_input` payload:

```json
{
  "message": "I can redact the content, but need to confirm the redaction target and output choice first.",
  "questions": [
    {
      "id": "source_scope",
      "question": "Which content should I redact?"
    },
    {
      "id": "redaction_level",
      "question": "What redaction level should I use?",
      "options": [
        { "label": "high", "description": "Fully hide high-risk fields for external sharing, logs, contracts, resumes, or training data." },
        { "label": "medium", "description": "Partially preserve low-risk context for internal sharing." },
        { "label": "low", "description": "Preserve more context; use only when the risk is low." }
      ]
    },
    {
      "id": "strategy",
      "question": "How should matched fields be replaced?",
      "options": [
        { "label": "auto", "description": "Let the tool choose based on level and risk." },
        { "label": "partial", "description": "Keep limited prefixes or suffixes where safe." },
        { "label": "full", "description": "Replace sensitive values with redacted markers." },
        { "label": "label", "description": "Replace values with type labels such as [PHONE]." }
      ]
    }
  ]
}
```

After calling `request_user_input`, stop the turn and wait for the user's answer.

## References

Read exactly one reference after choosing the scenario:

| Scenario | Read reference |
| --- | --- |
| Phones, emails, ID cards, bank cards, names, addresses, personal identifiers | `personal-identifiers.md` |
| Companies, customer names, order numbers, contract numbers, tickets, business identifiers | `business-identifiers.md` |
| API keys, tokens, passwords, private keys, IPs, URLs, logs, technical traces | `secrets-technical.md` |
| User asks about low/medium/high, partial/full/label, or preserve rules | `redaction-strategies.md` |
| User asks to export, save, download, or create a redacted file | `document-export.md` |

If multiple scenarios apply, read the reference for the highest-risk primary content. For mixed personal and secret data, use `secrets-technical.md`.

## Tool Usage

`redact_text` accepts:

- `text`: required source text.
- `level`: optional `low`, `medium`, or `high`. Defaults to `medium`.
- `strategy`: optional `auto`, `partial`, `full`, or `label`. Defaults to `auto`.
- `preserve_rules`: optional object or JSON object string with `keep_last_digits`, `keep_email_domain`, `keep_city`, and `keep_url_domain`.
- `entity_types`: optional array, JSON array string, or comma-separated list to limit matching.
- `locale`: optional `auto`, `zh-CN`, or `en-US`.
- `include_field_list`: optional boolean. Defaults to true.

## Output Rules

Default answer sections:

- `脱敏后内容` / `Redacted content`
- `敏感信息统计` / `Sensitive information statistics`
- `脱敏字段列表` / `Redacted field list`
- `风险提示` / `Risk warnings`

Do not include complete original sensitive values in the field list. Show only redacted replacements and counts.

## Constraints

- Do not expose internal reasoning or mention this skill.
- Do not output JSON unless the user explicitly asks for JSON.
- Do not directly read, parse, extract, OCR, or inspect uploaded files or images.
- Do not claim that every sensitive field was found.
- Always tell the user that rule-based redaction may miss context-dependent or malformed sensitive information and requires review before external sharing.
- Do not store, repeat, or export unredacted sensitive content.
- Use high redaction for secrets, tokens, passwords, private keys, training data, logs, contracts, resumes, HR data, customer data, and external sharing unless the user explicitly chooses otherwise.
- Do not generate files directly. Use `file-generator` only after `redact_text` succeeds and only with redacted content.
