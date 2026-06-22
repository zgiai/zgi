# PPT Quality Check

Run this check before preparing the payload for the `file-generator` skill.

## Deck-Level Checks

- The deck has a cover slide.
- Decks with 5 or more slides have an agenda or contents slide.
- The final slide gives a summary, conclusion, recommendation, or next steps.
- Slide order follows the user's requested story.
- Font, title hierarchy, colors, and spacing are consistent.
- User-provided brand rules are applied, or neutral defaults are used with a stated assumption.
- No unsupported features are promised.
- The final `presentation` argument is strict JSON, not Markdown, prose, JSON5, or a partial object.

## Slide-Level Checks

For every slide:

- Title is present unless it is a deliberate cover or section divider design.
- Page objective is represented by visible content.
- Elements use exact `x`, `y`, `w`, and `h` when layout matters.
- Readable text and tables stay within slide bounds.
- Title, text, and table elements do not overlap.
- Body text is not too dense.
- Placeholder areas are clear and labeled.
- Tables fit in the assigned box; split long tables across slides.
- Every title/text element has enough height for its font size and expected wrapped line count.
- Every title/text element has safety headroom; do not size text boxes to the exact estimated height.
- No single body text element carries a full section, long paragraph, or source-summary dump.

## JSON Payload Checks

Before handing off to `file-generator`:

- `presentation` starts with `{` and ends with `}`.
- No Markdown code fence is included.
- No explanatory prose appears before, after, or inside the JSON.
- No comments, trailing commas, single-quoted strings, or unquoted Chinese/English text appear in the JSON.
- Any intended line break inside visible text is encoded as `\n`.
- Any double quote inside visible text is escaped as `\"` or replaced with Chinese quotes.
- Braces and brackets are balanced outside quoted strings.
- The `slides` array is closed and the final non-space character is `}`.
- The serialized `presentation` is preferably under 10,000 characters for normal decks.
- The deck uses no more than 8 slides unless the user explicitly requires more.

## Text Density Checks

- Cover title: short enough to fit one or two lines.
- Cover title box: use `h >= 1.6` when the title may wrap to two lines.
- Normal slide title: preferably under 28 Chinese characters or 80 English characters.
- Normal slide title box: use `h >= 1.15` when the title may wrap to two lines.
- Subtitle or short label text at 16-24 pt: use `h >= 1.15` unless it is guaranteed to be one short line with estimated height under `0.65`.
- Caption or metadata text at 11-14 pt: use `h >= 0.55` for one short line and `h >= 0.85` for two lines.
- Do not create any visible `text` element with `h < 0.55`.
- Body text box: hard limit 3 to 5 bullets.
- Body text box: hard limit 10 estimated wrapped lines. If estimated wrapped lines exceed 10, split into another slide before handoff.
- Body text box: hard limit 4.0 inches estimated height for normal content. If a body text estimate exceeds 4.0 inches, split the content unless the user explicitly requested a dense appendix slide.
- Body text box: absolute hard stop 4.5 inches estimated height. Never hand off a single `text` element above this estimate; split it into continuation slides.
- Local text box: for two-column, comparison-card, visual-placeholder, process-step, sidebar, or any body box with `w < 7.0` or `h < 4.2`, keep estimated height at or below `min(2.8, h * 0.70)` inches.
- Local text box: if the estimate is above `min(2.8, h * 0.70)`, split content, move details to a continuation slide, or switch to a full-width layout before handoff.
- Narrow column text box: for `w <= 5.8`, keep to 3 bullets or about 90 Chinese characters / 220 English characters per text element.
- Small card text box: for `h <= 3.5`, keep estimated height at or below `h * 0.65`; do not fix overflow by increasing `h` in small cards.
- Full-width `title_body` body box around `11.5 x 5.3` inches at 18 pt: keep to 8 to 10 concise wrapped lines, not dense paragraphs.
- Medium Chinese body box around `5.6 x 3.2` inches: keep under about 120 Chinese characters.
- Wide Chinese body box around `7.5 x 3.5` inches: keep under about 180 Chinese characters.
- If a body element contains more than about 280 Chinese characters or 650 English characters, split it regardless of box height.
- If a body element has paragraph-style text instead of bullets, convert it into bullets or split it.
- Split slides before reducing body text below 16 pt.

## Text Fit Estimation

Use this conservative check for every `title` and `text` element before preparing the handoff payload:

- Match the file-generator formula: line capacity is `(w - 2 * margin) / (font_size / 72)`, with usable width floored to at least `0.2` inches.
- Count CJK characters as 1 unit, ASCII letters/numbers as 0.55 unit, spaces as 0.35 unit.
- Estimate wrapped lines as `ceil(text_units / line_capacity)` for each paragraph, then sum paragraphs.
- Estimate height as `wrapped_lines * (font_size / 72) * 1.2 * line_spacing + 2 * margin + 0.08`.
- File-generator accepts only `estimated_height <= h + max(0.08, h * 0.08)`. Treat that as a renderer tolerance, not as usable design space.
- Required planning height is `estimated_height * 1.25`, rounded up to the next `0.1` inch.
- If `required_height` is larger than available slide space, split content; do not pass the smaller box and hope file-generator tolerance accepts it.
- If estimated height is greater than `0.75 * h`, split content before handoff. Do not rely on exact tolerance.
- If estimated height is within `0.2` inches of `h`, increase `h` by at least `0.3` inches before handoff.
- If `w < 7.0` or `h < 4.2`, use the local-box limit first: estimated height must be at or below `min(2.8, h * 0.70)`.
- If `h <= 3.5`, use the small-card limit first: estimated height must be at or below `h * 0.65`.
- For a `title_body` slide with `h=5.3`, any estimate above 4.0 inches should be split; any estimate above 4.5 inches must be split because there is no useful room to increase the box.
- For a body box at `h=5.0`, do not hand off estimates above 4.0 inches. A near-full body text box is already too dense for a presentation slide even if file-generator might accept it.
- For the concrete failure pattern `estimated height around 4.1` in a `h=3.0` to `h=3.5` local text box, do not increase the same box and retry. Split the text or convert the slide to full-width content.

## Layout Checks

- Wide slide size is `13.333 x 7.5`.
- Normal readable content should use `x >= 0.6`, `y >= 0.35`, and leave at least `0.45` inches bottom margin.
- Do not use negative coordinates for readable text or tables.
- Use negative coordinates only for decorative shapes intentionally bleeding off an edge.
- For two columns, use stable non-overlapping tracks such as `x=0.85,w=5.55` and `x=6.95,w=5.55`.

## Failure Handling

If a slide fails the check:

- Split dense content into another slide.
- Increase `h` before handoff when the text is appropriate but the box is too short.
- If downstream file generation reports estimated height only slightly above box height, still increase the box by at least 25% or 0.3 inches; do not retry with a near-equal height.
- If the failed element is a large body box such as `h=5.0` or `h=5.3`, split the body into two slides; do not increase `h`.
- If the failed element is a local text box such as a two-column body, comparison card, visual-placeholder summary, or sidebar, split the text or change layout; do not only increase `h` inside the same local box.
- When downstream file generation reports `slides[i].elements[j].text does not fit`, locate that exact slide and element, halve its text, move the remainder to a continuation slide, and prepare a new payload.
- Repair invalid JSON before retrying; do not send the same `presentation` string again.
- For `unexpected EOF`, rebuild a complete shorter payload from the slide plan; do not only append missing closing characters.
- Reduce the number of elements.
- Convert paragraphs into concise bullets.
- Move placeholders to a dedicated visual slide.
- Use a table only when the data is compact enough to fit.
