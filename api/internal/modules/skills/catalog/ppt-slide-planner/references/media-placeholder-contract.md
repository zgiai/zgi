# Media Placeholder Contract

The current PPTX renderer supports editable text, tables, and simple shapes. It does not embed real images, videos, charts, or external media.

## Placeholder Rules

When media is needed:

- Create a visible `shape` element for the reserved area.
- Add a `text` element inside or near the shape with a clear placeholder label.
- Describe what should be placed there, such as "Product screenshot", "Architecture diagram", "Market size chart", or "Demo video placeholder".
- Do not claim the real media is embedded.
- Keep placeholder labels short.

## Placeholder Types

- `image`: screenshots, product UI, team photo, scenario visual.
- `chart`: bar chart, line chart, KPI trend, distribution, market size.
- `diagram`: architecture, process, workflow, data flow.
- `video`: demo clip, customer quote clip, training video.

## PPTX Element Pattern

Use one shape plus one label:

```json
[
  {
    "type": "shape",
    "x": 6.4,
    "y": 1.35,
    "w": 6.1,
    "h": 5.2,
    "fill_color": "F3F4F6",
    "line_color": "CBD5E1"
  },
  {
    "type": "text",
    "text": "产品界面截图占位",
    "x": 6.7,
    "y": 3.55,
    "w": 5.5,
    "h": 0.45,
    "style": {
      "font_size": 16,
      "color": "64748B",
      "align": "center"
    }
  }
]
```

## Quality Rules

- Placeholder shape and label must not overlap unrelated text.
- Placeholder must have enough area to be useful.
- Use consistent placeholder styling across the deck.
- If the user requires actual images or video, state that this generator reserves placeholders only.
