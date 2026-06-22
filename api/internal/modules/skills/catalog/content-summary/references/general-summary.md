# General Summary

## Use When

Use for general summaries, abstracts, very short summaries, concise overviews, executive summaries, and key point extraction from existing source content.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and labels only. Do not use `Summary`, `Key Points`, `Keywords`, or `TL;DR`.
- For English requests, use English headings and labels only.
- Keep proper nouns, code identifiers, file formats, APIs, and quoted source terms in their original language when translation would change meaning.

## Output Shapes

For Chinese output:

```md
## 摘要

...

## 核心要点

- ...

## 结论

...
```

For a very short Chinese summary:

```md
一句话总结：...
```

For Chinese key points only:

```md
## 核心要点

- ...
```

If the user explicitly asks for English, translate all headings and labels consistently into English.

## Data Rules

- Keep facts grounded in the source.
- Preserve important numbers, names, dates, statuses, constraints, and conclusions.
- Merge duplicates and remove low-value repetition.
- Keep source uncertainty visible. Use `未说明`, `不明确`, or `来源显示` in Chinese output.
- Do not turn opinions into facts.

## Clarification Rules

Ask before summarizing when the target source is missing, when multiple prior messages could be the source, or when the user requested a specific audience or length but did not define it.
