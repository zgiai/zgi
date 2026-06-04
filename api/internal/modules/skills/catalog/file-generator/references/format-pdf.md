# PDF Reference

Use this reference when `format` is `pdf`.

## Tool Parameters

- `content`: Plain text lines for the PDF body.
- `format`: Use `pdf`.
- `filename`: Optional display filename without path separators. The `.pdf` extension is added automatically.
- `title`: Optional PDF title heading. Use a short meaningful title when available.
- `lifecycle`: Optional `persistent` or `temporary`. Defaults to `persistent`.

## Content Rules

- Provide plain text lines.
- Use PDF when the user asks for a simple read-only distribution file.
- The generated PDF is a simple text PDF.
- If the user asks for richer layout, print CSS, tables, colors, page margins, page breaks, or business-report formatting, use `generate_pdf` with `format-pdf-html.md` instead.
- Do not promise complex layout, rich text, tables, images, custom fonts, exact pagination, page headers, footers, or design fidelity.
