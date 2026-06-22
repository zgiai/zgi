# PPTX Payload Contract

Read this before preparing a `file-generator/generate_pptx` handoff payload.

The final `presentation` argument must be a strict JSON string accepted by `file-generator/generate_pptx`.

## Supported PPTX JSON

Top-level fields:

- `layout`: `wide` or `4:3`. Default to `wide`.
- `language`: BCP 47 language tag. Use `zh-CN` for Chinese decks, `en-US` for English decks unless the user specifies otherwise.
- `default_style`: shared font, size, color, alignment, and line spacing.
- `slides`: required array.

Slide fields:

- `background_color`: optional `RRGGBB`.
- `elements`: required array.

Element types:

- `title`: editable title text.
- `text`: editable body or label text.
- `table`: basic editable table.
- `shape`: rectangle shape for cards, backgrounds, dividers, or placeholders. A shape must include at least one of `fill_color` or `line_color`; otherwise `generate_pptx` rejects it.

## Style Defaults

For Chinese business decks:

```json
{
  "font_face": "Microsoft YaHei",
  "font_size": 18,
  "color": "111827",
  "line_spacing": 1.15
}
```

For English business decks:

```json
{
  "font_face": "Arial",
  "font_size": 18,
  "color": "111827",
  "line_spacing": 1.12
}
```

## Style Mapping

Map slide-plan style tokens into explicit `style` objects:

- `cover_title`: `font_size: 40`, `font_weight: "700"`, `align: "center"`.
- `cover_subtitle`: `font_size: 22`, `color: "475569"`, `align: "center"`.
- `page_title`: `font_size: 30`, `font_weight: "700"`, `color: "0F172A"`.
- `section_title`: `font_size: 38`, `font_weight: "700"`, `align: "center"`.
- `body`: `font_size: 18`, `color: "1F2937"`, `line_spacing: 1.15`.
- `body_card`: `font_size: 17`, `color: "1F2937"`, `line_spacing: 1.15`, `margin: 0.12`.
- `caption`: `font_size: 12`, `color: "64748B"`.
- `table_text`: `font_size: 12`, `color: "1F2937"`.

## Text Fit Requirements

`file-generator/generate_pptx` measures text before rendering and rejects any title or text whose estimated height does not fit its box. Its tolerance is only `h + max(0.08, h * 0.08)`, so planning must leave real design headroom instead of relying on renderer tolerance.

Before preparing the handoff payload:

- Cover titles at 36-44 pt must use `h >= 1.6` when they may wrap to two lines.
- One-line cover titles may use `h >= 0.9`.
- Normal titles at 26-32 pt must use `h >= 1.3` when they may wrap to two lines.
- Body text at 16-20 pt should reserve about `0.42` inches per wrapped bullet line plus margin.
- Every title/text box must reserve at least 25% height headroom over estimated rendered height.
- If estimated height is within `0.2` inches of `h`, increase `h` by at least `0.3` inches before handoff.
- If a body text estimate exceeds 4.0 inches, split the content into another slide unless the user explicitly requested a dense appendix slide.
- Never hand off a single body `text` element with estimated height above 4.5 inches.
- For body boxes with `h=5.0` or `h=5.3`, do not treat the full height as usable text capacity; keep the estimate at or below 4.0 inches and split the remainder.
- For local text boxes in two-column, comparison-card, visual-placeholder, process-step, sidebar, or any body box with `w < 7.0` or `h < 4.2`, keep estimated height at or below `min(2.8, h * 0.70)`.
- For small cards with `h <= 3.5`, keep estimated height at or below `h * 0.65`.
- For narrow columns with `w <= 5.8`, use at most 3 bullets or about 90 Chinese characters / 220 English characters per text element.
- If a local text box fails or estimates around 4.0 inches while its `h` is only 3.0 to 3.5, split it or switch to a full-width layout. Do not merely increase `h` to 3.5 and retry.
- Subtitle, short label, and metadata text at 16-24 pt should normally use `h >= 1.15`.
- Captions at 11-14 pt should use `h >= 0.55` for one short line.
- If the title is long, insert a deliberate line break and increase `h`, or shorten the title and move details into subtitle/body text.
- Never prepare a handoff payload with a title/text box shorter than the expected wrapped text height.

## JSON Serialization Requirements

The `presentation` argument must be strict JSON produced from one complete presentation object.

Before handoff, verify all of these are true:

- The string starts with `{` and ends with `}`.
- There is no Markdown fence such as ```json or ```.
- There are no comments, trailing commas, ellipses, or explanatory sentences inside the JSON.
- Every object key is double-quoted.
- Every string value is double-quoted.
- Chinese and English visible text appear only inside quoted string values.
- Literal line breaks inside visible text are encoded as `\n`, not raw multi-line string content.
- Double quotes inside visible text are escaped as `\"`, or replaced with Chinese quotes.
- Do not concatenate JSON fragments by hand. Build one object and serialize it.

If any field value is uncertain, put an empty string or concise placeholder string inside quotes; never leave unquoted notes in the JSON.

## Completeness and Size Requirements

`unexpected EOF` means the JSON was truncated or not fully closed. Prevent this before handoff.

Before handoff:

- Verify braces and brackets are balanced outside quoted strings: every `{` has `}`, every `[` has `]`.
- Verify the `slides` array is closed and the final non-space character is `}`.
- Verify the JSON contains a complete top-level object with `slides`, not a partial object ending inside `elements`, `style`, `rows`, or a text value.
- Keep the serialized `presentation` compact. Prefer under 10,000 characters for normal decks.
- For long source material, create fewer slides with concise bullets first. Do not produce a very large JSON payload in one tool call.
- Prefer no more than 8 slides per generated deck unless the user explicitly requires more.
- Prefer no more than 10 elements per slide. Merge decorative shapes and remove nonessential labels before expanding payload size.
- Keep text concise: normal body slides should use 3 to 5 bullets, not paragraphs.

If the payload is likely to exceed the size budget, reduce slide count, shorten text, simplify styles, and remove optional decorative elements before handoff.

If downstream file generation returns `unexpected EOF`, do not resend the same payload and do not just append closing braces. Rebuild the complete presentation JSON from the slide plan with fewer slides/elements and validate closure again.

## Minimal Valid Shape

```json
{
  "type": "shape",
  "x": 0.75,
  "y": 1.25,
  "w": 5.75,
  "h": 4.9,
  "fill_color": "F8FAFC",
  "line_color": "CBD5E1"
}
```

## Valid Payload Example

```json
{
  "layout": "wide",
  "language": "zh-CN",
  "default_style": {
    "font_face": "Microsoft YaHei",
    "font_size": 18,
    "color": "111827",
    "line_spacing": 1.15
  },
  "slides": [
    {
      "background_color": "FFFFFF",
      "elements": [
        {
          "type": "title",
          "text": "AI 客服平台升级方案",
          "x": 1.0,
          "y": 2.05,
          "w": 11.3,
          "h": 1.7,
          "style": {
            "font_size": 40,
            "font_weight": "700",
            "align": "center",
            "color": "0F172A"
          }
        },
        {
          "type": "text",
          "text": "面向企业级工单分流、知识检索与人工复核协同",
          "x": 1.4,
          "y": 3.75,
          "w": 10.5,
          "h": 1.15,
          "style": {
            "font_size": 22,
            "align": "center",
            "color": "475569"
          }
        }
      ]
    }
  ]
}
```

## Downstream File-Generator Call

This skill does not call tools directly. When PPTX output is required, hand this payload to the `file-generator` skill, which calls:

- `tool_name`: `generate_pptx`
- `arguments.presentation`: strict JSON string, not a Markdown code block.
- `arguments.filename`: short ASCII filename.
- `arguments.title`: deck title.
- `arguments.lifecycle`: `persistent` unless temporary output was requested.

Do not pass Markdown, HTML, raw outline text, or the slide plan object directly as `presentation`.
Do not pass a partially generated object with prose mixed into it. If the JSON cannot be validated mentally as strict JSON, repair it before handoff.
