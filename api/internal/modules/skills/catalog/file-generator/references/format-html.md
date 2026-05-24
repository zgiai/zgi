# HTML Reference

Use this reference when `format` is `html` or `htm`.

## Tool Parameters

- `content`: Runnable HTML source. Prefer a complete HTML document.
- `format`: Use `html`.
- `filename`: Optional display filename without path separators. The `.html` extension is added automatically.
- `title`: Optional document title used only when the tool wraps an HTML fragment.
- `lifecycle`: Optional `persistent` or `temporary`. Defaults to `persistent`.

## Content Rules

- Provide complete runnable HTML when the user asks for an HTML file, web page, or browser-openable artifact.
- Include `<!doctype html>`, `<html>`, `<head>`, and `<body>` when producing a full page.
- Inline CSS and JavaScript are preserved and can run when the downloaded file is opened in a browser.
- If `content` is only an HTML fragment, the tool wraps it in a basic HTML document and uses `title` as the document title.
- Do not use HTML for plain text exports. Use `txt` or `md` unless the user wants a browser-openable HTML file.
