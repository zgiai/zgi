# TXT Reference

Use this reference when `format` is `txt` or `text`.

## Tool Parameters

- `content`: Plain text to write into the file.
- `format`: Use `txt`.
- `filename`: Optional display filename without path separators. The `.txt` extension is added automatically.
- `title`: Ignored for TXT.
- `lifecycle`: Optional `persistent` or `temporary`. Defaults to `persistent`.

## Content Rules

- Preserve the user's text as plain text.
- Do not add Markdown, HTML, or other markup unless the user explicitly wants those characters in the text file.
- Use TXT for unformatted notes, logs, lists, transcripts, or simple exports.
