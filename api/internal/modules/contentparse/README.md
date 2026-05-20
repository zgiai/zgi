# Content Parse Module

This module is reserved for the independent provider-policy management layer of content parsing.

Phase 1 goals:
- keep current dataset and file_process business unchanged
- introduce independent persistence for provider config, route policy, health, parse runs, and chunking runs
- support shadow-first rollout

This module intentionally does not replace the existing content parsing execution path yet.

Dataset shadow inspection:
- `/shadow/datasets/:dataset_id` returns latest parse shadow runs grouped by document
- chunking shadow data is aggregated into a dataset-level `chunk_quality` gate
- `chunk_quality.decision` is one of `unknown`, `ready`, `observe`, or `blocked`
- the decision is advisory only and must not switch dataset indexing traffic by itself

Detailed phase-1 architecture, table planning, and directory layout live in:
- [PROVIDER_POLICY_ARCHITECTURE.md](./PROVIDER_POLICY_ARCHITECTURE.md)
- [ARTIFACT_LAYER_ARCHITECTURE.md](./ARTIFACT_LAYER_ARCHITECTURE.md)
