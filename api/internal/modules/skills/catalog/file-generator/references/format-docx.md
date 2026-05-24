# DOCX Reference

Use this reference when `format` is `docx` or `word`.

## Tool Parameters

- `content`: Plain text lines for the Word document body.
- `format`: Use `docx`.
- `filename`: Optional display filename without path separators. The `.docx` extension is added automatically.
- `title`: Ignored for DOCX.
- `lifecycle`: Optional `persistent` or `temporary`. Defaults to `persistent`.

## Content Rules

- Provide plain text lines.
- Each line becomes a simple Word paragraph.
- Use DOCX when the user asks for Word, an editable document, or a formal document file.
- Do not promise Markdown rendering, custom styles, tables, images, headers, footers, sections, or page layout controls.
