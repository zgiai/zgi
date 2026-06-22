# Meeting Invitation Reference

## 使用场景

Use for meeting invitations, agenda emails, schedule coordination, meeting rescheduling, meeting confirmation, or sending pre-read material.

Trigger examples: 会议邀约, 会议邀请, 约会议, 调整会议时间, 发送会议议程, meeting invitation, meeting agenda, schedule a meeting, reschedule.

## Payload 示例

```json
{
  "purpose": "Invite the team to a project review meeting.",
  "recipient_role": "colleague",
  "time": "Thursday 15:00",
  "location": "Tencent Meeting",
  "agenda": ["Progress review", "Risk discussion", "Next milestones"],
  "tone": "concise",
  "language": "Chinese"
}
```

## 数据规则

- Include meeting purpose, time, location or meeting link, attendees or audience, and agenda when available.
- If time, location, or link is missing, use placeholders rather than fabricating details.
- For rescheduling, acknowledge inconvenience and clearly state the original and new time if both are provided.
- For external meetings, include preparation material or expected outcome when relevant.

## 质询规则

Call `request_user_input` if the email cannot be useful without a meeting purpose or recipient audience. If the user asks to schedule or send the invitation, clarify that this skill only drafts email text and does not operate calendars or mailboxes.
