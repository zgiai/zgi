# Content Parse Capability

This package is the non-invasive foundation for a platform parsing capability.

Goals:

1. Give `dataset`, `chat`, and `file_process` a stable contract boundary.
2. Hide concrete parsing engines such as Hyperparse SDK or remote parsing APIs.
3. Introduce a reusable capability layer without changing any current business flow.

Non-goals in this first step:

1. No route registration.
2. No persistence changes.
3. No existing dataset indexing path rewired to this capability yet.

Current rollout shape:

```text
modules/* -> internal/contracts.ContentParseService -> capabilities/contentparse -> adapters/*
```

The first adapter is `hyperparse_sdk`, which keeps all direct Hyperparse SDK
knowledge inside the capability layer. Existing modules can adopt this later
without importing engine-specific packages.

Chunking architecture foundation:

```text
ParseArtifact
  -> capabilities/chunking.CanonicalMapper
  -> contracts.ChunkSourceDocument
  -> contracts.ChunkPlanner
  -> contracts.ChunkPlan
  -> future ChunkUnit producers in dataset/chat/workflow
```

This foundation is intentionally introduced without rewiring current business
processors yet. It exists so downstream modules can adopt one canonical
intermediate representation instead of depending directly on parser-specific
fields or ad-hoc metadata maps.

Provider-policy routing foundation:

```text
ParseRequest
  -> routing.DefaultPlanner
  -> RoutePlan (shadow-first)
  -> future provider-policy execution
```

The planner is intentionally introduced ahead of any traffic cutover so route
plans can be generated, inspected, and compared without changing current parse
execution behavior.
