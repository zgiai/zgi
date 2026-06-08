# Radar Chart Reference

Use this reference when `chart_type` is `radar`, `spider`, score profile, subject ability chart, or personal-vs-class-average comparison.

## Payload Shape

```json
{
  "chart_type": "radar",
  "title": "Score Radar Chart",
  "output_filename": "score-radar-chart",
  "data": {
    "dimensions": ["Chinese", "Math", "English", "Physics", "Chemistry", "Biology"],
    "max_value": 100,
    "series": [
      {
        "name": "Class Average",
        "values": [78, 82, 80, 75, 73, 76],
        "color": "#94a3b8"
      },
      {
        "name": "Student",
        "values": [88, 92, 84, 81, 77, 86],
        "color": "#2563eb"
      }
    ]
  },
  "options": {
    "style": "comparison",
    "width": 900,
    "height": 700,
    "show_values": true,
    "legend": true
  }
}
```

## Data Rules

- `dimensions`: required list of subject or metric names. It must contain at least 3 items.
- `data.series`: required list of 1 or 2 data layers.
- Each series requires:
  - `name`: layer label.
  - `values`: numeric values with the same length as `dimensions`.
  - `color`: optional hex color.
- `max_value`: optional positive number. Defaults to `100`.
- Every value must be between `0` and `max_value`.
- Use one series for a single score radar chart.
- Use two series for comparison charts. Put the baseline, such as class average, first. Put the focus layer, such as personal score, second so it is rendered above the baseline.

## Color Rules

- Use layer colors to distinguish different data layers.
- Default single-layer color is blue.
- Default comparison colors:
  - Baseline: muted gray-blue.
  - Focus layer: stronger blue.
- Do not assign a different fill color to each subject by default. In a radar chart, color represents data layers; subjects are represented by axes and labels.

## Style Rules

- Use `comparison` style when the chart compares a focus layer against a baseline such as class average.
- Use `teaching` style for classroom or learning material.
- Use `business` style for reports.
- Use `simple` style when the user does not specify a look.

## Clarification Rules

Ask for clarification before generating the chart when:

- The chart type is not explicit and several chart types could fit.
- The user asks for a polished chart but does not specify title or style.
- The number of dimensions and values differs.
- A score is missing or non-numeric.
- A score exceeds `max_value`.
- The user asks for a comparison but only provides one layer.
- The user provides different full marks per subject. This first version expects one shared `max_value`.

## Examples

Single-layer:

```json
{
  "chart_type": "radar",
  "title": "个人成绩雷达图",
  "output_filename": "personal-score-radar",
  "data": {
    "dimensions": ["语文", "数学", "英语", "物理", "化学", "生物"],
    "max_value": 100,
    "series": [
      {"name": "个人成绩", "values": [88, 92, 84, 81, 77, 86]}
    ]
  }
}
```

Comparison:

```json
{
  "chart_type": "radar",
  "title": "个人与班级平均分对比",
  "output_filename": "score-comparison-radar",
  "data": {
    "dimensions": ["语文", "数学", "英语", "物理", "化学", "生物", "历史"],
    "max_value": 100,
    "series": [
      {"name": "班级平均分", "values": [78, 82, 80, 75, 73, 76, 79]},
      {"name": "个人成绩", "values": [88, 92, 84, 81, 77, 86, 83]}
    ]
  },
  "options": {
    "show_values": true
  }
}
```
