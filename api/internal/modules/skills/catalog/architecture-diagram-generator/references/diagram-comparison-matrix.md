# Comparison Matrix Reference

Use this reference when `diagram_type` is `comparison_matrix`, feature matrix, product comparison, competitor analysis, or solution selection.

## Payload Shape

```json
{
  "diagram_type": "comparison_matrix",
  "title": "Vector Database Comparison",
  "data": {
    "columns": ["Milvus", "Pinecone", "pgvector"],
    "rows": ["Deployment", "Filtering", "Cost", "Operations"],
    "cells": [
      ["Self-hosted", "Managed", "Postgres extension"],
      ["Strong", "Strong", "Moderate"],
      ["Infra cost", "Usage based", "DB cost"],
      ["Higher", "Lower", "Moderate"]
    ]
  },
  "options": {"style": "business"}
}
```

## Data Rules

- `columns`: required compared products, vendors, options, or plans.
- `rows`: required criteria, features, metrics, or decision factors.
- `cells`: required matrix values. It must have exactly one row per `rows` entry and one value per `columns` entry.

## Clarification Rules

Ask when options, criteria, scoring labels, or missing cells are unclear.
