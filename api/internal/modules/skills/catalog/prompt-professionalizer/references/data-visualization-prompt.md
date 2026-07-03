# Data Visualization Prompt

## Use When

Use for prompts targeting chart generators, ECharts, Tableau, Power BI, Datawrapper, dashboard builders, BI tools, or data visualization agents.

## Required Elements

Check for:

- Analysis goal.
- Dataset or data source description.
- Data fields.
- Metrics.
- Dimensions.
- Time range.
- Chart type.
- Comparison relationship.
- Grouping or segmentation.
- Filters.
- Sorting.
- Title, subtitle, annotations, and units.

## Prompt Shape

Include:

- Business question or analysis goal.
- Data fields and definitions supplied by the user.
- Chart type and rationale.
- Encodings: X axis, Y axis, color/grouping, size, labels, tooltip.
- Filters, sorting, aggregation, and time window.
- Title, subtitle, notes, and caveats.

## Chart-Type Hints

- Trend over time: line chart or area chart.
- Category comparison: bar chart.
- Share of total: pie, doughnut, stacked bar, or treemap.
- Distribution: histogram, box plot, violin, score distribution.
- Relationship: scatter plot.
- Ranking: sorted bar chart or table.
- Multi-metric comparison: grouped bar, small multiples, radar only when dimensions are comparable.

## Clarification Rules

Ask when data fields or analysis goal are missing. If chart type is missing but the analysis goal is clear, choose a chart type and state the reason under `说明`.

## Boundary Rules

- Do not invent metrics, dimensions, data values, time ranges, or field names.
- Do not claim data was analyzed if only a prompt was produced.
- Do not create a chart directly unless the user explicitly routes the prepared prompt to a chart tool.
