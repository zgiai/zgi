# Question Shapes Reference

## 使用场景

Use when the workflow needs open text, date or number as text, confirmation, or a file upload prompt using the current `request_user_input` contract.

Trigger examples: 输入框, 文本输入, 日期, 数字, 确认一下, 上传文件, 补充说明, open text, confirmation, file upload prompt.

## Payload 示例

Open text:

```json
{
  "message": "我需要先确认标题，确认后继续生成。",
  "questions": [
    {
      "id": "title",
      "question": "请填写要展示的标题。"
    }
  ]
}
```

Date as text:

```json
{
  "message": "我需要确认日期，确认后继续安排计划。",
  "questions": [
    {
      "id": "target_date",
      "question": "请按 YYYY-MM-DD 输入目标日期。"
    }
  ]
}
```

Confirmation:

```json
{
  "message": "继续前请确认以下处理方式。",
  "questions": [
    {
      "id": "confirm_plan",
      "question": "是否按当前方案继续？",
      "options": [
        { "label": "确认继续", "description": "按当前方案进入下一步。" },
        { "label": "需要修改", "description": "先补充修改意见。" }
      ]
    }
  ]
}
```

## 数据规则

- Use open text when the answer cannot be represented by 2-5 concrete options.
- For dates and numbers, state the expected format in the visible question.
- For confirmation, options must be concrete actions.
- Do not add unsupported fields such as `type`, `placeholder`, or `validation`.
- Keep each question short and directly tied to the blocker.

## 质询规则

Ask only for information needed to continue. If the workflow needs both a date and a description, ask both in the same card only when both are blocking.
