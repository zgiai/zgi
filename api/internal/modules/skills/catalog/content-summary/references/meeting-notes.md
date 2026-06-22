# Meeting Notes

## Use When

Use when the user asks for meeting notes, meeting minutes, discussion recap, decision log, or a summary of a meeting transcript or chat discussion.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and table fields only. Do not use `Meeting Summary`, `Topics Discussed`, `Decisions`, `Action Items`, `Owner`, `Due`, or `Open Questions`.
- For English requests, use English headings and table fields only.

## Output Shape

For Chinese output:

```md
## 会议摘要

...

## 讨论议题

- ...

## 决策

- ...

## 行动项

| 行动项 | 负责人 | 截止时间 |
| --- | --- | --- |
| ... | 未说明 | 未说明 |

## 待确认问题

- ...
```

If the user explicitly asks for English, translate all headings and field names consistently into English.

## Data Rules

- Preserve decisions separately from discussion points.
- Use `未说明` in Chinese output or `Not specified` in English output for missing owners or dates.
- Do not infer attendee names unless clearly present.
- Compress repeated discussion without losing decisions, disagreements, or blockers.
- Keep tone neutral and factual.

## Clarification Rules

Ask when the user provides multiple meetings or chat threads and does not specify which one to summarize.
