# Pie Chart Reference

Use this reference when `chart_type` is `pie`, proportion, composition, or share of a whole.

## Payload Shape

```json
{
  "chart_type": "pie",
  "title": "Score Band Share",
  "output_filename": "score-band-pie",
  "data": {
    "items": [
      { "label": "90-100", "value": 2, "color": "#2563eb" },
      { "label": "80-89", "value": 3, "color": "#16a34a" },
      { "label": "70-79", "value": 2, "color": "#f97316" },
      { "label": "0-69", "value": 1, "color": "#dc2626" }
    ]
  },
  "options": {
    "style": "teaching",
    "show_values": true,
    "legend": true
  }
}
```

## Data Rules

- `items`: required list of 1 to 50 slices.
- Each item requires:
  - `label`: slice label.
  - `value`: numeric slice value.
  - `color`: optional hex color.
- Values must be greater than or equal to 0.
- The total item value must be greater than 0.
- Use pie charts for part-to-whole relationships, not ordered trends.

## Clarification Rules

Ask for clarification before generating the chart when:

- The user asks for a generic chart and does not explicitly request a pie chart.
- The values are raw scores but the intended slices or bands are not specified.
- A slice label or value is missing.
- The user asks for a polished chart but does not specify title or style.
