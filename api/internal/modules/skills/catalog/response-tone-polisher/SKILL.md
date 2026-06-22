---
name: response-tone-polisher
description: Rewrite existing assistant replies, service replies, workplace messages, or explanatory answers to sound more natural, human, warm, polite, concise, professional, empathetic, and easy to understand while preserving the original facts and boundaries. Use when the user asks for 回复口吻优化, 语气优化, 改得自然一点, 去 AI 味, 减少机械感, 像正常人说话, 温和一点, 亲切一点, 礼貌一点, 客服话术优化, 办公回复优化, 心理关怀回复, 面向普通用户, polish response, rewrite tone, make it natural, make it warmer, make it less robotic, or humanize this reply.
when_to_use: Use this skill when the user provides existing reply text or prior assistant output and wants only the wording, tone, warmth, clarity, politeness, or human feel improved. This skill does not create new facts, commitments, policies, legal claims, medical claims, promises, files, emails to send, or prompts for downstream generation tools. If the user asks to export the optimized reply as Word, PDF, Markdown, or another file, produce the optimized reply first and then route it to file-generator.
runtime_type: prompt
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: message-circle-heart
  category: productivity
  label:
    en_US: Response Tone Polisher
    zh_Hans: 回复口吻优化
  description:
    en_US: Makes existing replies more natural, warm, polite, professional, and easy to understand.
    zh_Hans: 将已有回复改写得更自然、温和、亲切、专业、易懂，减少机械感。
  when_to_use:
    en_US: Use when an existing reply needs tone, wording, warmth, or clarity optimization without changing facts.
    zh_Hans: 当已有回复需要优化语气、表达、亲和度或可读性且不能改变事实时使用。
  tags:
    en_US:
      - Tone
      - Rewrite
      - Communication
    zh_Hans:
      - 口吻
      - 改写
      - 沟通
---

# Response Tone Polisher Skill

Use this skill to rewrite an existing reply so it sounds more like a normal person speaking: natural, warm, polite, concise, professional, empathetic, and easy to understand. This skill changes expression only; it must preserve the original meaning, facts, boundaries, uncertainty, and intent.

## Scope

Use this skill for:

- Making assistant replies less mechanical, less templated, less stiff, or less "AI-like".
- Polishing customer service replies, after-sales explanations, complaint responses, and support messages.
- Rewriting workplace replies, collaboration messages, status explanations, and professional responses.
- Softening or warming replies for ordinary users.
- Rewriting supportive or emotionally sensitive responses with more empathy and care.
- Shortening, smoothing, or simplifying wording while keeping the key information intact.

Do not use this skill to:

- Create new facts, promises, policies, deadlines, prices, compensation, refunds, legal positions, medical advice, or guarantees.
- Turn a refusal, limitation, uncertainty, or safety boundary into agreement.
- Hide important risks, warnings, constraints, missing information, or required next steps.
- Write a full email workflow, send an email, or claim that a message was sent.
- Optimize prompts for downstream tools.
- Summarize source content as the primary task.
- Read, parse, extract, OCR, or inspect uploaded files directly.
- Generate Word, PDF, Markdown, TXT, or any downloadable file directly.

## Routing Rules

- If the user provides a reply and asks to make it more natural, warmer, more polite, less robotic, or easier to understand, use this skill.
- If the user asks to write or polish a business email, route to `email-writing`.
- If the user asks to optimize a prompt, route to `prompt-professionalizer`.
- If the user asks to summarize or extract key points from content, route to `content-summary`.
- If the user uploads a file and asks to polish text inside it, use this skill only after the system document parser has supplied the text to polish.
- If the user asks to export the polished reply, produce the optimized reply first, then route that content to `file-generator`.

## Workflow

1. Identify the exact original reply or text to polish.
2. Identify the target scenario: customer service, workplace professional, empathy support, general user, or concise friendly.
3. If the scenario is clear, read exactly one relevant reference before producing the rewrite.
4. Preserve facts, names, dates, amounts, conditions, caveats, refusals, limitations, and next steps from the original text.
5. Improve only wording, tone, flow, warmth, politeness, clarity, and readability.
6. If the user asks for multiple versions, label them clearly in the user's language.
7. If any information is missing, keep the original uncertainty or use neutral placeholders instead of inventing details.
8. If the user requests export after the optimized reply is prepared, hand the prepared content to `file-generator`.

## Clarification Workflow

Use this workflow to call `request_user_input` instead of writing a plain clarification when a missing decision blocks a reliable rewrite. Ask 1-4 focused questions. Options must be concrete answers that can be used directly. A single question may contain at most 5 options.

Ask when:

- No original reply or source text is available.
- Multiple possible replies exist and the user did not specify which one to polish.
- The target audience or scenario is unclear and would materially change the tone.
- The user asks to make a sensitive reply more certain, stronger, or more reassuring, but the original content does not support that certainty.
- The reply involves refunds, compensation, contracts, legal liability, medical issues, psychological crisis, safety risk, disciplinary action, pricing, or official policy and the requested tone could add an unsupported commitment.

Example `request_user_input` payload:

```json
{
  "message": "我可以优化这段回复的口吻，但需要先确认目标场景。",
  "questions": [
    {
      "id": "target_scenario",
      "question": "这段回复主要用于什么场景？",
      "options": [
        { "label": "客服沟通", "description": "更礼貌、耐心、清楚，不新增承诺。" },
        { "label": "办公沟通", "description": "更简洁、专业、适合协作场景。" },
        { "label": "心理关怀", "description": "更温和、共情、支持性更强。" },
        { "label": "普通用户", "description": "减少术语，更自然易懂。" },
        { "label": "简洁友好", "description": "压缩表达，同时保留关键信息。" }
      ]
    }
  ]
}
```

After calling `request_user_input`, stop the turn and wait for the user's answer.

## Language Rules

- Match the user's requested output language. If the user writes in Chinese or asks in Chinese, output all visible headings, field names, labels, and content in Chinese.
- If the user explicitly asks for English, output all visible headings, field names, labels, and content in English.
- Do not mix Chinese and English section headings in the final answer. Avoid English labels such as `Summary`, `Tone`, `Rewrite`, `Keywords`, `Notes`, and `Version` in Chinese output.
- For Chinese output, use Chinese labels such as `优化后回复`, `修改说明`, `版本一`, `版本二`, `注意事项`, and `保留的信息`.
- Keep product names, company names, code identifiers, API names, legal terms quoted from source, and exact user-provided terms in their original language when translation would change meaning.

## References

Read exactly one reference after choosing the target scenario:

| Target scenario | Read reference |
| --- | --- |
| Customer service, support, after-sales, complaint, apology, explanation | `customer-service.md` |
| Workplace, professional collaboration, reporting, manager or colleague reply | `workplace-professional.md` |
| Emotional support, empathy, comfort, psychological care, sensitive personal situation | `empathy-support.md` |
| Ordinary user, public-facing explanation, user education, less jargon | `general-user.md` |
| Shorten, make friendlier, make more conversational, reduce template feel | `concise-friendly.md` |

If the user asks for multiple tones, read the reference that matches the primary audience and produce multiple versions only when the same facts can be preserved.

## Constraints

- Do not expose internal reasoning or mention this skill.
- Do not output JSON unless the user explicitly asks for JSON.
- Do not add facts, promises, deadlines, compensation, policy statements, medical advice, legal advice, safety guarantees, or product capabilities not present in the original text.
- Do not remove important caveats, limitations, risks, safety guidance, or uncertainty from the original text.
- Do not make the reply manipulative, deceptive, coercive, or falsely intimate.
- Do not imitate a specific living person, copyrighted character voice, or brand voice unless the user provides authorized style guidelines and the rewrite remains generic.
- Do not claim that a file was read unless parsed text is already available in the conversation.
- Do not generate files directly. Use `file-generator` only after the optimized reply is prepared and the user requested export.
