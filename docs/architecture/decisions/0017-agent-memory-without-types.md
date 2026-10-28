# ADR 0017: Agent Memory Without Configured Types

- Status: Accepted
- Date: 2026-10-26
- Builds on: ADR 0013, ADR 0014, ADR 0016
- Amends: ADR 0016 (block definitions, the builder AI tool, and the `/memory/blocks` routes)

## Context

ADR 0016 left Agents with one memory runtime: blocks on the relationship store. A builder still declared the blocks, and each one had several settings:
- a key and a name;
- a description of what belongs in it;
- a character limit;
- read-only and automatic-write switches.

Two problems remained:
- Builders had to plan a memory schema before they knew what users would say. Facts that fit no block were lost or forced into the wrong one.
- Read-only blocks never worked end to end. A builder could not give one a value, so to the model a read-only block was an empty file it could not edit.

DigitalHumans already work without a schema: memory instructions describe what to remember, and the model writes what it needs (ADR 0014, ADR 0015).

## Decision

### 1. No configured types

- An Agent's memory is a set of files the model names and organizes itself under `/user/`, for example `/user/profile.md` or `/user/travel_plans.md`.
- The builder configures three things:
  - whether memory is on;
  - whether automatic memory is on;
  - the memory instructions (up to 2,000 characters), which say what this Agent should remember and what it should not.
- Read-only blocks, per-block limits, per-block automatic-write switches, and block descriptions are removed. Fixed information belongs in the system prompt or a knowledge base.
- Scenario templates keep their memory instructions and sample conversations and drop their block lists.

### 2. One limit: the user's total

- A user's files together hold at most 10,000 characters (`agentmemory.MaxMemoryChars`). There is no per-file limit.
- The relationship store enforces the total in a new `files` contract mode: any valid file name, contact details and secrets rejected, and the total checked inside the write transaction. Every write path goes through this check: the conversation, consolidation, the external API, WebApp, and profile edits.
- The runtime tells the model how full memory is. `memory_edit` rejects an edit past the total and points to the `archive` command, which moves older text to the user's archive.

### 3. Runtime

- `memory_edit` accepts any `/user/<name>.md` path. Editing a file that does not exist creates it.
- `memory_view` lists the user's files.
- Rendering shows each file with its size and the memory's total use.
- Background consolidation reads the user's files as they are when it runs. It may add to them, create new files, and archive older text. Extraction jobs no longer carry a slot snapshot.
- Memory is on for an Agent when its memory switch is on. It no longer also requires at least one enabled block.

### 4. Configuration and API

- Drafts and published snapshots no longer carry blocks. `agent_memory_slots` stays in snapshots as an empty list, so readers of older versions keep working. The memory config revision covers only the two switches.
- Removed:
  - the console routes `/agents/:agent_id/memory/blocks` and `/memory/slots`;
  - the builder AI tool `replace_agent_memory_blocks`, together with its `replace_agent_memory_slots` alias;
  - the block-definition writes added by ADR 0016 step 5.
- The builder AI sets instructions through `update_agent_config.agent_memory_instructions`. Earlier conversations that called the removed tool still render.
- External, WebApp, and profile memory APIs keep their paths and fields. `values[]` lists the user's files, with `key` as the file name, and writing a new key creates a file.

### 5. Existing data

No data migration is needed: each block's content is already stored under its key, and becomes a file of the same name. The model can merge, split, or rename files later. `memory_block_definitions` keeps its rows, and items keep their `definition_id`, but nothing new is written there.

## Consequences

- Builders describe intent instead of designing a schema, and Agents and DigitalHumans now configure memory the same way.
- File names vary between users and models. Consumers of the memory APIs should treat `key` as a label, not a fixed field.
- Content of blocks a builder had disabled or removed becomes visible to the model again, because it is now just a file of the user's. Users could already see, export, and delete it.
- Contact details are always rejected. Before this change, a block described as holding them could keep them.
