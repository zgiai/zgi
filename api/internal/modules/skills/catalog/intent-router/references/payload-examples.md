# route_intent Payload Examples

Use this reference when constructing a `route_intent` payload.

## File Generation

```json
{
  "user_input": "Export the current report as a Word document.",
  "intent_id": "file_generation.docx",
  "task_type": "file_generation",
  "subtype": "docx",
  "confidence": 0.94,
  "recommended_action": "call_skill",
  "recommended_skill_id": "file-generator",
  "recommended_tool_name": "generate_docx",
  "routing_hints": {
    "requires_file_generation": true
  },
  "missing_info": [],
  "evidence": [
    "User explicitly asked to export as a Word document."
  ],
  "normalized_request": "Generate a DOCX file from the current report."
}
```

## Chart Request With Missing Type

For a generic chart request, use `request_user_input` before `route_intent` when chart type is not explicit.

If a route record is still needed without executing, use:

```json
{
  "user_input": "Use these scores to generate a chart.",
  "intent_id": "chart_generation.unknown",
  "task_type": "chart_generation",
  "subtype": "unknown",
  "confidence": 0.72,
  "recommended_action": "request_user_input",
  "recommended_skill_id": "chart-generator",
  "routing_hints": {
    "requires_chart_generation": true,
    "requires_confirmation": true
  },
  "missing_info": [
    {
      "field": "chart_type",
      "reason": "The user asked for a chart but did not specify the chart type.",
      "question": "Which chart type should be generated?",
      "options": ["bar", "line", "radar", "pie", "doughnut"]
    }
  ],
  "evidence": [
    "User asked to generate a chart from scores."
  ],
  "normalized_request": "Generate a chart from the provided score data after confirming the chart type."
}
```

## Database Query

```json
{
  "user_input": "Find active customers in Shanghai.",
  "intent_id": "database_query.filter_records",
  "task_type": "database_query",
  "subtype": "filter_records",
  "confidence": 0.82,
  "recommended_action": "query_database",
  "recommended_skill_id": "internal-database",
  "routing_hints": {
    "requires_database": true
  },
  "missing_info": [
    {
      "field": "database_table",
      "reason": "The target customer table is not known from context.",
      "question": "Which customer table or database should be queried?"
    }
  ],
  "evidence": [
    "User asked to find active customer records."
  ],
  "normalized_request": "Query customer records filtered by active status and Shanghai location."
}
```

## Knowledge Retrieval

```json
{
  "user_input": "What is our reimbursement policy for taxi receipts?",
  "intent_id": "knowledge_retrieval.policy_lookup",
  "task_type": "knowledge_retrieval",
  "subtype": "policy_lookup",
  "confidence": 0.9,
  "recommended_action": "retrieve_knowledge",
  "recommended_skill_id": "internal-knowledge",
  "routing_hints": {
    "requires_knowledge_base": true
  },
  "missing_info": [],
  "evidence": [
    "User asks about an organization policy."
  ],
  "normalized_request": "Retrieve policy guidance about taxi receipt reimbursement."
}
```
