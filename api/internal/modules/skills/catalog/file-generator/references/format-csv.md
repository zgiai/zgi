# CSV Reference

Use this reference when `format` is `csv`.

## Tool Parameters

- `content`: Valid CSV text.
- `format`: Use `csv`.
- `filename`: Optional display filename without path separators. The `.csv` extension is added automatically.
- `title`: Ignored for CSV.
- `lifecycle`: Optional `persistent` or `temporary`. Defaults to `persistent`.

## Content Rules

- Provide valid CSV content with rows separated by newlines.
- Quote fields when they contain commas, quotes, or newlines.
- Use CSV for simple table-like data, spreadsheet imports, and machine-readable tabular exports.
- The tool validates CSV and saves the provided CSV text.
