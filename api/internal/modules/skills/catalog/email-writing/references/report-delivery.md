# Report Delivery Email Reference

## 使用场景

Use for sending weekly reports, monthly reports, project reports, analysis reports, meeting minutes, status updates, or attachments by email.

Trigger examples: 周报邮件, 月报邮件, 发送报告邮件, 项目进展邮件, 附件发送邮件, report delivery email, weekly report email, monthly report email, attached report.

## Payload 示例

```json
{
  "purpose": "Send the monthly operations report to leadership.",
  "recipient_role": "leader",
  "report_period": "May 2026",
  "attachment": "May operations report",
  "key_points": ["Revenue increased", "Delivery risks remain", "Need decision on staffing"],
  "tone": "concise",
  "language": "Chinese"
}
```

## 数据规则

- Mention report name, reporting period, attachment or link, and requested action.
- Provide a brief context summary only if the user supplied key points.
- Do not summarize or analyze an attached report unless the report content is already provided and the user asks for it.
- Do not claim an attachment is included unless the user supplied the attachment information; use `【附件名称】` when missing.
- Do not fabricate metrics, conclusions, approvals, or decisions.

## 质询规则

Call `request_user_input` if the recipient role, report type, or requested action is unclear. If the user needs the report itself summarized, route the summarization task to the appropriate content-summary flow after source content is available.
