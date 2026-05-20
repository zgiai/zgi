---
name: file-generator
description: Generate downloadable text-based files from provided content.
when_to_use: Use this skill when the user asks to create, export, save, download, or produce a text-based file such as TXT, Markdown, HTML, JSON, or CSV.
provider_type: builtin
provider_id: file_generator
runtime_type: tool
tools:
  - generate_file
max_calls_per_turn: 3
timeout_seconds: 5
display:
  icon: file-plus
  category: productivity
  label:
    en_US: File Generator
    zh_Hans: 文件生成器
  description:
    en_US: Creates downloadable TXT, Markdown, HTML, JSON, and CSV files.
    zh_Hans: 创建可下载的 TXT、Markdown、HTML、JSON 和 CSV 文件。
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

Use this skill when the user wants content delivered as a generated file.

## Workflow

1. Decide the target file format from the user's request.
2. If the user did not specify a format, choose the most appropriate supported format:
   - Use `md` for reports, notes, plans, structured prose, and documentation drafts.
   - Use `txt` for plain text without formatting.
   - Use `html` for simple previewable web documents.
   - Use `json` for machine-readable structured data.
   - Use `csv` for table-like data.
3. Call `call_skill_tool` with `tool_name` set to `generate_file`.
4. Use `persistent` lifecycle by default unless the user asks for a temporary file.
5. In the final answer, briefly mention the generated filename and format.
6. Do not invent, rewrite, shorten, or manually format download links. The system UI displays generated file download controls from structured artifact events.

## Constraints

- Do not use this skill for PDF generation. PDF is not supported in this version.
- For HTML output, provide the visible text content. The tool creates a safe HTML document and escapes user-provided content.
- For JSON output, provide valid JSON content.
- For CSV output, provide valid CSV content.
- Do not include filesystem paths in filenames.
- Do not claim a file was generated unless the `generate_file` tool call succeeded.

## Tool Usage

`generate_file` accepts:

- `content`: text content to write into the file.
- `format`: `txt`, `md`, `html`, `json`, or `csv`.
- `filename`: optional display filename. The extension is added automatically.
- `title`: optional title used by generated HTML files.
- `lifecycle`: optional file lifecycle, `persistent` or `temporary`. Defaults to `persistent`.
