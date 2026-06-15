# Validation Guidance Reference

## 使用场景

Use when the workflow needs required values, expected formats, numeric ranges, date formats, count limits, or file type hints, but the current frontend contract cannot enforce validation fields.

Trigger examples: 必填, 格式, 日期格式, 数字范围, 文件类型, 校验, validation, required field, format hint.

## Payload 示例

```json
{
  "message": "我需要补齐几个必填信息，避免后续生成失败。",
  "questions": [
    {
      "id": "quantity",
      "question": "需要生成几张图片？请输入 1-4 的数字。"
    },
    {
      "id": "aspect_ratio",
      "question": "请选择图片比例。",
      "options": [
        { "label": "1:1" },
        { "label": "16:9" },
        { "label": "9:16" },
        { "label": "4:3" }
      ]
    }
  ]
}
```

## 数据规则

- Put validation hints in the visible `question` text.
- Use concrete examples such as `YYYY-MM-DD`, `1-4`, `PDF/DOCX/TXT`, or `中文/英文`.
- Do not add unsupported machine validation fields.
- If a value is required by a downstream tool, state that it is required in natural language.
- If user input may still be invalid after submission, the downstream skill or tool must validate it before execution.

## 质询规则

Ask for required fields before optional preferences. Do not ask for optional formatting details when missing required data already blocks the workflow.
