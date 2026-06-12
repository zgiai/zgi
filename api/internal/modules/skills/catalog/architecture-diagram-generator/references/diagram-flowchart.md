# Flowchart Reference

Use this reference when `diagram_type` is `flowchart`, business process, approval workflow, task execution process, or user operation flow.

## Payload Shape

```json
{
  "diagram_type": "flowchart",
  "title": "Purchase Approval Flow",
  "data": {
    "groups": [
      {"id": "requester", "label": "Requester"},
      {"id": "review", "label": "Review"},
      {"id": "result", "label": "Result"}
    ],
    "nodes": [
      {"id": "submit", "label": "Submit Request", "type": "start", "group": "requester", "layer": "1"},
      {"id": "manager", "label": "Manager Review", "type": "approval", "group": "review", "layer": "2"},
      {"id": "finance", "label": "Finance Review", "type": "approval", "group": "review", "layer": "3"},
      {"id": "done", "label": "Approved", "type": "end", "group": "result", "layer": "4"}
    ],
    "edges": [
      {"from": "submit", "to": "manager", "label": "request"},
      {"from": "manager", "to": "finance", "label": "approved"},
      {"from": "finance", "to": "done", "label": "approved"}
    ]
  },
  "options": {"direction": "left_to_right", "style": "business"}
}
```

## Data Rules

- `nodes`: required process steps or decisions.
- `edges`: required transitions. Use labels for decision branches such as approved, rejected, yes, no, retry, or timeout.
- Use `groups` and node `group` values when steps belong to roles, departments, modules, phases, or business domains.
- Use ordered `layer` values to keep the process readable.

## Clarification Rules

Ask when decision branches, rollback paths, owners, or terminal states are unclear.
