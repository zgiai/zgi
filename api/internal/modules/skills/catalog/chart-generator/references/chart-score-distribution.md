# Score Distribution Chart Reference

Use this reference when `chart_type` is `score_distribution`, score band distribution, grade range count, or score interval statistics.

## Payload Shape

From raw scores:

```json
{
  "chart_type": "score_distribution",
  "title": "Score Distribution",
  "output_filename": "score-distribution",
  "data": {
    "scores": [
      { "label": "Zhang San", "value": 98 },
      { "label": "Sun Ba", "value": 98 },
      { "label": "Li Si", "value": 88 }
    ],
    "bands": [
      { "label": "90-100", "min": 90, "max": 100 },
      { "label": "80-89", "min": 80, "max": 89 },
      { "label": "70-79", "min": 70, "max": 79 },
      { "label": "60-69", "min": 60, "max": 69 },
      { "label": "0-59", "min": 0, "max": 59 }
    ]
  },
  "options": {
    "style": "teaching",
    "show_values": true,
    "grid": true
  }
}
```

From precomputed counts:

```json
{
  "chart_type": "score_distribution",
  "title": "Score Distribution",
  "data": {
    "bands": [
      { "label": "90-100", "count": 2 },
      { "label": "80-89", "count": 3 },
      { "label": "70-79", "count": 2 },
      { "label": "60-69", "count": 1 }
    ]
  }
}
```

## Data Rules

- `bands`: required list of 1 to 30 score bands.
- Bands must use one consistent shape:
  - All bands provide `count`, or
  - All bands provide `min` and `max`, plus `scores` is provided.
- `scores`: required only when bands use `min` and `max`.
- `scores` can be an array of numbers or objects with `value`.
- Band `min` and `max` are inclusive.
- Band `count` must be greater than or equal to 0.

## Clarification Rules

Ask for clarification before generating the chart when:

- The user asks for a generic chart and does not explicitly request a score distribution chart.
- The user requests score distribution but does not provide band rules.
- Bands mix `count` with `min`/`max`.
- A score value is missing or non-numeric.
- The user asks for a polished chart but does not specify title or style.
