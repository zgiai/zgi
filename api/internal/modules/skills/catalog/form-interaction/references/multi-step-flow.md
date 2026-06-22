# Multi-step Flow Reference

## 使用场景

Use when the workflow has conditional questions, depends on earlier answers, or would require too many questions in one card.

Trigger examples: 分步表单, 多轮确认, 先确认再继续, 根据选择继续问, multi-step form, conditional flow.

## Payload 示例

First step:

```json
{
  "message": "我先确认处理方向，再根据你的选择继续。",
  "questions": [
    {
      "id": "task_mode",
      "question": "这次主要想完成什么？",
      "options": [
        { "label": "生成摘要", "description": "提炼核心内容。" },
        { "label": "输出行动项", "description": "整理待办、负责人和时间。" },
        { "label": "识别风险", "description": "提炼风险、问题和影响。" }
      ]
    }
  ]
}
```

Second step after answer:

```json
{
  "message": "已确认处理方向，还需要补充输出要求。",
  "questions": [
    {
      "id": "output_length",
      "question": "希望结果多详细？",
      "options": [
        { "label": "简短", "description": "只保留最关键内容。" },
        { "label": "标准", "description": "包含主要依据和说明。" },
        { "label": "详细", "description": "逐项展开说明。" }
      ]
    }
  ]
}
```

## 数据规则

- Ask the smallest useful set of questions in each step.
- Stop the turn after each `request_user_input` call.
- Do not call downstream tools in the same turn after asking.
- Use the user's answer to decide the next step.
- Avoid asking more than 3 questions unless all are truly blocking.

## 质询规则

Use a multi-step flow when there are more than 4 blocking questions, when an answer determines later questions, or when too many options would make the card hard to use.
