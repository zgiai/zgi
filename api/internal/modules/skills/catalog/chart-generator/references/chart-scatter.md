# Scatter Chart Reference

Use this reference when `chart_type` is `scatter`, scatter plot, or two-variable relationship.

## Payload Shape

```json
{
  "chart_type": "scatter",
  "title": "Rank and Score Relationship",
  "output_filename": "rank-score-scatter",
  "data": {
    "x_label": "Rank",
    "y_label": "Score",
    "x_min": 1,
    "x_max": 8,
    "y_min": 0,
    "y_max": 100,
    "points": [
      { "x": 1, "y": 98, "label": "Zhang San" },
      { "x": 2, "y": 98, "label": "Sun Ba" },
      { "x": 3, "y": 88, "label": "Li Si" }
    ]
  },
  "options": {
    "style": "teaching",
    "grid": true,
    "show_labels": true
  }
}
```

## Data Rules

- `points`: required list of 1 to 500 points.
- Each point requires:
  - `x`: numeric x-axis value.
  - `y`: numeric y-axis value.
  - `label`: optional point label.
  - `color`: optional hex color.
- `x_label` and `y_label` are optional axis labels.
- `x_min`, `x_max`, `y_min`, and `y_max` are optional numeric bounds.
- If explicit bounds are provided, max must be greater than min.

## Clarification Rules

Ask for clarification before generating the chart when:

- The user asks for a generic chart and does not explicitly request a scatter chart.
- The mapping of x and y values is unclear.
- A point is missing `x` or `y`.
- Axis labels or bounds are important but not specified.
- The user asks for a polished chart but does not specify title or style.
