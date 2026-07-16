# Content Parse Capability

This package is the platform parsing capability shared by interactive parsing,
data ingestion, and runtime file-content consumers.

Goals:

1. Give `dataset`, `chat`, and `file_process` a stable contract boundary.
2. Hide concrete parsing engines such as Hyperparse SDK or remote parsing APIs.
3. Keep provider selection, health checks, request-scoped configuration, and
   fallback execution consistent across business flows.

Runtime shape:

```text
playground / data library / datasource / chat / agent file tools
  -> internal/contracts.RoutedContentParseService
  -> request-scoped provider catalog
  -> routing planner
  -> provider adapters with fallback
```

`hyperparse_sdk` keeps direct parser-engine knowledge inside the capability
layer. Runtime consumers should call `ParseWithRouting` when the routed
interface is available. `Parse` remains as a compatibility boundary for
callers that intentionally select an engine directly.

The workflow `ContentExtractor` preserves upload-text caching, size limits, and
media handling around this capability. Its legacy `ExtractProcessor` path is a
last-resort compatibility fallback, not the primary routing mechanism.

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

Provider-policy routing:

```text
ParseRequest
  -> routing.DefaultPlanner
  -> request-scoped provider catalog
  -> RoutePlan
  -> provider-policy execution with fallback
```
