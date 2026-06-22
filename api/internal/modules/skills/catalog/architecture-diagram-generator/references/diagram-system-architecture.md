# System Architecture Diagram Reference

Use this reference when `diagram_type` is `system_architecture`, microservice architecture, frontend/backend architecture, database architecture, API gateway architecture, or deployment/component architecture.

## Payload Shape

```json
{
  "diagram_type": "system_architecture",
  "title": "Order Platform Architecture",
  "output_filename": "order-platform-architecture",
  "data": {
    "groups": [
      {"id": "frontend", "label": "Frontend"},
      {"id": "backend", "label": "Backend"},
      {"id": "data", "label": "Data Layer"}
    ],
    "nodes": [
      {"id": "web", "label": "Web App", "type": "frontend", "group": "frontend", "layer": "client"},
      {"id": "gateway", "label": "API Gateway", "type": "gateway", "group": "backend", "layer": "edge"},
      {"id": "orders", "label": "Order Service", "type": "service", "group": "backend", "layer": "services"},
      {"id": "db", "label": "PostgreSQL", "type": "database", "group": "data", "layer": "data"}
    ],
    "edges": [
      {"from": "web", "to": "gateway", "label": "HTTPS"},
      {"from": "gateway", "to": "orders", "label": "REST"},
      {"from": "orders", "to": "db", "label": "SQL"}
    ]
  },
  "options": {
    "style": "technical",
    "direction": "left_to_right",
    "formats": ["svg", "html"]
  }
}
```

## Data Rules

- `nodes`: required list of components. Each node requires `id` and should include a human-readable `label`.
- `edges`: required list. Each edge requires `from` and `to`; both must match existing node IDs.
- `type`: optional component type such as frontend, gateway, service, queue, database, cache, object_storage, external.
- `groups`: recommended list of semantic modules. Use labels such as Frontend, Backend, Data Layer, External Systems, Observability, or Infrastructure.
- `group`: recommended node module ID matching `groups[].id`.
- `layer`: optional layout grouping such as client, edge, services, data, external.

## Clarification Rules

Ask before generating when the system boundary, main components, external systems, or relationships are unclear.
