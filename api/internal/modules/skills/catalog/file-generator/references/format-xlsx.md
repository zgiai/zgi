# XLSX Reference

Use this reference when `format` is `xlsx` or `excel`.

## Tool Parameters

- `content`: Valid CSV text.
- `format`: Use `xlsx`.
- `filename`: Optional display filename without path separators. The `.xlsx` extension is added automatically.
- `title`: Optional visible workbook title. When provided, it is rendered as a merged title row above the table.
- `lifecycle`: Optional `persistent` or `temporary`. Defaults to `persistent`.

## Content Rules

- Provide valid CSV content. The tool uses CSV as the input representation for the workbook.
- The CSV must include at least one row.
- CSV rows become spreadsheet rows. CSV fields become string cells.
- Use XLSX when the user asks for Excel or a spreadsheet workbook.
- The first CSV row is treated as the table header row. When `title` is provided, the title uses row 1 and the CSV header row starts on row 2.
- The generated workbook applies default table styling:
  - Optional merged title row with bold 16pt text.
  - Header row with light blue background, bold 12pt text, centered alignment, and thin borders.
  - Content cells with 11pt text, thin borders, top alignment, and wrapping.
  - A clear outer border around the generated title/table area.
  - Automatic column widths and an auto filter over the generated table range.
- Do not promise formulas, charts, multiple sheets, custom colors, custom fonts, custom styles, merged cells, manual column widths, or cell types beyond strings.
