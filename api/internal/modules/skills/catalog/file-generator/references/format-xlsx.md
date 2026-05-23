# XLSX Reference

Use this reference when `format` is `xlsx` or `excel`.

## Tool Parameters

- `content`: Valid CSV text.
- `format`: Use `xlsx`.
- `filename`: Optional display filename without path separators. The `.xlsx` extension is added automatically.
- `title`: Ignored for XLSX.
- `lifecycle`: Optional `persistent` or `temporary`. Defaults to `persistent`.

## Content Rules

- Provide valid CSV content. The tool uses CSV as the input representation for the workbook.
- The CSV must include at least one row.
- CSV rows become spreadsheet rows. CSV fields become string cells.
- Use XLSX when the user asks for Excel or a spreadsheet workbook.
- Do not promise formulas, charts, multiple sheets, formatting, merged cells, column widths, filters, or cell types beyond strings.
