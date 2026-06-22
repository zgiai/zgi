# ER Diagram Reference

Use this reference when `diagram_type` is `er`, ERD, entity relationship diagram, database design, or data model communication.

## Payload Shape

```json
{
  "diagram_type": "er",
  "title": "Order Data Model",
  "data": {
    "entities": [
      {"id": "users", "label": "users", "fields": ["id PK", "email", "created_at"]},
      {"id": "orders", "label": "orders", "fields": ["id PK", "user_id FK", "status", "total_amount"]},
      {"id": "order_items", "label": "order_items", "fields": ["id PK", "order_id FK", "sku", "quantity"]}
    ],
    "relationships": [
      {"from": "users", "to": "orders", "label": "1:N"},
      {"from": "orders", "to": "order_items", "label": "1:N"}
    ]
  },
  "options": {"style": "technical"}
}
```

## Data Rules

- `entities`: required list. Each entity requires `id`, should include `label`, and may include `fields`.
- `relationships`: required list. Each relationship requires `from` and `to`; both must match entity IDs.
- Use labels such as `1:1`, `1:N`, `N:M`, FK, owns, references, or belongs_to.

## Clarification Rules

Ask when entities, primary keys, foreign keys, cardinality, or relationship direction are unclear.
