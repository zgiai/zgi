# Contract Ledger Export

Use this reference when the user asks to export, save, download, or generate a contract extraction file or contract ledger.

## Export Formats

- Use `json` for system integration.
- Use `csv` for contract ledger import or spreadsheet workflows.
- Use `md` for human review.
- Use `txt` only for plain text.

## Tool Usage

Call `generate_file` only after the extraction result is complete.

Arguments:

- `content`: final JSON, CSV, Markdown, or text content.
- `format`: `json`, `csv`, `md`, or `txt`.
- `filename`: short ASCII name such as `contract-field-extraction` or `contract-ledger`.
- `title`: optional document title such as `Contract Field Extraction`.
- `lifecycle`: `persistent` unless the user asks for a temporary file.

## Safety Rules

- Do not export invented values.
- Preserve `missing`, `uncertain`, and `conflict` statuses in exported files.
- Include review warnings for low-confidence or conflicting fields.
- If the export is intended for database import, prefer CSV or JSON and keep stable field names.
