---
name: chart-generator
description: Generate downloadable SVG charts from structured data, including radar, bar, line, pie, doughnut, scatter, and score distribution charts.
when_to_use: Use this skill when the user asks to create, export, or generate a chart, graph, radar chart, spider chart, bar chart, line chart, pie chart, doughnut chart, scatter chart, score distribution chart, score chart, comparison chart, or data visualization from provided data. For casual, vague, incomplete, or non-structured chart and data visualization requests, first route through prompt-professionalizer to optimize the visualization prompt and extract chart requirements, then call this skill.
provider_type: builtin
provider_id: chart_generator
runtime_type: tool
tools:
  - generate_chart
supported_callers:
  - aichat
  - agent
max_calls_per_turn: 5
timeout_seconds: 5
tool_governance:
  generate_chart:
    tool_id: chart.generate
    skill_id: chart-generator
    domain: files
    effect: create
    asset_type: file
    risk_level: medium
    requires_asset_resolution: false
    reversible: true
    bulk_sensitive: false
    external_side_effect: false
    permission_scopes:
      - file:create
    default_approval_policy: never_ask
    allowed_permission_tiers:
      - basic
      - advanced
      - full
    audit_required: true
    idempotency_required: false
display:
  icon: chart-no-axes-combined
  category: data_analysis
  scenarios:
    - data_insights
    - content_creation
    - office_collaboration
  label:
    en_US: Chart Generator
    zh_Hans: 图表生成器
  description:
    en_US: Designed for reports and data analysis that need visual evidence; turns structured data into SVG radar, bar, line, pie, doughnut, scatter, or distribution charts.
    zh_Hans: 适用于汇报和数据分析中的可视化展示，可将结构化数据生成雷达图、柱状图、折线图、饼图、环形图、散点图或分布图。
  when_to_use:
    en_US: Use when the answer should include a generated chart artifact.
    zh_Hans: 当回答需要生成图表文件时使用。
  tags:
    en_US:
      - Chart
      - Visualization
      - Data
    zh_Hans:
      - 图表
      - 可视化
      - 数据
---

# Chart Generator Skill

Use this skill to generate downloadable SVG chart artifacts from structured data. Supported chart types are `radar`, `bar`, `line`, `pie`, `doughnut`, `scatter`, and `score_distribution`; the structure is designed so future chart types can be added with a reference document and builtin renderer.

## Supported Chart Types

- `radar`: radar/spider charts for multi-dimensional score profiles and personal-vs-average comparisons.
- `bar`: bar/column charts for category comparison, grouped comparison, and score comparison.
- `line`: line charts for trends, ordered stages, time series, and score changes.
- `pie`: pie charts for proportions, composition, and share-of-whole views.
- `doughnut`: doughnut/ring charts for proportions with a visible center total.
- `scatter`: scatter plots for two-variable relationships such as rank vs score.
- `score_distribution`: score band distribution charts from raw scores or precomputed band counts.

## Workflow

1. Determine whether the user explicitly requested a `chart_type`: `radar`, `bar`, `line`, `pie`, `doughnut`, `scatter`, or `score_distribution`.
2. If the request is casual, vague, incomplete, or not already structured for a chart or visualization tool, first use `prompt-professionalizer` to produce an optimized data visualization prompt and chart requirements.
3. If the user only says a generic request such as "generate a chart", "make a graph", "生成图表", "做个图", or "可视化", call `request_user_input` before calling `generate_chart`.
4. Read exactly one reference document for that chart type before calling `generate_chart`.
5. Convert the user's data into the JSON payload documented in the selected reference.
6. Validate that all required data is present and internally consistent.
7. Call `call_skill_tool` with `tool_name` set to `generate_chart`.
8. If the user explicitly asks to save, create, add, upload, or import the chart into File Management or the current Files page, first generate the temporary chart artifact here, then call `file-manager/save_file_to_management` with the returned `tool_file_id`/`file_id` and destination filename.
9. In the final answer, briefly mention the generated chart filename and any assumptions. Do not paste SVG source unless the user explicitly asks for it.

## Clarification Workflow

When any required decision is missing or ambiguous, call `request_user_input` instead of writing a plain clarification message. This ensures the UI renders the clarification as a structured confirmation card with optional quick replies.

Generic chart requests are incomplete even when the data can be parsed. Do not infer `bar` just because the data is ranking or category scores, do not infer `line` just because the data is ordered, do not infer `pie` or `doughnut` just because values can be totaled, do not infer `scatter` just because values have order, and do not infer `radar` just because the data contains multiple values. If the user did not explicitly name the chart type, ask.

For generic chart requests, chart type, chart title, and rendering style are required decisions. Ask for them before reading a chart reference or calling `generate_chart`.

Use a brief `message` explaining what needs confirmation, then provide 1-4 focused `questions`. Include `options` only when each option is a concrete answer that can be used directly. Omit options for open-ended questions such as the chart title, because the user can type freely.

After calling `request_user_input`, stop the turn and wait for the user's answer. Do not call `generate_chart` in the same turn.

Ask about:

- Chart type when the user did not explicitly request `radar`, `bar`, `line`, `pie`, `doughnut`, `scatter`, or `score_distribution`.
- Chart title when the user did not provide a title.
- Data mapping when labels, x-axis/category names, dimensions, series names, or values are unclear.
- Rendering style when the user asks for a specific look or the use case implies a choice. Supported styles: `simple`, `business`, `teaching`, `comparison`.
- Whether to show values, legend, or grid lines when the user explicitly cares about readability or presentation.
- Score bands when the user requests `score_distribution` but did not specify the band rules.

Example generic request that must trigger `request_user_input`:

```text
1张三98 2孙八98 3李四88 4吴十82 5钱七80 6周九78 7王五74 8赵六67 生成图表
```

Do not answer "I will use a bar chart" for this kind of request. Ask the user to confirm chart type, title, and style first.

Example `request_user_input` payload:

```json
{
  "message": "I can generate the chart, but need to confirm a few details first.",
  "questions": [
    {
      "id": "chart_type",
      "question": "Which chart type should I generate: bar, line, radar, pie, doughnut, scatter, or score_distribution?"
    },
    {
      "id": "title",
      "question": "What chart title should be shown?"
    },
    {
      "id": "style",
      "question": "Which rendering style should I use?",
      "options": [
        { "label": "simple" },
        { "label": "business" },
        { "label": "teaching" },
        { "label": "comparison" }
      ]
    },
    {
      "id": "show_values",
      "question": "Should values be displayed on the chart?",
      "options": [
        { "label": "show values" },
        { "label": "hide values" }
      ]
    }
  ]
}
```

## References

Read exactly one reference after choosing the chart type:

| Requested chart | Read reference |
| --- | --- |
| `radar`, `spider`, score profile, subject ability chart, personal-vs-average comparison | `chart-radar.md` |
| `bar`, `column`, category comparison, grouped comparison | `chart-bar.md` |
| `line`, trend, time series, score change, progress over attempts | `chart-line.md` |
| `pie`, proportion, composition, share of whole | `chart-pie.md` |
| `doughnut`, `donut`, ring chart, share of whole with center total | `chart-doughnut.md` |
| `scatter`, scatter plot, two-variable relationship | `chart-scatter.md` |
| `score_distribution`, score band distribution, grade range count | `chart-score-distribution.md` |

If the user requests a chart type that is not listed, say it is not supported yet and offer to structure the data for a future chart type.

## Unified Payload

`generate_chart` accepts:

- `chart_type`: required. Supported values: `radar`, `bar`, `line`, `pie`, `doughnut`, `scatter`, `score_distribution`.
- `title`: optional chart title.
- `output_filename`: optional ASCII filename without extension. Defaults to `chart`.
- `data`: required chart-specific data object.
- `options`: optional rendering options. Common keys: `width`, `height`, `style`, `show_values`, `legend`, `grid`.
- For scatter charts, `options.show_labels` controls point labels.
- `lifecycle`: optional file lifecycle, `persistent` or `temporary`. Defaults to `persistent`.

## Constraints

- Before calling `generate_chart`, use `prompt-professionalizer` when the user's request is casual, vague, incomplete, or not already structured for chart generation. Direct tool calls are allowed only when the chart type, data mapping, title or purpose, and key rendering requirements are already complete.
- Do not call `generate_chart` until the selected chart reference has been read.
- Do not read a chart reference until the chart type has been explicitly provided by the user or confirmed through `request_user_input`.
- Generate SVG artifacts only. Do not promise PNG, PDF, or interactive charts.
- Do not invent scores, labels, dimensions, class averages, or comparison data.
- Do not silently choose a chart type, title, or style for a generic chart request.
- Do not use unsupported chart types silently. Unsupported chart types must be reported as unsupported.
- Keep filenames short, ASCII, and free of path separators.
- If the user's data is ambiguous, ask for clarification or state the assumptions before generating a chart.
