# Concise Friendly Tone Reference

## 使用场景

Use when the user asks to make a reply shorter, more conversational, less templated, less robotic, friendlier, smoother, or suitable for quick chat.

Trigger examples: 简洁一点, 口语一点, 自然一点, 去模板感, 不要太官方, 短一点, 亲切一点, concise friendly, less robotic, more conversational.

## Payload 示例

```json
{
  "original_reply": "Thank you for your feedback. We will evaluate it carefully and provide an update when we have more information.",
  "target_scenario": "concise friendly",
  "tone": "short and warm"
}
```

## 数据规则

- Preserve the core message, key constraints, required action, and any important caveats.
- Remove repetitive openings, filler, over-formal phrasing, and template-like transitions.
- Keep the result warm but not overly intimate.
- Do not delete safety boundaries, refusal reasons, deadlines, amounts, owners, or next steps.
- Do not add confidence, promises, or certainty not present in the original text.
- If shortening would remove critical information, keep the critical information and explain that it was retained.

## 输出建议

For Chinese output:

```md
## 优化后回复

...
```

If multiple lengths are requested, use:

```md
## 简洁版

...

## 更温和版

...
```

## 质询规则

Call `request_user_input` when the user asks for a much shorter version but the original text contains legal, safety, medical, financial, privacy, or policy constraints that cannot be safely removed.
