---
name: content-summary
description: Summarize and distill existing content from pasted text, chat context, or already parsed attachment content into summaries, TL;DR, key points, action items, risks, conclusions, decisions, meeting notes, or requirements summaries. Use when the user asks for 总结, 摘要, 提炼重点, 归纳, 归纳需求点, 行动项, 风险, 结论, 会议纪要, summarize, summary, TL;DR, key points, action items, risks, conclusions, decisions, or meeting notes.
when_to_use: Use this skill only after the source content is already available in the conversation, pasted by the user, or supplied by the platform attachment parser. Do not use it to read, parse, extract, OCR, or inspect uploaded PDF, DOCX, XLSX, CSV, image, or other files directly. If the user asks to export the summary as Word, PDF, Markdown, or another file, produce the summary first and then route the generated content to file-generator.
runtime_type: prompt
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: list-collapse
  category: document_processing
  scenarios:
    - document_handling
    - office_collaboration
    - knowledge_research
  label:
    en_US: Content Summary
    zh_Hans: 内容总结
  description:
    en_US: Designed for long text, meeting notes, or parsed documents that need faster reading; extracts summaries, key points, actions, risks, conclusions, and meeting minutes.
    zh_Hans: 适用于快速理解长文本、会议记录或已解析文档，可提炼摘要、重点、行动项、风险、结论和会议纪要。
  when_to_use:
    en_US: Use when existing text or extracted document content needs to be summarized or distilled.
    zh_Hans: 当已有文本或文档读取结果需要总结、提炼或归纳时使用。
  tags:
    en_US:
      - Summary
      - Extraction
      - Productivity
    zh_Hans:
      - 总结
      - 提炼
      - 归纳
---

# Content Summary Skill

Use this skill to compress, distill, and structure existing content. This skill does not read files, parse file formats, OCR images, generate files, or create downstream artifacts.

## Scope

Use this skill for:

- Summaries, abstracts, TL;DR, executive summaries, and concise overviews.
- Key points, important facts, decisions, conclusions, assumptions, and open questions.
- Action items, owners, deadlines, dependencies, blockers, and follow-ups.
- Risks, issues, mitigations, tradeoffs, and unresolved concerns.
- Meeting notes, discussion summaries, decisions, and next steps.
- Requirements summaries from already extracted requirement source content.

Do not use this skill to:

- Read, parse, extract, or inspect uploaded files directly.
- Process PDF, DOCX, XLSX, CSV, image, or scanned document bytes.
- Generate Word, PDF, Markdown, TXT, CSV, XLSX, or any downloadable file.
- Invent source facts, owners, dates, decisions, requirements, or risks not present in the source content.

## Routing Rules

- If the user directly pastes text and asks to summarize, use this skill only.
- If the user uploads a file and asks to read, extract, parse, inspect, or preserve original content, rely on the platform's parsed attachment content; do not use this skill unless a summary or distillation is requested.
- If the user uploads a file and asks to summarize, distill, or extract conclusions, use this skill only after the attachment parser has supplied source content in the conversation.
- If the user says "read it and summarize", summarize the parsed attachment content that is already available in the conversation.
- If the user asks to export the summary as Word, PDF, Markdown, or another file, first produce the summary text, then route that text to `file-generator`.

## Workflow

1. Identify the source content. It must be pasted text, prior chat context, parsed attachment content, or another extraction result already available in the conversation.
2. Identify the requested output type: general summary, TL;DR, key points, action items, risks, conclusions, meeting notes, or requirements summary.
3. If the output type is clear, read exactly one relevant reference before writing the summary.
4. Preserve important qualifiers, numbers, dates, owners, decisions, and constraints from the source. Mark uncertain or missing details instead of inventing them.
5. Choose a concise Markdown format unless the user requests another format.
6. If the source is long, prioritize user-requested aspects first, then include only high-signal supporting details.
7. If the user asks for a file export after the summary is prepared, hand the prepared content to `file-generator`.

## Language Rules

- Match the user's requested output language. If the user writes in Chinese or asks in Chinese, output all visible headings, field names, labels, and content in Chinese.
- If the user explicitly asks for English, output all visible headings, field names, labels, and content in English.
- Do not mix Chinese and English section headings in the final answer. Avoid English labels such as `Summary`, `Key Points`, `Action Items`, `Risks`, `Conclusions`, `Keywords`, `TL;DR`, `Owner`, `Due`, and `Notes` in Chinese output.
- For Chinese output, use Chinese labels such as `摘要`, `核心要点`, `行动项`, `风险`, `结论`, `关键词`, `负责人`, `截止时间`, and `备注`.
- Keep product names, protocol names, code identifiers, file formats, API names, and quoted source terms in their original language when translation would change meaning.

## Clarification Workflow

Call `request_user_input` instead of writing a plain clarification when a missing decision blocks a reliable summary. Ask 1-4 focused questions. Options must be concrete answers that can be used directly. A single question may contain at most 5 options.

Ask when:

- No source content is available.
- Multiple possible source contents exist and the target is unclear.
- The user requests a generic "summarize this" but "this" is ambiguous.
- The user asks for a specialized output but the desired angle is unclear, such as risks vs action items vs requirements.
- The user asks for a specific length or audience but does not provide enough detail to satisfy it.

Example `request_user_input` payload:

```json
{
  "message": "I can summarize the content, but need to confirm the target and output shape first.",
  "questions": [
    {
      "id": "source_scope",
      "question": "Which content should I summarize?"
    },
    {
      "id": "summary_type",
      "question": "What kind of summary do you need?",
      "options": [
        { "label": "key points", "description": "Extract the main points." },
        { "label": "TL;DR", "description": "Produce a very short summary." },
        { "label": "action items", "description": "Extract tasks, owners, and follow-ups." },
        { "label": "risks and conclusions", "description": "Extract risks, issues, conclusions, and decisions." },
        { "label": "meeting notes", "description": "Format as meeting notes." }
      ]
    }
  ]
}
```

After calling `request_user_input`, stop the turn and wait for the user's answer.

## References

Read exactly one reference after choosing the output type:

| Requested output | Read reference |
| --- | --- |
| General summary, concise summary, abstract, TL;DR, key points | `general-summary.md` |
| Action items, next steps, follow-ups, owners, deadlines | `action-items.md` |
| Risks, issues, conclusions, decisions, tradeoffs | `risks-conclusions.md` |
| Meeting notes, meeting minutes, agenda recap, decisions and follow-ups | `meeting-notes.md` |
| Requirements summary, requirement points, PRD/MRD summary, scope and acceptance points | `requirements-summary.md` |

If the user asks for multiple output types, read the reference that matches the primary requested deliverable and include secondary sections only when they are directly supported by the source.

## Constraints

- Do not expose internal reasoning or mention this skill.
- Do not output JSON unless the user explicitly asks for JSON.
- Keep final-answer headings and table field names in one language. For Chinese requests, do not use English structural labels such as `summary`, `keywords`, or `action items`.
- Do not claim to have read an uploaded file unless parsed attachment content or another extraction result is already available in the conversation.
- Do not summarize away contradictions, caveats, uncertain facts, or explicit risks when they matter.
- Do not fabricate owners, deadlines, decisions, scores, requirements, or conclusions.
- Do not generate a file directly. Use `file-generator` only after the summary text is prepared and the user requested export.
