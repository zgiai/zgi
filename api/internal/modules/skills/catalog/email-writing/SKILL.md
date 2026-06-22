---
name: email-writing
description: Draft and polish formal, clear, business-appropriate emails in Chinese or English. Use when the user asks to write, draft, generate, optimize, rewrite, or polish business emails, customer follow-up emails, meeting invitations, reminders, apology or explanation emails, announcements, report delivery emails, supplier emails, candidate emails, bilingual emails, 商务邮件, 邮件撰写, 写邮件, 客户跟进, 会议邀约, 催办邮件, 道歉邮件, 解释说明, 通知公告, 周报邮件, 月报邮件, 润色邮件, 英文邮件, draft email, follow-up email, meeting invitation, reminder email, apology email, announcement email, or email polishing.
when_to_use: Use this skill only to generate or polish email subject lines, email bodies, alternate versions, and follow-up wording. Do not send emails, schedule emails, create external email drafts, generate attachments, or invent commitments, prices, contract terms, legal conclusions, or financial promises.
runtime_type: prompt
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: mail-pen
  category: productivity
  label:
    en_US: Email Writing
    zh_Hans: 邮件撰写
  description:
    en_US: Drafts and polishes business emails with clear subject lines, tone, audience, and follow-up wording.
    zh_Hans: 生成或润色商务邮件，覆盖主题、正文、语气、对象和后续跟进话术。
  when_to_use:
    en_US: Use when the user needs a business email, follow-up email, meeting invitation, reminder, apology, announcement, report delivery note, or email polishing.
    zh_Hans: 当用户需要商务沟通、客户跟进、会议邀约、催办、道歉解释、通知公告、报告发送或邮件润色时使用。
  tags:
    en_US:
      - Email
      - Business Writing
      - Productivity
    zh_Hans:
      - 邮件
      - 商务写作
      - 效率
---

# Email Writing Skill

Use this skill to draft or polish email content. The skill only produces text for the user to review and copy. It never sends email, schedules delivery, creates external drafts, or generates attachments.

## Scope

Use this skill for:

- Business communication emails.
- Customer or partner follow-up emails.
- Meeting invitations, agenda emails, and schedule coordination.
- Reminder,催办, escalation, and deadline follow-up emails.
- Apology, explanation, delay, correction, or incident communication emails.
- Notification, announcement, policy update, or internal bulletin emails.
- Weekly report, monthly report, project update, or attachment delivery emails.
- Polishing, rewriting, shortening, formalizing, or translating an existing email draft.
- Generating multiple versions, such as concise, formal, friendly, or stronger reminder versions.

Do not use this skill for:

- Sending emails or operating any mailbox.
- Creating calendar events, external drafts, or scheduled sends.
- Generating downloadable files or attachments.
- Summarizing source documents as the main task.
- Creating legal, financial, pricing, contract, or compliance commitments not provided by the user.

## Workflow

1. Identify the email scenario and read exactly one matching reference before writing.
2. Determine the recipient identity, language, purpose, key facts, expected tone, and whether the user wants one or multiple versions.
3. If the missing information blocks a reliable email, call `request_user_input` instead of writing a normal Markdown clarification.
4. If non-critical facts are missing, use explicit placeholders such as `【时间】`, `【地点】`, `【附件名称】`, `【收件人姓名】`, or `[time]`.
5. Draft a subject line and body that match the user's language, audience, and tone.
6. If the user supplied an existing draft, preserve its factual meaning and intent. Improve structure, tone, clarity, and grammar without adding new facts.
7. If the user asks for export to Word, PDF, Markdown, or another file, first prepare the email text, then route the prepared content to `file-generator`.

## Clarification Workflow

Call `request_user_input` when a required decision is missing. Ask 1-4 focused questions. Each question may contain at most 5 shortcut options, and every option must be a directly usable answer.

Ask when:

- The email purpose is unclear.
- The recipient identity or relationship is missing and affects tone.
- The requested tone is important but ambiguous, especially for reminders, apologies, or sensitive business communication.
- The user asks for multiple versions but does not specify the number or dimensions.
- The user asks for English, bilingual, or translation but the target language is unclear.
- The email depends on legal, financial, pricing, contract, compensation, refund, or delivery commitments that the user has not supplied.

Example `request_user_input` payload:

```json
{
  "message": "I can draft the email, but need to confirm the key writing choices first.",
  "questions": [
    {
      "id": "email_purpose",
      "question": "What is the main purpose of the email?"
    },
    {
      "id": "recipient_role",
      "question": "Who is the recipient?",
      "options": [
        { "label": "customer", "description": "Write for an external customer." },
        { "label": "leader", "description": "Write upward to a manager or executive." },
        { "label": "colleague", "description": "Write to an internal peer." },
        { "label": "supplier", "description": "Write to an external supplier or vendor." },
        { "label": "candidate", "description": "Write to a recruiting candidate." }
      ]
    },
    {
      "id": "tone",
      "question": "What tone should the email use?",
      "options": [
        { "label": "formal", "description": "Use a professional and complete style." },
        { "label": "concise", "description": "Keep it short and direct." },
        { "label": "polite", "description": "Emphasize courtesy and cooperation." },
        { "label": "strong reminder", "description": "Make the ask firm while staying professional." },
        { "label": "friendly", "description": "Use a warmer business tone." }
      ]
    }
  ]
}
```

After calling `request_user_input`, stop the turn and wait for the user's answer.

## References

Read exactly one reference after choosing the scenario:

| Scenario | Read reference |
| --- | --- |
| General business communication, cross-team or external business email | `business-email.md` |
| Customer, partner, supplier, or candidate follow-up | `follow-up-email.md` |
| Meeting invitation, agenda, schedule coordination, or calendar-related email | `meeting-invitation.md` |
| Reminder,催办, deadline follow-up, stronger request, or escalation wording | `reminder-email.md` |
| Apology, explanation, delay, correction, incident explanation, or service issue | `apology-explanation.md` |
| Notice, announcement, policy update, internal bulletin, or broad communication | `announcement-email.md` |
| Weekly report, monthly report, project update, attachment delivery, or report sharing | `report-delivery.md` |
| Polishing, rewriting, shortening, translating, or improving an existing draft | `polish-existing-draft.md` |

If the user asks for multiple scenarios, choose the reference for the primary deliverable and include secondary elements only when they are directly relevant.

## Output Contract

Default output:

- `邮件主题` / `Subject`
- `邮件正文` / `Body`

Optional output when requested or useful:

- `简短版` / `Concise version`
- `正式版` / `Formal version`
- `跟进话术` / `Follow-up wording`
- `需确认事项` / `Items to confirm`

For Chinese requests, use Chinese headings. For English requests, use English headings. Do not mix Chinese and English structural labels unless the user asks for bilingual output.

## Constraints

- Do not expose internal reasoning or mention this skill.
- Do not output JSON unless the user explicitly asks for JSON.
- Do not claim the email has been sent, scheduled, saved, or attached.
- Do not invent prices, discounts, refunds, compensation, deadlines, legal statements, contract terms, delivery commitments, or approval status.
- If legal, financial, pricing, contract, compensation, refund, or compliance wording is involved, mark it as requiring user confirmation.
- Use placeholders for missing non-critical facts instead of inventing them.
- Preserve all user-supplied facts, names, dates, product names, amounts, and constraints unless the user asks to remove them.
- Do not summarize away user-provided requirements. If the user later asks for summarization, table generation, file generation, or other downstream processing, route to the appropriate skill after the email text is prepared.
