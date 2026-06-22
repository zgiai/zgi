# Option Rules Reference

## 使用场景

Use when the user needs to choose from a small set of concrete options, including single-choice style quick replies or multi-choice style selection.

Trigger examples: 单选, 多选, 选项框, 快捷选项, 选择维度, 选择格式, 选择风格, quick replies, single choice, multi choice.

## Payload 示例

Single-choice style:

```json
{
  "message": "我需要先确认输出形式。",
  "questions": [
    {
      "id": "output_format",
      "question": "希望输出成哪种形式？",
      "options": [
        { "label": "摘要", "description": "只输出核心结论和重点。" },
        { "label": "表格", "description": "按维度整理，便于查看。" },
        { "label": "详细清单", "description": "逐项列出依据和说明。" }
      ]
    }
  ]
}
```

Multi-choice style:

```json
{
  "message": "我需要确认对比维度，确认后继续处理。",
  "questions": [
    {
      "id": "compare_dimensions",
      "question": "请选择一个或多个对比维度，可直接回复选项名称。",
      "options": [
        { "label": "价格" },
        { "label": "交付时间" },
        { "label": "职责边界" },
        { "label": "风险点" },
        { "label": "功能范围" }
      ]
    }
  ]
}
```

## 数据规则

- Use 2-5 options.
- Each option must be directly usable as an answer.
- For single-choice style questions, options should be mutually exclusive.
- For multi-choice style questions, explicitly say the user may choose one or more option labels.
- Do not use vague options such as `其他`, `随便`, `都可以`, `不确定`, or `看情况`.
- If the valid option set is larger than 5, ask open text or split the question into multiple rounds.

## 质询规则

Use options only when they reduce ambiguity. If options would constrain the user incorrectly, omit options and ask an open text question.
