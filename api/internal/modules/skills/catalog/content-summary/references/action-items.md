# Action Items

## Use When

Use when the user asks for action items, next steps, follow-ups, todos, task extraction, owners, deadlines, blockers, or dependencies.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and table fields only. Do not use `Action Items`, `Owner`, `Due`, `Dependency`, `Source Basis`, or `Open Questions`.
- For English requests, use English headings and table fields only.

## Output Shape

For Chinese output:

```md
## 行动项

| 行动项 | 负责人 | 截止时间 | 依赖 | 来源依据 |
| --- | --- | --- | --- | --- |
| ... | 未说明 | 未说明 | 未说明 | ... |

## 待确认问题

- ...
```

If the user explicitly asks for English, translate all headings and field names consistently into English.

## Data Rules

- Extract only actionable tasks supported by the source.
- Do not invent owners or dates. Use `未说明` in Chinese output or `Not specified` in English output when absent.
- Distinguish confirmed actions from suggestions or optional ideas.
- Include dependencies, blockers, and prerequisites when stated.
- Keep each action item outcome-oriented and concise.

## Clarification Rules

Ask when the user wants task assignment but the source lacks owners, or when several source sections contain conflicting actions and the user did not specify which scope to use.
