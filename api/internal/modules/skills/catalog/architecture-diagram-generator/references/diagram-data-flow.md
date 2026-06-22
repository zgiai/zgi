# Data Flow Diagram Reference

Use this reference when `diagram_type` is `data_flow`, data pipeline, ETL flow, or input-process-store-output chain.

## Payload Shape

```json
{
  "diagram_type": "data_flow",
  "title": "Event Analytics Data Flow",
  "data": {
    "groups": [
      {"id": "input", "label": "Input"},
      {"id": "processing", "label": "Processing"},
      {"id": "storage", "label": "Storage"},
      {"id": "output", "label": "Output"}
    ],
    "nodes": [
      {"id": "events", "label": "User Events", "type": "input", "group": "input", "layer": "input"},
      {"id": "stream", "label": "Event Stream", "type": "queue", "group": "processing", "layer": "ingest"},
      {"id": "processor", "label": "Stream Processor", "type": "process", "group": "processing", "layer": "processing"},
      {"id": "warehouse", "label": "Data Warehouse", "type": "store", "group": "storage", "layer": "storage"},
      {"id": "dashboard", "label": "Dashboard", "type": "output", "group": "output", "layer": "output"}
    ],
    "edges": [
      {"from": "events", "to": "stream", "label": "publish"},
      {"from": "stream", "to": "processor", "label": "consume"},
      {"from": "processor", "to": "warehouse", "label": "write"},
      {"from": "warehouse", "to": "dashboard", "label": "query"}
    ]
  }
}
```

## Data Rules

- `nodes` must include each input, processor, store, and output that should appear.
- `edges` must show data movement and should use labels such as ingest, validate, transform, store, query, export, or notify.
- Use `groups` and node `group` values for semantic stages such as Input, Ingestion, Processing, Storage, Output, Consumers, or External Systems.
- Use `layer` to preserve the natural order: input, ingest, processing, storage, output.

## Clarification Rules

Ask when input sources, storage targets, processing stages, or output consumers are missing.
