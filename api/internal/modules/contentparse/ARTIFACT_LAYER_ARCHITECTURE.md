# Content Parse Artifact Layer

This module owns parse and chunk artifacts for the future Data Library asset
layer. It must not switch dataset indexing traffic by itself.

## Goal

ZGI should treat enterprise documents as reusable knowledge assets:

```text
source file
  -> parse artifact
  -> chunk artifact set
  -> vector artifact
  -> graph / extraction artifacts
  -> downstream reuse by knowledge bases, databases, agents, workflows
```

The current implementation is still shadow-first. Dataset indexing remains the
business path until artifact quality, materialization, and rollback are proven.

## Ownership

```text
internal/capabilities/contentparse
  Provider-agnostic parsing capability:
  routing, orchestration, adapter catalog, ParseArtifact production.

internal/capabilities/chunking
  Provider-agnostic chunking capability:
  ParseArtifact -> ChunkSourceDocument -> ChunkPlan -> ChunkUnit.

internal/modules/contentparse
  Persistence and inspection for parse/chunk artifacts:
  provider configs, route policies, parse runs, chunking runs,
  parse artifacts, chunk artifact sets.

internal/modules/dataset
  Existing knowledge-base business flow:
  dataset documents, segments, child chunks, embedding, vectorDB writes.

future internal/modules/datalibrary
  Product-level DocumentAsset lifecycle:
  document versions, reuse events, vector artifacts, graph artifacts,
  extraction artifacts, permission inheritance, lineage.
```

## Current Safe Path

```text
dataset indexing
  -> legacy Extract
  -> legacy Transform
  -> loadSegments
  -> embedding
  -> vectorDB

content parse shadow
  -> ParseArtifact
  -> ChunkArtifactSet
  -> quality summary
  -> no segment writes
  -> no vector writes
```

The two flows can compare quality, but the shadow flow must not mutate
`document_segments`, `child_chunks`, or dataset vector collections.

## Design Pattern

Use a pipeline with adapters at explicit seams:

```text
Parse Pipeline
  ParseRequest -> ParseArtifact

Chunk Pipeline
  ParseArtifact -> ChunkSourceDocument -> ChunkPlan -> ChunkUnit

Artifact Registry
  ParseArtifact / ChunkArtifactSet / VectorArtifact

Business Adapters
  ChunkArtifactSet -> Dataset SegmentPlan
  ChunkArtifactSet -> VectorArtifactPlan
  ParseArtifact -> ExtractionArtifact
```

The business adapters are the only place where artifact output can become
dataset-specific records. They must support dry-run before write mode.

## Cutover Rule

Dataset traffic can only switch from legacy transform to artifact materializer
behind an explicit feature flag after all of these are true:

- text retention is stable against legacy baselines
- parent/child chunk counts are explainable
- metadata required by `loadSegments` is generated deterministically
- materializer dry-run matches expected segment plans
- rollback can return to legacy transform without data loss
- vector writes are isolated by dataset or permission-safe shared filters

## Near-Term Refactor Target

`internal/modules/dataset/indexing` should keep only a thin shadow hook:

```go
type ContentParseShadowRunner interface {
    EnqueueDatasetIndexingShadow(ctx context.Context, input DatasetShadowInput) bool
}
```

The implementation should live under `internal/modules/contentparse/service`.
Dataset indexing should not know how parse runs, chunking runs, chunk artifact
sets, or provider fallback metadata are persisted.
