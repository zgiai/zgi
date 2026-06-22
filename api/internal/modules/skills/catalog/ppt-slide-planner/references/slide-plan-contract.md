# Slide Plan Contract

Every deck must have a strict `slide_plan` before preparing PPTX JSON for `file-generator`.

## Required Slide Fields

Each slide plan object must include:

- `page_number`: 1-based slide number.
- `slide_type`: `cover`, `agenda`, `section`, `content`, `image_text`, `process`, `table`, `comparison`, `conclusion`, or `placeholder`.
- `title`: exact visible title text.
- `objective`: what this slide must communicate.
- `layout`: concrete layout name from `slide-layout-contract.md`.
- `speaker_intent`: short internal explanation of why the slide exists.
- `elements`: exact visible elements with text, position, size, style token, and text-fit budget.
- `media_placeholders`: optional image, chart, diagram, or video placeholder definitions.
- `quality_rules`: slide-specific checks.

## Element Fields

Each visible element must include:

- `id`: stable element ID.
- `type`: `title`, `text`, `table`, or `shape`.
- `text`: exact text for title/text/placeholder label.
- `x`, `y`, `w`, `h`: position and size in inches.
- `style`: style token or explicit style object.
- `purpose`: why the element is on the slide.
- `text_fit`: for `title` and `text`, note expected line count, estimated height, and why the box height includes safety headroom.

For tables, include:

- `headers`
- `rows`
- `column_widths` when useful

For shapes, include at least one of:

- `fill_color`
- `line_color`

`shape` elements without `fill_color` and `line_color` are invalid and will fail in `generate_pptx`.

## Required Deck Structure

Unless the user explicitly requests otherwise, include:

1. Cover slide.
2. Agenda or contents slide for decks with 5 or more slides.
3. Content slides split by topic.
4. Summary, conclusion, or next steps slide.

## Content Density Rules

- One slide should communicate one main idea.
- Use 3 to 5 bullets per normal content slide.
- Hard limit normal body text to 10 estimated wrapped lines per text element.
- Hard limit normal body text to 4.0 inches estimated height per text element.
- Never plan a single body `text` element above 4.5 inches estimated height; split it into continuation slides.
- For local text boxes such as two-column bodies, comparison cards, visual-placeholder summaries, process steps, sidebars, or any body box with `w < 7.0` or `h < 4.2`, limit estimated height to `min(2.8, h * 0.70)`.
- For small cards with `h <= 3.5`, limit estimated height to `h * 0.65`.
- For narrow columns with `w <= 5.8`, use at most 3 bullets or about 90 Chinese characters / 220 English characters per text element.
- Do not put full paragraphs or full source summaries into one body text element.
- Keep Chinese body text concise. Prefer under 120 Chinese characters per medium text box.
- If a slide needs more than one title, three body blocks, or one medium table, split it.
- Do not create a slide that only repeats the title without useful content.
- Long titles should be shortened, deliberately line-broken with enough height, or moved partly into subtitle/body text.

## Text Fit Planning Rules

- For cover titles, reserve `h >= 1.6` whenever the title may wrap to two lines.
- For normal page titles, reserve `h >= 1.3` whenever the title may wrap to two lines.
- For 16-24 pt subtitles, labels, or metadata, use `h >= 1.15` unless the text is one short line with estimate under `0.65`.
- For each body text element, estimate wrapped bullet lines and reserve about `0.42` inches per line plus margin.
- For all title/text elements, use `h >= estimated_height * 1.25`, rounded up to the next `0.1` inch.
- If estimated height is within `0.2` inches of `h`, increase `h` by at least `0.3` or split/shorten the text.
- If any body text element estimates above 10 wrapped lines or above `0.75 * h`, split it into a continuation slide.
- If any body text element estimates above 4.0 inches, split it into a continuation slide unless the user explicitly requested a dense appendix slide.
- If any body text element estimates above 4.5 inches, splitting is mandatory.
- If a local text box estimates above `min(2.8, h * 0.70)`, split the content or switch to a full-width layout before preparing PPTX JSON.
- If a small card text box with `h <= 3.5` estimates above `h * 0.65`, do not increase the card height and retry; split or shorten it.
- If a slide uses `title_body` with body `h=5.3` or another near-full-height body box such as `h=5.0`, overflow must be fixed by splitting; there is no safe room to increase `h`.
- If the selected layout's default height is too small, adjust coordinates before creating the PPTX payload.
- Do not prepare a handoff payload when any planned title/text element has a known overflow risk.

## Slide Plan Example

```json
{
  "page_number": 3,
  "slide_type": "comparison",
  "title": "方案 A 与方案 B 对比",
  "objective": "帮助决策者理解两种方案在成本、交付周期、风险上的差异。",
  "layout": "two_column_comparison",
  "speaker_intent": "Use parallel cards to make the trade-off obvious.",
  "elements": [
    {
      "id": "title",
      "type": "title",
      "text": "方案 A 与方案 B 对比",
      "x": 0.7,
      "y": 0.35,
      "w": 11.9,
      "h": 1.0,
      "style": "page_title",
      "purpose": "Page title",
      "text_fit": "One short line at 30 pt; estimated height below 0.7, h=1.0 leaves headroom."
    },
    {
      "id": "left_summary",
      "type": "text",
      "text": "方案 A：上线快，成本低，但长期扩展性较弱。",
      "x": 1.0,
      "y": 1.55,
      "w": 5.25,
      "h": 1.4,
      "style": "body_card",
      "purpose": "Summarize option A",
      "text_fit": "About two wrapped lines at 17 pt; h=1.4 includes more than 25% headroom."
    },
    {
      "id": "right_summary",
      "type": "text",
      "text": "方案 B：初期投入高，但更适合长期平台化演进。",
      "x": 7.1,
      "y": 1.55,
      "w": 5.25,
      "h": 1.4,
      "style": "body_card",
      "purpose": "Summarize option B",
      "text_fit": "About two wrapped lines at 17 pt; h=1.4 includes more than 25% headroom."
    }
  ],
  "media_placeholders": [
    {
      "type": "chart",
      "purpose": "展示两种方案的成本和周期对比",
      "x": 1.0,
      "y": 4.1,
      "w": 11.3,
      "h": 2.4,
      "placeholder_text": "成本 / 周期对比图"
    }
  ],
  "quality_rules": [
    "正文不超过 120 个中文字符",
    "左右两栏宽度一致",
    "标题与正文不重叠",
    "所有文本框高度满足 text_fit 预算并保留安全余量"
  ]
}
```
