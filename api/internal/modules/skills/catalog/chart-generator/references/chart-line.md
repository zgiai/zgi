# Line Chart Reference

Use this reference when `chart_type` is `line`, trend chart, time series, score trend, progress over attempts, or change over stages.

## Payload Shape

```json
{
  "chart_type": "line",
  "title": "Score Trend",
  "output_filename": "score-trend-line",
  "data": {
    "x_axis": ["First", "Second", "Third"],
    "max_value": 100,
    "series": [
      {
        "name": "Math",
        "values": [78, 85, 92],
        "color": "#2563eb"
      },
      {
        "name": "English",
        "values": [82, 84, 88],
        "color": "#16a34a"
      }
    ]
  },
  "options": {
    "style": "teaching",
    "width": 900,
    "height": 620,
    "show_values": true,
    "legend": true,
    "grid": true
  }
}
```

## Data Rules

- `x_axis`: required list of x-axis labels. It must contain at least 1 item.
- `categories` is accepted as an alias for `x_axis` when the user provides category-like labels.
- `data.series`: required list of 1 to 8 data series.
- Each series requires:
  - `name`: line label.
  - `values`: numeric values with the same length as `x_axis`.
  - `color`: optional hex color.
- `max_value`: optional positive number. If omitted, the renderer chooses an automatic maximum.
- Values must be greater than or equal to 0.
- If `max_value` is provided, every value must be less than or equal to `max_value`.

## Style Rules

- Use `line` for changes over time, attempts, stages, or ordered categories.
- Use `bar` instead when the x-axis categories are unordered and the goal is direct category comparison.
- Use `teaching` style for score progress and learning material.
- Use `business` style for operational metrics or reports.

## Clarification Rules

Ask for clarification before generating the chart when:

- The user provides values but no ordered x-axis labels.
- The order of x-axis labels is ambiguous.
- A series name is missing and the meaning is not obvious.
- The number of values differs from the number of x-axis labels.
- A value is missing, negative, non-numeric, or exceeds the provided `max_value`.
- The user asks for a polished chart but does not specify title or style.
