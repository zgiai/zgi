# Workplace Professional Tone Reference

## 使用场景

Use when the reply is for workplace communication, internal collaboration, manager updates, colleague replies, cross-team coordination, project status, meeting follow-up, work handoff, or professional explanation.

Trigger examples: 办公回复, 工作沟通, 汇报口吻, 给领导回复, 给同事回复, 专业一点, 简洁专业, workplace reply, professional tone, manager update.

## Payload 示例

```json
{
  "original_reply": "This is not ready because product has not confirmed the scope.",
  "target_scenario": "workplace professional",
  "tone": "concise and professional",
  "audience": "manager"
}
```

## 数据规则

- Preserve task status, owner, blockers, decisions, dates, dependencies, risks, and next steps.
- Make the reply concise, respectful, factual, and easy to scan.
- Avoid blame-shifting; express blockers as objective dependencies.
- Do not invent approvals, commitments, schedule changes, resource allocation, business decisions, or completed work.
- Do not over-soften critical risks or blockers.
- If the user asks for a more direct version, keep it professional and non-aggressive.

## 输出建议

For Chinese output, prefer:

```md
## 优化后回复

...

## 保留的信息

- ...
```

Use `保留的信息` only when the user asks to explain changes or when important constraints may be easy to miss.

## 质询规则

Call `request_user_input` when the target recipient is materially important but missing, such as direct manager, customer, vendor, HR, legal, or cross-team stakeholder. Ask before changing wording that affects responsibility, deadline, commitment, escalation, or approval status.
