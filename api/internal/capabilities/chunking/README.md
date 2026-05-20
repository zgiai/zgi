# Chunking Capability

This package is the provider-agnostic boundary between parsing output and
business indexing flows.

Current foundation:

```text
contracts.ParseArtifact
  -> chunking.CanonicalMapper
  -> contracts.ChunkSourceDocument
  -> chunking.DefaultPlanner
  -> contracts.ChunkPlan
```

Future rollout:

```text
contracts.ChunkPlan
  -> executor.Partitioner
  -> executor parallel workers
  -> stable merge
  -> quality/*
  -> contracts.ChunkUnit
  -> adapters/dataset
  -> existing dataset TransformedChunk loading
```

Rollout rule: keep the current dataset indexing path unchanged until shadow
metrics show that the new chunk output is at least as stable as the legacy path.

Shadow comparison:

```text
legacy ExtractOutput
  -> current dataset Transform + cleanDatasetTransformedChunks
  -> legacy TransformedChunk baseline

contentparse shadow artifact
  -> chunking.CanonicalMapper
  -> chunking.DefaultPlanner
  -> executor.Partitioner + bounded workers
  -> quality.Score
```

The quality score is an inspection signal only. It compares text retention,
chunk count expansion, stable ordering, low-value filtering, and layout
coverage before any dataset business flow is allowed to switch traffic.
