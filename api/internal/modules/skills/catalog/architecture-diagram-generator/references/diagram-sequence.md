# Sequence Diagram Reference

Use this reference when `diagram_type` is `sequence`, API call sequence, service interaction, request lifecycle, or interface documentation.

## Payload Shape

```json
{
  "diagram_type": "sequence",
  "title": "Checkout API Sequence",
  "data": {
    "participants": ["Client", "API Gateway", "Order Service", "Payment Service"],
    "messages": [
      {"from": "Client", "to": "API Gateway", "label": "POST /checkout"},
      {"from": "API Gateway", "to": "Order Service", "label": "Create order"},
      {"from": "Order Service", "to": "Payment Service", "label": "Authorize payment"},
      {"from": "Payment Service", "to": "Order Service", "label": "Payment result"},
      {"from": "Order Service", "to": "API Gateway", "label": "Order status"}
    ]
  },
  "options": {"style": "technical"}
}
```

## Data Rules

- `participants`: required ordered list of actors or services.
- `messages`: required ordered list. Each message requires `from`, `to`, and should include `label`.
- Message endpoints must match participant names exactly.

## Clarification Rules

Ask when participants, call order, sync/async behavior, or response messages are unclear.
