# Intent Router Taxonomy

Read this reference before calling `route_intent`.

## Task Types

Use exactly one `task_type`:

- `general_qa`: direct answer, explanation, rewrite, translation, brainstorming, or general assistance that does not need a specialized route.
- `knowledge_retrieval`: answer requires searching configured knowledge bases, documents, policies, FAQs, manuals, or organization knowledge.
- `database_query`: read-only database/table lookup, aggregation, filtering, SQL-like query, or record inspection.
- `database_mutation`: create, update, delete, import, or otherwise mutate database records.
- `workflow_execution`: run, inspect, or route to an available workflow or automation.
- `file_generation`: create, export, save, download, or convert content into a file such as txt, markdown, html, json, csv, docx, xlsx, pdf, or pptx.
- `chart_generation`: create a chart, graph, visualization, radar chart, bar chart, line chart, pie chart, doughnut chart, scatter chart, score distribution chart, or SVG chart artifact.
- `report_generation`: create a weekly report, monthly report, work summary, project status report, business report, or management summary.
- `schedule_planning`: plan a day, week, agenda, study plan, workload schedule, or project timeline without creating real calendar events.
- `calculation`: exact arithmetic, percentages, date calculations, deterministic formula evaluation, or unit-like numeric transformation.
- `code_or_debugging`: write, modify, explain, test, or debug source code, configuration, build errors, or repository behavior.
- `data_analysis`: analyze a dataset, spreadsheet, metrics, trends, experiment, KPI movement, cohort, funnel, or business/product data.
- `clarification_required`: the request is too ambiguous to classify into one actionable route.
- `unsupported`: the request cannot be handled safely or is outside available capabilities.

## Recommended Actions

Use exactly one `recommended_action`:

- `answer_directly`: respond without another business tool.
- `call_skill`: load and use a specific Skill.
- `call_tool`: invoke a specific non-skill or builtin tool directly.
- `run_workflow`: run a known workflow or automation binding.
- `query_database`: perform read-only database retrieval.
- `mutate_database`: perform database mutation only after required safeguards.
- `retrieve_knowledge`: retrieve from knowledge bases or agent-bound knowledge.
- `request_user_input`: ask structured clarification before executing.
- `reject_or_escalate`: refuse, escalate, or explain unsupported/high-risk limitations.

## Intent ID Rules

Use a stable dotted identifier:

- Prefer `<task_type>.<subtype>`.
- Use lowercase ASCII, digits, underscores, and dots only.
- Examples: `file_generation.docx`, `chart_generation.bar`, `database_query.filter_records`, `knowledge_retrieval.policy_lookup`, `workflow_execution.run_bound_workflow`.
- If no subtype is clear, use `<task_type>.unknown`.

## Confidence Scale

- `0.90-1.00`: Explicit request with clear target and low ambiguity.
- `0.75-0.89`: Strong route, minor missing execution details.
- `0.50-0.74`: Plausible route, meaningful ambiguity remains.
- `0.20-0.49`: Weak route; likely needs clarification.
- `0.00-0.19`: Unsupported, conflicting, or not enough signal.

Do not exceed `0.85` when two materially different task types could both be correct.

## Target Skill Hints

When `recommended_action` is `call_skill`, use these existing skill IDs when appropriate:

- `file-generator` for `file_generation`.
- `chart-generator` for `chart_generation`.
- `work-report-generator` for `report_generation`.
- `schedule-planner` for `schedule_planning`.
- `calculator` for `calculation`.
- `internal-knowledge` or `agent-knowledge` for `knowledge_retrieval`.
- `internal-database` or `agent-database` for `database_query` and `database_mutation`.
- `agent-workflow` for `workflow_execution`.
