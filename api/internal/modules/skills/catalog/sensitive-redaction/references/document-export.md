# Document Export Reference

## 使用场景

Use when the user asks to export, save, download, or create a redacted document after redaction.

Trigger examples: 导出脱敏文档, 生成脱敏 Word, 保存脱敏 PDF, 下载脱敏文本, export redacted file, save redacted document.

## Payload 示例

```json
{
  "text": "parsed or pasted document content",
  "level": "high",
  "strategy": "auto"
}
```

After `redact_text` succeeds, pass only the redacted content to `file-generator`.

## 数据规则

- This skill does not read uploaded files directly.
- Use parsed document content already supplied by the platform parser.
- Do not claim original layout preservation.
- Do not pass unredacted content to `file-generator`.
- Recommended formats: `md` for structured editable text, `docx` for Word, `pdf` for read-only sharing, `txt` for plain text.

## 质询规则

Call `request_user_input` if the output format is missing and the user specifically asks for an exported file. Do not ask about format when the user only wants an inline redacted answer.
