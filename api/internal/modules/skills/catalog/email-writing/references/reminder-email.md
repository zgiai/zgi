# Reminder Email Reference

## 使用场景

Use for reminders,催办, deadline follow-ups, overdue tasks, approval chasing, payment reminders, document reminders, or stronger business requests.

Trigger examples: 催办邮件, 提醒邮件, 强提醒, 催客户回复, 催供应商, 催审批, reminder email, overdue reminder, escalation wording.

## Payload 示例

```json
{
  "purpose": "Remind the supplier to send the signed document.",
  "recipient_role": "supplier",
  "deadline": "Friday 18:00",
  "consequence": "Project timeline may be affected",
  "tone": "strong reminder",
  "language": "Chinese"
}
```

## 数据规则

- State the pending item, current status, expected action, and deadline.
- Keep firm wording professional and factual.
- Escalation wording must be based on user-provided consequences or process rules.
- Do not threaten legal action, penalties, fees, contract termination, or reporting unless the user explicitly provided those terms.
- Use placeholders for missing deadline, owner, document name, or requested action.

## 质询规则

Call `request_user_input` if the user asks for a strong reminder but the target action or deadline is missing. Ask for confirmation before adding escalation, penalty, legal, payment, refund, compensation, or contract language.
