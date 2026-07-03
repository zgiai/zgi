# Slide Layout Contract

Use concrete layouts with explicit coordinates. Wide slides are `13.333 x 7.5` inches.

## Common Style Tokens

Convert these tokens to explicit PPTX styles in the payload:

- `cover_title`: 36-44 pt, bold, centered or left aligned.
- `cover_subtitle`: 18-24 pt, secondary color.
- `page_title`: 26-32 pt, bold.
- `section_title`: 34-40 pt, bold.
- `body`: 16-20 pt, normal.
- `body_card`: 16-18 pt, normal, dark text on light shape/card.
- `caption`: 11-13 pt, secondary color.
- `table_text`: 11-14 pt.

## Layouts

### `cover_center`

- Title: `x=1.0,y=2.05,w=11.3,h=1.7`. Use this safe height for Chinese or English titles that may wrap to two lines.
- Subtitle: `x=1.4,y=3.75,w=10.5,h=1.15`.
- Optional date/author: `x=1.4,y=5.75,w=10.5,h=0.65`.

### `agenda_list`

- Title: `x=0.7,y=0.35,w=11.9,h=1.0`.
- Agenda text: `x=1.2,y=1.45,w=10.8,h=4.8`.
- Use numbered agenda items. Keep under 8 items.

### `title_body`

- Title: `x=0.7,y=0.35,w=11.9,h=1.0`.
- Body: `x=0.9,y=1.35,w=11.5,h=5.3`.
- Use 3 to 5 concise bullets.
- Hard limit: the body must not exceed 10 estimated wrapped lines at 18 pt. If it does, create a continuation slide instead of increasing `h`.
- Hard limit: the body estimated height should stay at or below 4.0 inches. If the estimate is above 4.0 inches, split the content even though the box is 5.3 inches tall.

### `two_column`

- Title: `x=0.7,y=0.35,w=11.9,h=1.0`.
- Left body: `x=0.85,y=1.35,w=5.55,h=3.5` for normal column text. Use a lower visual area or a continuation slide for overflow.
- Right body: `x=6.95,y=1.35,w=5.55,h=3.5` for normal column text. Use a lower visual area or a continuation slide for overflow.
- Use when two themes need equal weight.
- Each column should stay at or below 2.8 inches estimated text height and no more than `0.70 * h`.
- Each column should use 3 bullets or about 90 Chinese characters / 220 English characters. Split into another slide when a column needs more.

### `two_column_comparison`

- Title: `x=0.7,y=0.35,w=11.9,h=1.0`.
- Left card shape: `x=0.75,y=1.25,w=5.75,h=4.9`.
- Right card shape: `x=6.85,y=1.25,w=5.75,h=4.9`.
- Left text: `x=1.0,y=1.55,w=5.25,h=4.3`.
- Right text: `x=7.1,y=1.55,w=5.25,h=4.3`.
- Each comparison text box should stay at or below 2.8 inches estimated text height. Use short bullets, not paragraphs.

### `process_steps`

- Title: `x=0.7,y=0.35,w=11.9,h=1.0`.
- Step shapes: evenly distribute 3 to 5 rectangles from `x=0.8` to `x=12.2`, `y=2.0,h=1.4`.
- Step labels: centered inside each shape.
- Use small connector shapes or text arrows only when they do not overlap.

### `table_focus`

- Title: `x=0.7,y=0.35,w=11.9,h=1.0`.
- Table: `x=0.85,y=1.35,w=11.6,h=5.4`.
- Keep tables short. Split rows across slides if needed.

### `visual_placeholder`

- Title: `x=0.7,y=0.35,w=11.9,h=1.0`.
- Text summary: `x=0.85,y=1.35,w=5.2,h=5.2`.
- Placeholder shape: `x=6.4,y=1.35,w=6.1,h=5.2`.
- Placeholder label centered inside the shape.
- Text summary should stay at or below 3.6 inches estimated text height so the visual placeholder slide remains readable.
- If the text summary estimate exceeds 3.0 inches, split the summary or move details to a dedicated content slide.

### `section_divider`

- Section title: `x=1.0,y=2.45,w=11.3,h=1.4`.
- Optional subtitle: `x=1.2,y=3.95,w=10.9,h=0.9`.

### `conclusion_next_steps`

- Title: `x=0.7,y=0.35,w=11.9,h=1.0`.
- Conclusion: `x=0.95,y=1.35,w=11.2,h=2.1`.
- Next steps table or bullets: `x=0.95,y=3.8,w=11.2,h=2.4`.

## Text Box Height Rules

Use these minimum heights before converting to PPTX:

- Cover title 36-44 pt: `h >= 0.9` for one short line; `h >= 1.6` for two lines or any title over 18 Chinese characters / 48 English characters.
- Cover subtitle 18-24 pt: `h >= 1.15` unless the text is guaranteed to be one short line with estimate under `0.65`.
- Normal page title 26-32 pt: `h >= 1.0` for one line; `h >= 1.3` for two lines.
- Short labels or metadata at 16-24 pt: `h >= 1.15` unless they are one short line and estimate under `0.65`.
- Captions at 11-14 pt: `h >= 0.55` for one line; `h >= 0.85` for two lines.
- Body text 16-20 pt: reserve about `0.42` inches per bullet line plus margin. A 4-bullet body box should usually be `h >= 2.1`.
- Body text 16-20 pt: if estimated text height is greater than `0.75 * h`, split the content before handoff.
- Body text 16-20 pt: if estimated text height is greater than 4.0 inches, split the content before handoff, even if the box is 5.0 or 5.3 inches tall.
- Body text 16-20 pt: never hand off one `text` element with estimated height above 4.5 inches.
- Local text boxes in two-column, comparison-card, visual-placeholder, process-step, sidebar, or any body box with `w < 7.0` or `h < 4.2`: estimated height must be at or below `min(2.8, h * 0.70)`.
- Small cards with `h <= 3.5`: estimated height must be at or below `h * 0.65`.
- If local text exceeds those limits, split content or switch to a full-width layout; do not keep increasing `h` inside the same local layout.
- All title/text boxes: set `h >= estimated_height * 1.25`, rounded up to the next `0.1` inch.
- All title/text boxes: if estimated height is within `0.2` inches of `h`, increase `h` by at least `0.3`.
- Full-height body boxes such as `h=5.3` cannot be made taller on a standard slide. If they overflow, split the slide.
- Table boxes must include header height and every row height; split tables when the calculated height is close to the box height.

If a title or text block needs more height than the selected layout provides, increase the box height and move lower elements down, or split the content into another slide.

## Layout Selection Rules

- Use `cover_center` for the first slide.
- Use `agenda_list` for decks with 5 or more slides.
- Use `section_divider` only when the deck has clear chapters.
- Use `table_focus` for structured metrics, timelines, roles, or comparison data.
- Use `visual_placeholder` when the slide needs an image, chart, diagram, or video placeholder.
- Use `conclusion_next_steps` for the final slide unless the user requests another ending.
