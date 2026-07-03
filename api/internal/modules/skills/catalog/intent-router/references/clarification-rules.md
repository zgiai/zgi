# Intent Clarification Rules

Use this reference when a request is ambiguous, incomplete, high impact, or risky.

## Missing Information

Add a `missing_info` item for each blocker:

- `field`: stable field name such as `chart_type`, `file_format`, `database_table`, `workflow_binding_id`, `date_range`, `filter`, `confirmation`, or `uploaded_file`.
- `reason`: why this information blocks reliable routing or execution.
- `question`: concise user-facing question that can resolve the blocker.
- `options`: optional concrete options only when each option is a directly usable answer.

Do not add vague options such as "other", "any", "not sure", or "depends".

## When to Use request_user_input

Call `request_user_input` instead of `route_intent` only when:

- No reliable `task_type` can be chosen.
- The route would trigger database mutation, workflow execution, payment, notification, deletion, approval, or another high-impact action and confirmation is missing.
- Two or more routes have similar confidence and would call different downstream capabilities.
- The user explicitly asks you to ask before acting.

Ask 1-4 focused questions. Stop the turn after requesting input.

## When missing_info Is Enough

Use `route_intent` with `missing_info` when:

- The broad task type is clear but execution details are missing.
- The next system can collect missing parameters later.
- The request is read-only or low impact.

Examples:

- "Export this as a file" -> `file_generation.unknown`, missing `file_format`.
- "Show me customer orders" -> `database_query.filter_records`, missing `database_table` or `filter` if not available in context.
- "Make a chart from these scores" -> if chart type is not explicit, use `request_user_input` because chart type changes the downstream reference and payload.

## Confidence and Ambiguity

- Use lower confidence when missing_info contains route-changing fields.
- Use `clarification_required` when missing information prevents choosing a task type.
- Use `unsupported` only when the request cannot be handled safely or no available route exists.
