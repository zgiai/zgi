# Doughnut Chart Reference

Use this reference when `chart_type` is `doughnut`, `donut`, ring chart, or share of a whole with a visible center total.

## Payload Shape

```json
{
  "chart_type": "doughnut",
  "title": "Score Band Share",
  "output_filename": "score-band-doughnut",
  "data": {
    "items": [
      { "label": "90-100", "value": 2 },
      { "label": "80-89", "value": 3 },
      { "label": "70-79", "value": 2 },
      { "label": "0-69", "value": 1 }
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
- Use doughnut charts when the chart should emphasize both proportions and the total.

## Clarification Rules

Ask for clarification before generating the chart when:

- The user asks for a generic chart and does not explicitly request a doughnut chart.
- The values are raw scores but the intended slices or bands are not specified.
- A slice label or value is missing.
- The user asks for a polished chart but does not specify title or style.
