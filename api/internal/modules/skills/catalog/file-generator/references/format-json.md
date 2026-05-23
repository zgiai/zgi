# JSON Reference

Use this reference when `format` is `json`.

## Tool Parameters

- `content`: Valid JSON text.
- `format`: Use `json`.
- `filename`: Optional display filename without path separators. The `.json` extension is added automatically.
- `title`: Ignored for JSON.
- `lifecycle`: Optional `persistent` or `temporary`. Defaults to `persistent`.

## Content Rules

- Provide syntactically valid JSON.
- Use JSON for machine-readable structured data, configuration-like output, exports, and API-style payloads.
- The tool parses the JSON and rewrites it with two-space indentation.
- Do not include comments, trailing commas, or non-JSON syntax.
