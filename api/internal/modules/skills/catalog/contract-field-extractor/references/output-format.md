# Output Format

Use this reference to format the extraction result.

## Default JSON Output

Return JSON by default unless the user requests another format.

```json
{
  "contract_summary": {
    "source_name": "",
    "extraction_time": "",
    "field_count": 0,
    "extracted_count": 0,
    "missing_count": 0,
    "uncertain_count": 0
  },
  "fields": [
    {
      "field_key": "contract_amount",
      "field_label": "合同金额",
      "value": "人民币 120,000 元",
      "normalized_value": "120000",
      "value_type": "money",
      "extraction_status": "extracted",
      "confidence": 0.92,
      "evidence": "合同总价为人民币壹拾贰万元整（￥120,000.00）",
      "source_location": "第 3 条 合同价款",
      "notes": ""
    }
  ],
  "review_warnings": []
}
```

## Field Object Rules

- Preserve the configured field order.
- Include every configured field.
- Use `未提取到` for missing values only when no explicit configured default is used.
- Use `defaulted` when an explicit configured default is used instead of contract text.
- Keep `confidence` numeric from `0` to `1`.
- Keep `evidence` concise.
- Use `notes` for conflicts, defaults, and uncertainty.

## Markdown Output

When the user asks for readable output, use a table with:

| 字段 | 值 | 状态 | 置信度 | 证据 | 备注 |
| --- | --- | --- | --- | --- | --- |

## CSV Output

Use headers:

```csv
field_key,field_label,value,normalized_value,value_type,extraction_status,confidence,evidence,source_location,notes
```

Escape commas, quotes, and newlines according to CSV rules.
