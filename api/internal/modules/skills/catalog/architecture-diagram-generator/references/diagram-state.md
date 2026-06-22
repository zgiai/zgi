# State Diagram Reference

Use this reference when `diagram_type` is `state`, state machine, lifecycle transition, status flow, or object lifecycle.

## Payload Shape

```json
{
  "diagram_type": "state",
  "title": "Order State Machine",
  "data": {
    "states": [
      {"id": "created", "label": "Created", "type": "initial", "layer": "1"},
      {"id": "paid", "label": "Paid", "type": "normal", "layer": "2"},
      {"id": "shipped", "label": "Shipped", "type": "normal", "layer": "3"},
      {"id": "closed", "label": "Closed", "type": "final", "layer": "4"}
    ],
    "transitions": [
      {"from": "created", "to": "paid", "label": "payment success"},
      {"from": "paid", "to": "shipped", "label": "ship"},
      {"from": "shipped", "to": "closed", "label": "confirm receipt"}
    ]
  }
}
```

## Data Rules

- `states`: required list of states. Each state requires `id` and should include `label`.
- `transitions`: required list of valid transitions.
- Transition endpoints must match state IDs.

## Clarification Rules

Ask when initial state, final state, failure state, cancellation path, or retry transition is unclear.
