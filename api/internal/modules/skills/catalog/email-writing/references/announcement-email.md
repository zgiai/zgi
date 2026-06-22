# Announcement Email Reference

## 使用场景

Use for company announcements, internal notices, policy updates, product notices, system maintenance notices, event announcements, HR notices, or broad communication to a group.

Trigger examples: 通知公告, 内部通知, 维护通知, 政策更新, 活动通知, announcement email, notice email, policy update, maintenance notice.

## Payload 示例

```json
{
  "purpose": "Notify all employees about system maintenance.",
  "audience": "all employees",
  "effective_time": "Saturday 22:00-24:00",
  "impact": "The reporting system will be unavailable.",
  "required_action": "Save work in advance.",
  "tone": "formal",
  "language": "Chinese"
}
```

## 数据规则

- Start with the announcement topic and affected audience.
- Include effective time, scope, impact, required action, contact point, and deadline when available.
- Use bullets for multi-item notices when that improves readability.
- Do not invent policy authority, approval status, legal requirements, or enforcement consequences.
- Use placeholders for missing time, scope, contact person, or action owner.

## 质询规则

Call `request_user_input` if the audience or announcement topic is unclear. Ask before including compliance, HR, legal, financial, or disciplinary consequences that the user did not provide.
