# Intent Routing Rules

Use this reference when a request may route to a skill, tool, workflow, database, or knowledge retrieval path.

## General Routing

- Prefer the most specific safe route supported by the user's wording.
- Route to `request_user_input` when the next step depends on a missing choice that changes the execution path.
- Include `missing_info` rather than inventing IDs, file formats, chart types, table names, workflow bindings, or data filters.
- If a request contains multiple tasks, set `is_multi_intent` to true and put secondary candidates in `alternate_intents`.

## Skill Routing

- Use `file_generation` when the user asks to export, save, download, create a file, convert content, or produce Word/Excel/PDF/PPTX/CSV/Markdown/HTML/JSON/TXT.
- Use `chart_generation` when the user asks for a graph, visualization, chart artifact, radar chart, bar chart, line chart, pie chart, doughnut chart, scatter chart, or score distribution.
- Use `report_generation` when the primary deliverable is a report narrative, even if it may later be exported as a file.
- Use `schedule_planning` for planning and agendas. Do not claim calendar events are created.
- Use `calculation` for deterministic arithmetic or date math.

## Knowledge Versus Database

Use `knowledge_retrieval` when:

- The user asks about policies, docs, manuals, historical notes, contracts, uploaded documents, FAQs, or semantic content.
- The answer depends on unstructured text or knowledge-base retrieval.

Use `database_query` when:

- The user asks for records, rows, fields, counts, filters, joins, aggregations, or structured table data.
- The wording references customers, orders, tickets, transactions, assets, or other table-like entities.

Use `database_mutation` when:

- The user asks to add, update, delete, import, approve, reject, assign, or otherwise change records.
- Mark high impact and include missing confirmation details unless the request is explicit and authorized.

## Workflow Routing

Use `workflow_execution` when:

- The user asks to start, run, trigger, continue, inspect, or route through an automation/workflow.
- The request matches a known workflow binding in context.

If no binding is known, include `missing_info` for the workflow or use `request_user_input`.

## File and Upload Routing

Set `uses_uploaded_files` to true when uploaded files are required for the task.

Do not assume file content from filename alone. Evidence may mention filename, MIME type, and user-provided description, but not unsupported content claims.

If the user says "process this file" without a requested operation, classify as `clarification_required`.

## Routing Hints

Populate booleans when known:

- `needs_context`
- `uses_uploaded_files`
- `requires_database`
- `requires_knowledge_base`
- `requires_workflow`
- `requires_file_generation`
- `requires_chart_generation`
- `requires_confirmation`
- `is_high_impact`
- `is_multi_intent`
