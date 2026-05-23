# HTML Reference

Use this reference when `format` is `html` or `htm`.

## Tool Parameters

- `content`: Visible text content for the generated page body.
- `format`: Use `html`.
- `filename`: Optional display filename without path separators. The `.html` extension is added automatically.
- `title`: Optional HTML document title. Use a short meaningful title when available.
- `lifecycle`: Optional `persistent` or `temporary`. Defaults to `persistent`.

## Content Rules

- Provide the visible text content, not raw HTML markup.
- The tool escapes user-provided content into a safe HTML document.
- Newlines in `content` become line breaks in the generated HTML.
- Do not promise custom HTML structure, CSS, scripts, embedded images, or raw HTML preservation.
