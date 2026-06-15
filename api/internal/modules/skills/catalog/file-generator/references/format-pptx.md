# PPTX Reference

Use `generate_pptx` for editable static PowerPoint presentations.

This path is for new decks only. It does not edit an existing PPTX template.

## Supported

- Wide or 4:3 slide layout.
- Multiple slides.
- Editable text boxes and title text.
- Basic tables.
- Simple rectangle shapes.
- Automatic safe fallback layout for elements that omit position fields.
- Font face, font size, font weight, bold, italic, underline, color, horizontal alignment, vertical alignment, line spacing, and text box margins.
- Slide background color.
- Table column widths, border color, header fill/color, and row fill.
- Shape fill, line color, rotation, and transparency.

## Not Supported

- Animations, transitions, speaker notes, comments, slide masters, theme inheritance, charts, media, embedded videos, and editing existing PPTX files.
- HTML-to-PPTX conversion.
- External images or remote assets.

## Presentation JSON

Pass a JSON string as `presentation`.

Top-level fields:

- `layout`: optional, `wide` or `4:3`. Defaults to `wide`.
- `language`: optional BCP 47 language tag for the generated presentation. Defaults to `en-US`.
- `default_style`: optional text defaults.
- `slides`: required array.

Slide fields:

- `background_color`: optional `RRGGBB` color.
- `elements`: required array.

Element fields:

- `type`: `title`, `text`, `table`, or `shape`.
- `x`, `y`, `w`, `h`: optional position and size in inches.
- `style`: optional text style.
- `margin`: optional text/table margin in inches.
- `break_line`: optional boolean for text boxes.
- `line_spacing`: optional line spacing multiplier for text boxes.

Text style fields:

- `font_face` or `font_family`
- `font_size`
- `font_weight`: `normal`, `regular`, `medium`, `semibold`, `bold`, `bolder`, or `400`-`900`. Bold-like values are treated as `bold: true`.
- `color`
- `bold`
- `italic`
- `underline`
- `align`: `left`, `center`, `right`, or `justify`.
- `valign`: `top`, `mid`, or `bottom`.
- `line_spacing`: `0.5` to `3`.
- `margin`: optional margin in inches.
- `break_line`: optional boolean.

Table fields:

- `headers`: optional array of strings.
- `rows`: optional array of string arrays.
- `column_widths`: optional array of column widths in inches.
- `border_color`: optional `RRGGBB` color.
- `header_fill_color`: optional `RRGGBB` color.
- `header_color`: optional `RRGGBB` color.
- `row_fill_color`: optional `RRGGBB` color.

Shape fields:

- `fill_color`: optional `RRGGBB` color.
- `line_color`: optional `RRGGBB` color.
- `rotation`: optional degrees.
- `transparency`: optional `0` to `100`.

## Layout Rules

- Wide slides are `13.333 x 7.5` inches. 4:3 slides are `10 x 7.5` inches.
- Keep normal content inside the slide bounds. Recommended content margins are `x >= 0.6`, `y >= 0.35`, and at least `0.45` inches above the bottom edge.
- Do not overlap `title`, `text`, or `table` elements. Move content down, use columns, or split dense content into another slide.
- `x` and `y` may be slightly negative only for decorative `shape` elements that should bleed off an edge. Do not use negative coordinates for readable text or tables.
- Prefer explicit `x`, `y`, `w`, and `h` for important content. If positions are omitted, the renderer applies a simple top-to-bottom fallback layout.
- For Chinese text, use `18` to `22` point body text and `1.1` to `1.3` line spacing. Keep each text box concise.
- Avoid long unbroken Chinese paragraphs. As a rough budget, a `5.6 x 3.2` inch body box at `16` to `18` pt should stay under about `120` Chinese characters, and a `7.5 x 3.5` inch body box at `15` to `17` pt should stay under about `180` Chinese characters.
- If the user asks for more text, add more slides or split into multiple short bullets. Do not pack long narrative paragraphs into a single text box.
- If a slide needs more than one title, three body blocks, or one medium table, split it into multiple slides instead of shrinking text.
- For two-column slides, use non-overlapping boxes such as left `x=0.75,w=5.6` and right `x=6.95,w=5.6`.

## Example

```json
{
  "layout": "wide",
  "language": "en-US",
  "default_style": {
    "font_face": "Arial",
    "font_size": 18,
    "color": "111827"
  },
  "slides": [
    {
      "background_color": "FFFFFF",
      "elements": [
        {
          "type": "title",
          "text": "Monthly Operations Review",
          "x": 0.7,
          "y": 0.35,
          "w": 11.9,
          "h": 0.8,
          "style": {
            "font_size": 30,
            "font_weight": "700",
            "align": "center"
          }
        },
        {
          "type": "table",
          "x": 1,
          "y": 1.5,
          "w": 11,
          "h": 3.5,
          "headers": ["Metric", "Value"],
          "rows": [
            ["Water usage", "958.57"],
            ["Electricity usage", "7229"]
          ],
          "column_widths": [4, 3],
          "header_fill_color": "F3F4F6",
          "border_color": "D1D5DB",
          "style": {
            "font_size": 14
          }
        }
      ]
    }
  ]
}
```

Call `generate_pptx` with the JSON string, a concise filename, and the default persistent lifecycle unless the user asks otherwise.
