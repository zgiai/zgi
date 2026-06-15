# Bar Chart Reference

Use this reference when `chart_type` is `bar`, `column`, category comparison, subject score comparison, or grouped comparison.

## Payload Shape

```json
{
  "chart_type": "bar",
  "title": "Subject Score Comparison",
  "output_filename": "subject-score-bar",
  "data": {
    "categories": ["Chinese", "Math", "English"],
    "max_value": 100,
    "series": [
      {
        "name": "Student",
        "values": [88, 92, 85],
        "color": "#2563eb"
      },
      {
        "name": "Class Average",
        "values": [78, 80, 79],
        "color": "#94a3b8"
      }
    ]
  },
  "options": {
    "style": "comparison",
    "width": 900,
    "height": 620,
    "show_values": true,
    "legend": true,
    "grid": true
  }
}
```

## Data Rules

- `categories`: required list of category labels. It must contain at least 1 item.
- `data.series`: required list of 1 to 8 data series.
- Each series requires:
  - `name`: series label.
  - `values`: numeric values with the same length as `categories`.
  - `color`: optional hex color.
- `max_value`: optional positive number. If omitted, the renderer chooses an automatic maximum.
- Values must be greater than or equal to 0.
- If `max_value` is provided, every value must be less than or equal to `max_value`.

## Style Rules

- Use `comparison` style when the chart compares a focus series against a baseline.
- Use `business` style for reports and operational dashboards.
- Use `teaching` style for classroom or learning material.
- Use `simple` style when the user does not specify a look.

## Clarification Rules

Ask for clarification before generating the chart when:

- The chart type is not explicit and both bar and line could fit.
- Category labels are missing.
- A series name is missing and the meaning is not obvious.
- The number of values differs from the number of categories.
- A value is missing, negative, non-numeric, or exceeds the provided `max_value`.
- The user asks for a polished chart but does not specify title or style.
