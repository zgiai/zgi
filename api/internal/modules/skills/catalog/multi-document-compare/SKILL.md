---
name: multi-document-compare
description: Compare two or more existing parsed documents, pasted document texts, or document contents from chat context to identify similarities, differences, additions, deletions, modifications, conflicts, risks, version changes, and dimension-based comparisons. Use when the user asks for 多文档对比, 文档对比, 版本对比, 合同对比, 方案对比, PRD对比, 招投标文件对比, 制度新旧版本对比, 供应商方案比较, 差异点, 共同点, 冲突点, 新增删除修改, 横向比较, compare documents, document comparison, version comparison, diff, conflict clauses, or vendor comparison.
when_to_use: Use this skill only after two or more document contents are already available from pasted text, chat context, or the system document parser. Do not use it to directly read, parse, extract, OCR, or inspect uploaded PDF, Word, TXT, Markdown, scanned files, or other file bytes. If the user asks to export the comparison as Word, PDF, Markdown, or another file, produce the comparison content first and then route it to file-generator.
runtime_type: prompt
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: files
  category: productivity
  label:
    en_US: Multi-document Compare
    zh_Hans: 多文档对比
  description:
    en_US: Compares parsed document content for differences, conflicts, risks, and version changes.
    zh_Hans: 对已解析文档内容进行差异、冲突、风险和版本变化对比。
  when_to_use:
    en_US: Use when two or more available document contents need comparison.
    zh_Hans: 当两个或多个已有文档内容需要对比时使用。
  tags:
    en_US:
      - Compare
      - Documents
      - Differences
    zh_Hans:
      - 对比
      - 文档
      - 差异
---

# Multi-document Compare Skill

Use this skill to compare two or more available document contents. This skill performs comparison and synthesis only; it does not read files, parse file formats, OCR images, generate redlines, or create downloadable files.

## Scope

Use this skill for:

- Contract version comparison and clause difference review.
- Proposal version comparison and solution change tracking.
- PRD or requirements document change comparison.
- Tender, bid, and procurement document comparison.
- Old-versus-new policy, procedure, or制度 comparison.
- Multi-vendor proposal comparison across dimensions such as price, delivery, responsibilities, timeline, risks, and functions.
- Identifying common points, differences, additions, deletions, modifications, conflicts, risks, and key version changes.

Do not use this skill to:

- Directly read, parse, extract, or inspect uploaded files or file bytes.
- Replace the system document parser.
- Produce legal, compliance, procurement, or professional review as a final authority.
- Make strong conclusions from unreadable, low-quality, incomplete, scanned, garbled, or partially parsed source content.
- Generate Word, PDF, Markdown, TXT, CSV, XLSX, or any downloadable file directly.
- Invent document content, clauses, prices, dates, responsibilities, risks, or conflicts not grounded in source text.

## Routing Rules

- If two or more document contents are already available from pasted text, chat context, or the system document parser, use this skill directly.
- If the user uploads files and asks to compare them, use this skill only after the system document parser has provided structured content for at least two documents.
- If the user only asks to read, extract, parse, inspect, or preserve original document text, do not use this skill; that is a document parsing task.
- If the user says "read and compare", the system document parser must provide the source content first, then this skill compares the parsed content.
- If the user asks to export the comparison result, produce the comparison content first, then route that content to `file-generator`.

## Workflow

1. Confirm that at least two source documents are available as text, structured parsed content, or prior chat context.
2. Identify each document's name, version, date, source label, or role when available. If labels are missing, use neutral labels such as `文档A`, `文档B`, and `文档C` in Chinese output.
3. Identify the comparison focus: generic comparison, version changes, clause conflicts, vendor comparison, requirements changes, or policy changes.
4. If the comparison focus is clear, read exactly one relevant reference before producing the comparison.
5. Extract shared points, differences, additions, deletions, modifications, conflicts, and risks only from source-backed content.
6. Preserve original numbers, dates, amounts, delivery times, responsibilities, clause names, requirement IDs, and scope constraints.
7. Clearly separate document-stated facts from inferred risks or recommendations.
8. If source quality is insufficient, state what cannot be judged and why.
9. If the user requests export after the comparison is prepared, hand the prepared content to `file-generator`.

## Clarification Workflow

Call `request_user_input` instead of writing a plain clarification when a missing decision blocks a reliable comparison. Ask 1-4 focused questions. Options must be concrete answers that can be used directly. A single question may contain at most 5 options.

Ask when:

- Fewer than two document contents are available.
- Multiple parsed documents exist but the user did not specify which ones to compare.
- The user says only "compare these documents" and the comparison focus is ambiguous.
- The user did not specify whether they want summary, table, or detailed differences and the output depth matters.
- The parsed source content is incomplete, garbled, scanned poorly, or missing critical sections.
- The requested dimensions are unclear or too broad to compare reliably.

Example `request_user_input` payload:

```json
{
  "message": "我可以对比这些文档，但需要先确认对比重点和输出形式。",
  "questions": [
    {
      "id": "compare_focus",
      "question": "重点按什么方向对比？",
      "options": [
        { "label": "版本变化", "description": "标记新增、删除、修改内容。" },
        { "label": "条款冲突", "description": "识别不一致、矛盾或冲突描述。" },
        { "label": "供应商横向比较", "description": "按价格、交付、职责、风险等维度比较。" },
        { "label": "需求变更", "description": "对比功能、范围、验收和依赖变化。" },
        { "label": "综合对比", "description": "输出共同点、差异点、冲突点和风险。" }
      ]
    },
    {
      "id": "output_format",
      "question": "希望用什么输出形式？",
      "options": [
        { "label": "摘要", "description": "先给重点变化和结论。" },
        { "label": "表格", "description": "按维度输出对比表。" },
        { "label": "详细差异", "description": "逐项列出新增、删除、修改和依据。" }
      ]
    }
  ]
}
```

After calling `request_user_input`, stop the turn and wait for the user's answer.

## Language Rules

- Match the user's requested output language. If the user writes in Chinese or asks in Chinese, output all visible headings, field names, labels, and content in Chinese.
- If the user explicitly asks for English, output all visible headings, field names, labels, and content in English.
- Do not mix Chinese and English section headings in the final answer. Avoid English labels such as `Summary`, `Diff`, `Comparison Table`, `Keywords`, `Risks`, `Owner`, `Due`, and `Notes` in Chinese output.
- For Chinese output, use Chinese labels such as `对比摘要`, `共同点`, `差异清单`, `新增、删除、修改`, `冲突点`, `风险提示`, `原文依据`, and `待确认问题`.
- Keep product names, company names, protocol names, code identifiers, file formats, API names, and quoted source terms in their original language when translation would change meaning.

## References

Read exactly one reference after choosing the comparison focus:

| Requested comparison | Read reference |
| --- | --- |
| Generic comparison, common points, differences, conflicts, risks | `generic-comparison.md` |
| Version comparison, new/old version, additions, deletions, modifications | `version-diff.md` |
| Contract clauses, conflict clauses, inconsistent responsibilities, payment, delivery, acceptance, breach terms | `clause-conflict.md` |
| Vendor comparison, supplier proposals, bidding documents, price, delivery, service, risk, function | `vendor-comparison.md` |
| PRD comparison, requirements changes, product scope, feature changes, acceptance changes | `requirements-diff.md` |
| Policy or制度 comparison, old/new procedures, scope, responsibilities, approvals, constraints | `policy-comparison.md` |

If the user asks for multiple comparison types, read the reference that matches the primary deliverable and include secondary sections only when they are directly supported by source content.

## Constraints

- Do not expose internal reasoning or mention this skill.
- Do not output JSON unless the user explicitly asks for JSON.
- Do not directly read, parse, extract, OCR, or inspect uploaded files.
- Do not claim that files were parsed unless the system document parser already supplied parsed content.
- Do not replace legal review, compliance review, procurement review, or other professional review.
- Do not make strong conclusions when source content is missing, unreadable, incomplete, scanned poorly, garbled, or only partially parsed.
- Every comparison conclusion must be grounded in document source text, parsed source content, or explicit user-provided content.
- Do not fabricate additions, deletions, modifications, conflicts, risks, prices, dates, deadlines, responsibilities, or clauses.
- Do not generate files directly. Use `file-generator` only after the comparison content is prepared and the user requested export.
