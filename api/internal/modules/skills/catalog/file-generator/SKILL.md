---
name: file-generator
description: Generate downloadable files from provided text content.
when_to_use: Use this skill when the user asks to create, export, save, download, or produce a file such as TXT, Markdown, HTML, JSON, CSV, DOCX, XLSX, or PDF.
provider_type: builtin
provider_id: file_generator
runtime_type: tool
tools:
  - generate_file
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: file-plus
  category: productivity
  label:
    en_US: File Generator
    zh_Hans: 文件生成器
  description:
    en_US: Creates downloadable TXT, Markdown, HTML, JSON, CSV, DOCX, XLSX, and PDF files.
    zh_Hans: 创建可下载的 TXT、Markdown、HTML、JSON、CSV、DOCX、XLSX 和 PDF 文件。
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
This skill turns text content into a workflow file; it does not write to a local filesystem path.

## Workflow

1. Decide the target file format from the user's request.
2. If the user did not specify a format, choose the most appropriate supported format:
   - Use `md` for reports, notes, plans, structured prose, and documentation drafts.
   - Use `docx` when the user explicitly asks for Word, an editable document, or a formal document file.
   - Use `xlsx` when the user explicitly asks for Excel or a spreadsheet workbook. Provide valid CSV content.
   - Use `pdf` when the user explicitly asks for PDF or a simple read-only distribution file.
   - Use `txt` for plain text without formatting.
   - Use `html` for runnable HTML pages or previewable web documents.
   - Use `json` for machine-readable structured data.
   - Use `csv` for table-like data.
3. Read the reference document for the selected format before calling `generate_file`.
4. Shape `content`, `title`, and `filename` according to that reference.
5. Call `call_skill_tool` with `tool_name` set to `generate_file`.
6. Use `persistent` lifecycle by default unless the user asks for a temporary file.
7. In the final answer, briefly mention the generated filename and format.
8. Do not invent, rewrite, shorten, or manually format download links. The system UI displays generated file download controls from structured artifact events.

## References

Read exactly one reference after choosing the target format:

| Requested format | Read reference |
| --- | --- |
| `txt`, `text` | `format-txt.md` |
| `md`, `markdown` | `format-md.md` |
| `html`, `htm` | `format-html.md` |
| `json` | `format-json.md` |
| `csv` | `format-csv.md` |
| `docx`, `word` | `format-docx.md` |
| `xlsx`, `excel` | `format-xlsx.md` |
| `pdf` | `format-pdf.md` |

## Constraints

- The tool has one unified parameter schema for every format. Format-specific expectations live in the reference documents.
- Do not call `generate_file` until the selected format reference has been read.
- The tool accepts only the documented parameters below. Do not invent format-specific parameters such as `sheets`, `styles`, `pages`, `columns`, `headers`, or `metadata`.
- Generated file content must fit within the backend file size limit.
- Do not include filesystem paths in filenames.
- Do not claim a file was generated unless the `generate_file` tool call succeeded.
- If the user asks for advanced document features that the selected reference says are unsupported, generate the closest basic file only when that still satisfies the request.

## Tool Usage

`generate_file` accepts:

- `content`: text content to write into the file.
- `format`: `txt`, `md`, `html`, `json`, `csv`, `docx`, `xlsx`, or `pdf`.
- `filename`: optional display filename. The extension is added automatically.
- `title`: optional title used by generated HTML and PDF files.
- `lifecycle`: optional file lifecycle, `persistent` or `temporary`. Defaults to `persistent`.
