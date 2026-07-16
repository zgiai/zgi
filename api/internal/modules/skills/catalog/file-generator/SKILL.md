---
name: file-generator
description: Generate downloadable files from provided content, including basic exports, SVG files, styled DOCX documents, HTML-rendered PDFs, and static editable PPTX decks.
when_to_use: Use this skill when the user asks to create, export, save, download, or produce a file such as TXT, Markdown, HTML, JSON, CSV, SVG, DOCX, XLSX, PDF, or PPTX.
provider_type: builtin
provider_id: file_generator
runtime_type: tool
tools:
  - generate_file
  - generate_docx
  - generate_pdf
  - generate_pptx
supported_callers:
  - aichat
  - agent
  - workflow
max_calls_per_turn: 5
timeout_seconds: 60
tool_governance:
  generate_file:
    tool_id: file.generate
    skill_id: file-generator
    domain: files
    effect: create
    asset_type: file
    risk_level: medium
    requires_asset_resolution: false
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - file:create
    default_approval_policy: never_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  generate_docx:
    tool_id: file.generate_docx
    skill_id: file-generator
    domain: files
    effect: create
    asset_type: file
    risk_level: medium
    requires_asset_resolution: false
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - file:create
    default_approval_policy: never_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  generate_pdf:
    tool_id: file.generate_pdf
    skill_id: file-generator
    domain: files
    effect: create
    asset_type: file
    risk_level: medium
    requires_asset_resolution: false
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - file:create
    default_approval_policy: never_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
  generate_pptx:
    tool_id: file.generate_pptx
    skill_id: file-generator
    domain: files
    effect: create
    asset_type: file
    risk_level: medium
    requires_asset_resolution: false
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - file:create
    default_approval_policy: never_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
display:
  icon: file-plus-2
  category: document_processing
  scenarios:
    - document_handling
    - office_collaboration
    - content_creation
  label:
    en_US: File Generator
    zh_Hans: 文件生成器
  description:
    en_US: Designed for delivering results as downloadable files; turns prepared content into TXT, Markdown, HTML, JSON, CSV, SVG, DOCX, XLSX, PDF, or PPTX documents.
    zh_Hans: 适用于将结果作为文件交付，可把已准备好的内容生成 TXT、Markdown、HTML、JSON、CSV、SVG、DOCX、XLSX、PDF 或 PPTX 文件。
  when_to_use:
    en_US: Use when the answer should be delivered as a generated file.
    zh_Hans: 当回答需要以生成文件交付时启用。
  tags:
    en_US:
      - File
      - Export
    zh_Hans:
      - 文件
      - 导出
---

# File Generator Skill

Use this skill when the user wants an answer delivered as a downloadable file artifact.
This skill creates new workflow files; it does not edit an existing uploaded file.

## Workflow

1. Decide the target file format from the user's request.
2. If the user did not specify a format, choose the most appropriate supported format:
   - Use `md` for reports, notes, plans, structured prose, and documentation drafts.
   - Use `docx` when the user explicitly asks for Word, an editable document, or a formal document file.
   - Use `xlsx` when the user explicitly asks for Excel or a spreadsheet workbook. Provide valid CSV content.
   - Use `pdf` when the user explicitly asks for PDF or a simple read-only distribution file.
   - Use `pptx` when the user explicitly asks for PowerPoint, slides, a deck, or an editable presentation.
   - Use `txt` for plain text without formatting.
   - Use `html` for runnable HTML pages or previewable web documents.
   - Use `svg` when the user explicitly asks for an SVG/vector image file.
   - Use `json` for machine-readable structured data.
   - Use `csv` for table-like data.
3. For PDF, DOCX, PPTX, XLSX, and SVG, read the selected format reference before calling a tool. For MD, TXT, JSON, CSV, and HTML, the tool schema and this core skill are sufficient unless advanced format details are needed.
4. Use `generate_file` for simple exports.
5. Use `generate_docx` when a Word document needs headings, fonts, font sizes, alignment, spacing, page margins, page breaks, colored/bold/underlined text, or simple tables.
6. Use `generate_pdf` when a PDF needs richer layout, print CSS, tables, colors, page margins, page breaks, or business-report formatting.
7. Use `generate_pptx` when the user needs an editable static PowerPoint deck.
8. For PPTX, plan the slide layout before writing JSON. Use non-overlapping boxes for titles, text, and tables. Split dense content into more slides instead of shrinking text or stacking elements.
9. For PPTX with Chinese or other dense text, treat "more text" as more slides, not denser text boxes. Keep body paragraphs short; use bullets, columns, or additional slides before exceeding the readable capacity of a box.
10. Before calling `generate_pptx`, self-check that each slide's readable content fits within the slide bounds, does not overlap, uses explicit `x`, `y`, `w`, and `h` when the layout matters, and avoids long unbroken lines.
11. Always create a temporary downloadable artifact. This skill does not write generated output into File Management.
12. If the user explicitly asks to save, create, upload, add, or import the generated result into File Management or the current Files page, first generate the artifact here, then use `file-manager/save_file_to_management` with the returned `tool_file_id`/`file_id` and destination filename.
13. For file-first requests, put the complete body directly in the generation tool call. Do not first emit the same complete body through `submit_intermediate_answer`. Only show the complete body in chat as well when the user explicitly asks to see both the chat text and the file.
14. In the final answer, briefly mention the generated filename and format.
15. Do not invent, rewrite, shorten, or manually format download links. The system UI displays generated file download controls from structured artifact events.

## References

Read exactly one reference after choosing a complex target format. References for MD, TXT, JSON, CSV, and HTML are optional and should be read only when the request needs format-specific details:

| Requested format | Read reference |
| --- | --- |
| `txt`, `text` | `format-txt.md` |
| `md`, `markdown` | `format-md.md` |
| `html`, `htm` | `format-html.md` |
| `json` | `format-json.md` |
| `csv` | `format-csv.md` |
| `svg` | `format-svg.md` |
| simple `docx`, `word` | `format-docx.md` |
| styled `docx`, `word` | `format-docx-rich.md` |
| `xlsx`, `excel` | `format-xlsx.md` |
| simple `pdf` | `format-pdf.md` |
| HTML-rendered `pdf` | `format-pdf-html.md` |
| `pptx`, `powerpoint`, `presentation`, `slides` | `format-pptx.md` |

## Constraints

- The skill creates new files only. It does not edit existing uploaded files or preserve an existing template.
- Do not call `generate_docx`, `generate_pdf`, `generate_pptx`, or complex `generate_file` formats (XLSX or SVG) until the selected format reference has been read. MD, TXT, JSON, CSV, and HTML generation may proceed directly from the core skill and tool schema.
- `generate_file` accepts only its documented simple parameters. Do not invent format-specific parameters such as `sheets`, `styles`, `pages`, `columns`, `headers`, or `metadata`. XLSX output applies the default table styling documented in `format-xlsx.md`; the caller cannot customize those styles through `generate_file`.
- `generate_docx` accepts a JSON string document specification. Do not pass raw Markdown or HTML as `document`.
- `generate_pdf` accepts self-contained HTML and optional inline CSS. Do not pass JSON document specs.
- `generate_pptx` accepts a JSON string presentation specification. Do not pass HTML or Markdown as `presentation`.
- PPTX readable content should not use negative coordinates. Negative `x` or `y` is only appropriate for decorative shapes that intentionally bleed off an edge.
- Generated file content must fit within the backend file size limit.
- Do not include filesystem paths in filenames.
- Do not claim a file was generated unless the tool call succeeded.
- Treat the generated tool result as authoritative. A generated file is successful only when the result includes a file identity such as `file_id` or `tool_file_id`, plus a filename/format or an equivalent artifact record.
- If the generation tool fails or returns no artifact identity, do not say the file was generated. Explain the actual failure or missing confirmation.
- Retry at most once with corrected content or arguments when the tool error is recoverable. Do not repeat the same generation call with identical arguments after a failure.
- If the user asks for advanced document features that the selected reference says are unsupported, generate the closest supported file only when that still satisfies the request.

## Tool Usage

`generate_file` accepts:

- `content`: text content to write into the file.
- `format`: `txt`, `md`, `html`, `json`, `csv`, `svg`, `docx`, `xlsx`, or `pdf`.
- `filename`: optional display filename. The extension is added automatically.
- `title`: optional title used by generated HTML, XLSX, and PDF files. For XLSX, this becomes a merged title row above the table.
- `lifecycle`: optional temporary artifact lifecycle, `persistent` or `temporary`. Defaults to `temporary`.

`generate_docx` accepts:

- `document`: JSON string describing the styled DOCX document.
- `filename`: optional display filename. The `.docx` extension is added automatically.
- `title`: optional title hint. Visible content must be in `document.blocks`.
- `lifecycle`: optional temporary artifact lifecycle, `persistent` or `temporary`. Defaults to `temporary`.

`generate_pdf` accepts:

- `html`: self-contained HTML body or full HTML document.
- `css`: optional inline CSS appended to the document.
- `filename`: optional display filename. The `.pdf` extension is added automatically.
- `title`: optional title used when wrapping an HTML fragment. Visible content must be in `html`.
- `lifecycle`: optional temporary artifact lifecycle, `persistent` or `temporary`. Defaults to `temporary`.

`generate_pptx` accepts:

- `presentation`: JSON string describing the editable static PPTX deck.
- `filename`: optional display filename. The `.pptx` extension is added automatically.
- `title`: optional title hint. Visible content must be in `presentation.slides`.
- `lifecycle`: optional temporary artifact lifecycle, `persistent` or `temporary`. Defaults to `temporary`.
