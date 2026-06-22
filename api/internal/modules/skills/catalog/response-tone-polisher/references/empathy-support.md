# Empathy Support Tone Reference

## 使用场景

Use when the reply is for emotional support, psychological care, comfort, encouragement, sensitive personal situations, user frustration, sadness, anxiety, stress, grief, or disappointment.

Trigger examples: 心理关怀, 安慰一下, 温柔一点, 共情一点, 陪伴感, 别太冰冷, emotional support, empathetic response, make it warmer.

## Payload 示例

```json
{
  "original_reply": "You should rest and ask for help if needed.",
  "target_scenario": "empathy support",
  "tone": "warm and supportive",
  "safety_boundary": "do not provide diagnosis"
}
```

## 数据规则

- Preserve the original advice, boundaries, uncertainty, and safety guidance.
- Use warm, grounded, respectful language. Avoid judgment, blame, pressure, or empty reassurance.
- Do not diagnose, prescribe treatment, promise outcomes, or replace professional help.
- Do not say "一定会好", "我完全懂你", or other overconfident claims unless the original text supports them.
- If there is self-harm, violence, abuse, medical emergency, or immediate safety risk in the source context, keep or add appropriate immediate-help guidance without dramatizing.
- Avoid making the assistant sound like it has a real personal relationship, professional license, or human lived experience.

## 输出建议

For Chinese output, prefer direct warm prose. Use headings only if the user asks for versions or explanation.

```md
## 优化后回复

...
```

## 质询规则

Call `request_user_input` when the user asks to make a crisis, self-harm, medical, legal, or abuse-related reply "more comforting" in a way that might remove safety guidance. Confirm whether to preserve safety guidance and crisis resources.
