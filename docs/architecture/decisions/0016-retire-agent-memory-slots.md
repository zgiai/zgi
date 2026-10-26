# ADR 0016: Retire the Agent Memory Slot System

- Status: Accepted
- Date: 2026-10-26
- Builds on: ADR 0013, ADR 0014, ADR 0015
- Amends: ADR 0013 (rollout switches; phase 4 "legacy tables and APIs are retired" happens now for Agents)

## Context

ADR 0013 moved Agent memory to blocks on the relationship store behind two fail-closed switches:
- `ZGI_RELATIONSHIP_MEMORY_ENABLED` selects the relationship store over the legacy Agent value store.
- `ZGI_MEMORY_BLOCKS_ENABLED` selects the block runtime over the slot runtime on that store.

Both default to off, so a deployment that sets neither still runs the oldest path. Three generations of Agent memory therefore coexist:

| Generation | Storage | Runtime |
|---|---|---|
| Legacy slots | `agent_memory_values` and its states, jobs, undo records, and events | `mutate_user_relationship_memory`, slot rendering, the slot extraction worker |
| Slots on the relationship store | `user_memory_items` | the same slot runtime |
| Blocks | `user_memory_items` with `definition_id` | `memory_edit`, `memory_view`, block rendering, consolidation, archive, conversation and archive search |

The product decision is to keep only blocks. Every Agent's memory uses blocks on the relationship store, and both switches go away.

## Decision

### 1. One runtime: blocks on the relationship store

- `RuntimeEnabled()` and `MemoryBlocksEnabled()` are removed. Agents always use the relationship store through its Agent adapter, and always the block runtime:
  - `memory_edit` and `memory_view`;
  - block rendering;
  - consolidation;
  - the archive and the conversation and archive searches.
- An Agent's own memory switch (`agent_memory_enabled`) still turns memory off for that Agent. The automatic-write fuses (`ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED`, the Agent's `auto_extraction_enabled`, each block's `auto_write`) are unchanged.
- **Removed:**
  - the `mutate_user_relationship_memory` tool for Agents and its slot policy;
  - slot rendering;
  - the slot extraction path of the worker;
  - the `blocks_enabled` flag of the templates API;
  - every code path that reads or writes `agent_memory_values` at runtime.
- The chat runtime is always given the Agent adapter, including in the AI chat wiring, which today always passes the legacy service.
- The timeline names memory changes `memory_edit`.
- **Other consumers.** The relationship store becomes always on for every consumer. DigitalHuman user memory, the profile page, and the Runtime Workspace memory capability no longer depend on `ZGI_RELATIONSHIP_MEMORY_ENABLED`. The DigitalHuman's own legacy tool, used when its file view is off (ADR 0014), is separate code and stays.

### 2. Data moves automatically

- A schema migration copies whatever is still only in the legacy store into the relationship store. It replaces the manual `go run ./cmd/migrate memory-sync-legacy -apply`, whose SQL it copies in a frozen form inside `DataFix`. It:
  - creates the subjects and relationships;
  - raises the epoch and extraction-cutoff fences;
  - copies items that are newer in the legacy store, with their block definitions;
  - removes items the user deleted after the last copy.
- The migration is idempotent; running it after a deployment already used the relationship store changes nothing.
- The legacy tables stay, because up migrations never drop tables, but no runtime path writes them. Runtime Workspace teardown and end-user erasure keep deleting from them until a later cleanup drops them. The `memory-sync-legacy` command is removed.

### 3. Block definitions have one owner

`agent_memory_slots` is today the authoritative draft list of an Agent's blocks, mirrored into `memory_block_definitions`. The builder writes block definitions directly:
- `memory_block_definitions` becomes the draft source.
- The Agent config JSON (`agent_memory_slots` in drafts and published snapshots) remains what runs read.
- The key name stays for compatibility with published snapshots.
- Migration `202610261400000000_memory_block_definitions_from_slots` copies the slots onto their definitions once, then `agent_memory_slots` is no longer read or written. The mirror had stored blocks first created disabled or without automatic writes with the column defaults (enabled, automatic writes on); the copy corrects them, and definitions are now inserted with every column set.
- Block IDs in the builder are definition IDs. An ID from `agent_memory_slots` that no definition has is matched by key, so drafts saved before the change still save.

### 4. Names and compatibility

- The builder, docs, and API reference say "memory block". The builder AI tool `replace_agent_memory_slots` becomes `replace_agent_memory_blocks`, with the argument `agent_memory_blocks` (`agent_memory_slots` is still read). The old name is accepted as an alias: the tool provider resolves it without listing it, and stored events and plans that carry it still render.
- Removing a block stops the Agent from reading and writing it. Content users already saved there is kept, and they can still view, export, and delete it.
- External and WebApp memory APIs keep their paths and fields:
  - `/api/v1/agents/memory`, its key, export, undo, history, and archive routes;
  - `values[]` with `key`, `max_chars`, `revision`, and the other existing fields;
  - `expected_revision` and `If-Match`.
- Console routes under `/agents/:agent_id/memory/slots` stay as aliases of `/memory/blocks`.

## Steps

1. Done. Scenario templates (done separately: four Agent templates and DigitalHuman memory instruction templates).
2. Done. This ADR and the inventory.
3. Done. The automatic data migration, with a PostgreSQL test seeded from legacy rows.
4. Done. Remove both switches and the slot runtime, always wire the Agent adapter, and update tests, docs, env examples, and the evaluation harness.
5. Done. Block definitions owned by `memory_block_definitions`; builder naming and the `replace_agent_memory_blocks` alias.
6. Full Agent and DigitalHuman evaluation rerun, then a Docker smoke test with no memory switches set.

## Consequences

- Every deployment runs the same Agent memory. Nothing depends on two switches that default to off.
- The oldest data is migrated without an operator step. A deployment that never turned the switches on starts using the relationship store on upgrade.
- Rollback to the slot runtime is no longer possible from configuration; it needs the previous release. The legacy tables keep their rows, so a downgrade still finds them.
- **Risk:** deployments that never ran the relationship store now run it for every consumer. Mitigations:
  - the migration's row counts;
  - the evaluation gate on automatic writes, which stays as before;
  - a smoke test with no memory switches set.
