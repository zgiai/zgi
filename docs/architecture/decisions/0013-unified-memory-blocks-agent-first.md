# ADR 0013: Unified Memory — Blocks, Archive, Recall, and Consolidation (Agent First)

- Status: Accepted
- Date: 2026-10-25
- Builds on: ADR 0002, ADR 0004, ADR 0006
- Supersedes (when complete): the slot model in `docs/agent-memory.md`, the separate tool and prompt sets of each memory store, and AIChat account memory's planner.
- Amends (phase 2): the scopes and trust levels of ADR 0004 and the placement of instance memory in ADR 0006. Memory ownership in ADR 0002, ADR 0004, ADR 0006, and `docs/user-relationship-memory.md` is unchanged.
- Amended by: ADR 0016 (the slot runtime and both rollout switches are retired; Agents always use blocks on the relationship store)

## Context

ZGI has four memory stores, each with its own schema, prompts, limits, and UI:

| Store | Scope | Write | Read |
|---|---|---|---|
| Agent memory (slots) | Agent × user | Inline tool `mutate_user_relationship_memory` plus an idle background extractor | Every value dumped into context, at most ~2,048 tokens |
| User relationship memory | User × Agent or DigitalHuman | Same tool family; Agents use `slots`, DigitalHumans use `dynamic` keys with four categories | Dumped into context, 4,000 characters |
| DigitalHuman-owned memory | DigitalHuman, with `self` and business scopes (ADR 0002, 0004, 0006) | Inline tool plus background proposer, gated by source trust | Not preloaded; the model must call a retrieval tool |
| AIChat account memory | Account | A synchronous planner LLM call before every answer | Rendered by category caps |

The Agent slot model, which this ADR changes first, has these problems:

- **Coarse values.** A slot is one free-text blob of at most 2,000 characters, and an Agent has at most five. Every write replaces the whole blob, so a model can silently drop facts while "merging". There is no per-fact provenance, time, or confidence.
- **Little control for builders.** The extraction and tool-policy prompts are hard-coded Go strings. A builder's only lever is a 200-character slot description. Templates are three hard-coded frontend presets. Slot keys cannot change after creation.
- **Brittle extraction.** Evidence must be an exact substring of the current user message, so paraphrases and confirmations of the assistant's suggestion ("yes, that one") are rejected. One invalid operation fails the whole background batch. The background extractor never sees assistant text. The sensitive-content regex rejects any 11-digit number.
- **Off in practice.** Automatic capture needs two global switches and a per-relationship setting, and the builder switch is hidden in the UI. Agent memory is effectively explicit-only.
- **No upkeep.** There is no consolidation, decay, or expiry. When values exceed the render budget, the remainder is dropped silently.
- **Poor explainability.** End users see a source kind and a time, not the words the memory came from.

The other three stores repeat several of these problems in different shapes. Builders and end users meet a different memory concept on each surface, and we maintain four implementations.

### Reference: Letta

Letta (formerly MemGPT) structures agent memory as:

- **Memory blocks** (core memory). Small, always in context, rendered with their `description` and `chars_current`/`chars_limit`. Each block has a `label`, a `description` (the main guidance for how the agent reads and writes it), a `value`, a character `limit`, and a `read_only` flag.
- **Archival memory.** A tagged vector store that is searched on demand.
- **Recall.** Persisted conversation history that can be searched.
- **Fine-grained editing tools.**
  - `memory_insert` appends.
  - `memory_replace` makes targeted edits.
  - `memory_rethink` rewrites a whole block. Letta advises one designated owner for rewrites.
  - `archival_memory_insert` and `archival_memory_search` work on archival memory.
- **Background consolidation.** First called sleep-time agents, now "dreaming": background subagents review recent conversations and update memory, either after a number of steps or when context is compacted.
- **Shared and read-only blocks** across agents, and a per-user `human` block.
- **Versioned memory.** Letta is now moving memory onto MemFS, a git-backed memory filesystem.

We adopt the structure: three tiers, description-driven blocks, fine-grained edits, background consolidation, and versioning. We do not adopt Letta's single-tenant assumption that an agent may rewrite anything at any time. ZGI serves many external end users per Agent. It must keep:

- source-trust admission (ADR 0004);
- evidence for automatic writes;
- end-user ownership, export, and erasure of personal memory;
- the rule that injected memory is untrusted context.

### Reference: memory as files

Letta's MemFS and Anthropic's memory tool both present memory as files. Anthropic's tool is a `/memories` directory edited with `view`, `create`, `str_replace`, `insert`, `delete`, and `rename`. The file shape has two parts that can be taken separately.

**The interface.** We take this part.

- Models are trained heavily on listing directories, reading files, and making exact-substring edits. They use these more reliably than bespoke upsert tools.
- One path namespace serves both small always-loaded files and larger on-demand ones.
- Document-like memory, such as procedures, project notes, and playbooks, can grow without fixed fields.

**The storage.** A git-backed tree gives commits, diffs, and rollback for free, but it does not fit ZGI, so we leave this part:

- One repository per external end user would mean a very large number of tiny repositories.
- Append-only history conflicts with permanent deletion and instance erasure.
- We would have to rebuild these over paths and arbitrary diffs:
  - per-value encryption and memory epochs;
  - trust admission and per-block write policy;
  - per-change evidence.
- Merging free text between inline writes and consolidation still needs a single writer.
- Semantic search still needs a vector projection.

So we keep storage in PostgreSQL and model versions as commits in a table. The rest of this ADR follows from that split.

## Decision

### One memory model for every consumer

Every consumer (Agent, DigitalHuman, AIChat account) uses the same concepts:

| Concept | Meaning |
|---|---|
| **Block definition** | Declared by the consumer's configuration and frozen in its published version. It defines a block's label, description, limit, partition, owner, audience, and write policy. |
| **Block** | The value of one definition in one partition instance, such as one end user's `preferences`. It is always injected into context. |
| **Archive entry** | One itemized memory with tags, evidence, confidence, and optional expiry. It is vector-indexed and recalled by relevance. |
| **Recall** | Search over the consumer's own persisted conversation messages. Nothing new is stored. |
| **Consolidation** | A background pass that rewrites blocks, deduplicates and expires entries, and moves block overflow into the archive. |

### Partitions, owners, and audiences

Isolation today uses five scopes (`self`, `workspace`, `channel`, `instance`, and the separate relationship store). ADR 0004 also defines `project`, `customer`, and `campaign` scopes that are never resolved at runtime. Most of these distinctions are classification, not safety. We keep the safety rules and collapse the classification.

**Three partitions.** Each partition is one directory in the model's view.

| Partition | Directory | What it holds | One per |
|---|---|---|---|
| `shared` | `/me/` | The consumer itself: persona, experience, decisions, business knowledge | Consumer |
| `conversation` | `/conversation/` | Context of one channel conversation group | Channel scope (DigitalHuman only) |
| `person` | `/user/` | Everything about one person: their facts and preferences, and the consumer's commitments and open loops with them | User subject × consumer |

**File attributes.** These carry the distinctions the old scopes encoded.

| Attribute | Values | Meaning |
|---|---|---|
| `owner` | `user`, `consumer` | Who owns the data and holds the rights. All `shared` and `conversation` files are `consumer`. In `person`, personal facts are `user` and the consumer's commitments are `consumer`. |
| `audience` | `everyone`, `members` | `shared` files only. `members` files are never readable by external end users. |
| `read_only` | true, false | The model can read but never write the file. |

**Mapping from today:**

| Today | Becomes |
|---|---|
| DigitalHuman `self` memory | `shared`, `audience: everyone` |
| `workspace` business memory | `shared`, `audience: members` |
| `channel` business memory | `conversation` |
| `instance` memory (ADR 0006) | `person`, `owner: consumer` |
| User relationship memory, Agent memory | `person`, `owner: user` |
| `project`, `customer`, `campaign` scopes | Removed; never resolved at runtime |

**Three trust tiers** replace the current source-trust levels for writing:

| Tier | Today |
|---|---|
| `operator` | `authorized_operator` |
| `member` | Workspace members and `channel_participant` |
| `external` | `external_end_user` |

### Rules that do not simplify

These are enforced by the validator, not only by prompts. They carry today's guarantees from ADR 0002, ADR 0004, ADR 0006, and `docs/user-relationship-memory.md`.

1. **Owners never mix.**
   - No tool, consolidation pass, or migration copies, moves, or promotes content between `owner: user` and `owner: consumer` files.
   - The only promotion that remains is an explicit operator action: moving a consumer-owned `person` file's content into `shared`. This replaces today's `instance` → `self` promotion and is never done by a model.
2. **Personal facts never leave the person partition.** `shared` and `conversation` are read by other people, so a personal fact written there leaks.
3. **Writing `shared` requires trust.**

   | Tier | May write `shared` |
   |---|---|
   | `operator` | Any file |
   | `member` | Only files whose definition allows member writes; by default `audience: members` files |
   | `external` | Never |

4. **Who writes the other partitions.**
   - `conversation` is written only by `member` or `operator` runs in that conversation.
   - A `person` partition is written only by runs for that person. Its `owner: user` files also follow the relationship-memory surface policy.
5. **Unverified identity gets no person partition.** Channel, scheduled-task, and AIChat runs never see or write `/user/`, as today.
6. **Partitions of the same kind are isolated.** A run sees one `conversation` and one `person` at most: its own.
7. **One owner per commit.**
   - A commit touches files of exactly one owner.
   - Rollback, export, and erasure of one owner never involve the other's data.
   - A consolidation pass with work for both owners produces two commits.
8. **Rights follow the owner.**
   - `owner: user` files: the user views, edits, exports, deletes, and rolls back. Builders and operators do not read the values.
   - `owner: consumer` files in `person`: operators manage them. The end user can view them as "what this assistant noted about our work" but cannot edit them.
   - `shared` and `conversation` files are managed by operators.
   - Erasing a person removes the whole `person` partition, both owners, and leaves the identifier-only tombstone of ADR 0006.
9. **Storage stays per owner.**
   - `owner: user` files stay in `user_memory_relationships` / `user_memory_items`, with their encryption, epochs, and settings.
   - Consumer-owned files stay in the DigitalHuman memory tables until a later ADR decides otherwise.
   - Partitions and attributes are the shared model and the file interface over those tables. They are not one physical table.

### What the model sees

The model sees memory as a small file tree. Directories answer one question: **who is this about?** Me, this conversation, or this person.

```
/me/persona.md                 owner=consumer audience=everyone read_only
/me/playbook.md                owner=consumer audience=members
/conversation/context.md       owner=consumer
/user/preferences.md           owner=user
/user/open_loops.md            owner=consumer
```

Per surface, a run sees at most three directories:

| Surface | Directories |
|---|---|
| Agent (console, WebApp, API) | `/user/` |
| DigitalHuman, workspace member | `/me/` (all audiences), `/user/` |
| DigitalHuman, external end user | `/me/` (`audience: everyone`, read-only), `/user/` |
| DigitalHuman, channel | `/me/` (all audiences), `/conversation/` |

- **A block is a pinned file.**
  - Its path is `/<directory>/<label>.md`, and its definition decides the description, limit, owner, audience, and write policy.
  - Pinned files are always rendered into context. The model can edit their content but cannot create or delete them.
  - A run has at most 6 pinned files; default templates use 2 to 4.
- **Phase 2 adds unpinned files.**
  - Documents live under `/me/` directories that a definition marks as writable.
  - Archive entries are listed under `/<directory>/archive/` and keep their owner.
  - Unpinned files are read on demand and found through search.
- **Paths are projections, not storage.**
  - Each path resolves to a row in its partition instance.
  - Renaming a label changes the path. The value stays keyed by the definition ID.

**Few inline write points.** The conversation model writes mainly to `/user/` files:

- explicit "remember" and "forget" requests;
- proactive writes where `auto_write` allows them;
- the consumer-owned open-loops file.

`/me/` and `/conversation/` are written mainly by consolidation and operators, so the hard routing decisions happen in the background, not during the answer. Inline writes to them are limited to explicit operator requests.

**Wrong targets get a hint.** A write that breaks a rule above is rejected with a reason naming the file that fits, for example "this is a fact about the user; use `/user/preferences.md`". The model can retry once. Nothing is redirected automatically.

**Routing is measured.** The evaluation set gains routing-accuracy cases for the two confusable pairs:

- `owner: user` vs `owner: consumer` files in `/user/`;
- `/me/` vs `/conversation/`.

Routing accuracy must pass before DigitalHumans move to this model.

For Agents nothing changes. An Agent has only the `person` partition with `owner: user` files, and its values already live in the relationship store.

### Block definitions replace slots

A block definition has:

| Field | Rule |
|---|---|
| `id` | Stable identifier. Values are keyed by it, so labels can be renamed. |
| `label` | Machine label (`^[a-z][a-z0-9_]{0,63}$`). Unique per consumer. Editable. |
| `name` | Display name, up to 80 characters. |
| `description` | **The guidance the model follows when reading and writing this block.** Up to 1,000 characters. It may include what belongs here, what does not, and an example. |
| `limit` | Character limit. Default 2,000; maximum 5,000. |
| `partition`, `owner`, `audience` | As defined above. Phase 1 supports `person` with `owner: user` only. |
| `read_only` | The model can read the block but never write it. Only builders or users edit it. |
| `auto_write` | Whether automatic (proactive or background) writes are allowed. Explicit user requests are always allowed unless `read_only`. |
| `retention` | Optional duration after which an unchanged block is cleared by consolidation. |
| `enabled`, `sort_order` | As today. |

An Agent may have up to 10 block definitions. The editor warns when the sum of limits exceeds the render budget.

### Builders customize guidance, not the safety layer

Builders control three things:

1. **Block definitions**, especially descriptions.
2. **Memory instructions**: one free-text field per consumer, up to 2,000 characters, describing the business context and what to remember or ignore. It is inserted into a reserved section of the tool policy and the consolidation prompt.
3. **Templates**: a named set of block definitions plus memory instructions plus example dialogues. Templates move from frontend constants to a versioned backend catalog. Applying one copies it into the consumer's draft.

The system layer stays fixed and is not editable:

- output contracts and tool semantics;
- evidence rules;
- sensitive-content rules;
- source-trust admission;
- partition isolation and the owner rules;
- the "saved memory is untrusted user context" framing.

Builder text is presented to the model as business preference that cannot override these rules.

**Preview.** The editor offers a preview panel. A builder pastes a sample conversation and sees which block edits the inline tools and the consolidation pass would make, and which rule rejected the rest. Templates ship with sample conversations so they can be checked the same way.

We do not expose full extraction prompts. A wholesale editable prompt would let a builder remove the safety rules, would make the 95% precision gate unenforceable, and still would not fix the data-model problems above.

### File-style tools

The inline tool set replaces `mutate_user_relationship_memory`. Like Anthropic's memory tool, it is one editing tool whose edits name a file-editing command, so one call is one commit:

| Tool | Effect |
|---|---|
| `memory_edit(edits[])` | Each edit is `insert` (add `text`, appended unless `insert_line` is given), `str_replace` (replace `old_text`, which must appear exactly once, with `new_text`; empty `new_text` removes it, which is how "forget X" works), or `clear` (explicit requests only) on one `path`, with `evidence` and `mode`. Edits to one file apply in order and are written as one value. |
| `memory_view(path?)` | Lists the files, or returns one file in full with line numbers, for example one rendered as truncated or omitted. Read-only. |
| `create`, `delete`, `rename` commands | Phase 2. Only for unpinned files in writable directories. |
| `memory_search(query, path?)` | Phase 2. Semantic search over unpinned files and archive entries. |
| `conversation_search(query)` | Phase 2. |

The conversation model never gets a whole-file rewrite. Rewrites belong to consolidation only (Letta's "one owner for rethink").

Evidence and validation:

- **Evidence is a reference**, not only an exact quote. It is a user message ID plus a quoted span from that message.
  - The span may come from any user message in the current conversation window, not only the latest.
  - The assistant message immediately before it may be cited as context, so a confirmation like "yes, that one" can resolve to the assistant's proposal.
  - Automatic writes still require a user-sourced span.
- **Validation is per operation.** An invalid operation is rejected with a reason, and the rest of the batch still applies.
- **The sensitive-content check classifies before it blocks.** Credentials and secrets are always rejected. Contact data such as phone numbers is allowed only in a block whose description explicitly permits it, so a support Agent can keep a callback number.

### Consolidation replaces extraction workers

The existing idle-time background job becomes a consolidation pass:

- **Triggers:** the existing idle delay (about 1 minute, forced by 10 minutes), and in later phases also every N turns or on context compaction.
- **Input:** recent turns, both user and assistant (assistant text is context only, never evidence), current blocks with their limits, the consumer's memory instructions, and later relevant archive entries.
- **Output:** per-file operations (`insert`, `str_replace`, `rewrite`), plus later `create`, `delete`, and `expire` for unpinned files and archive entries.
- **Rewrite:** only consolidation may `rewrite` a file, and only within its limit. A rewrite keeps the facts it does not intentionally drop, and its commit records exactly what changed.
- **Model:** configurable per deployment with `ZGI_MEMORY_CONSOLIDATION_MODEL` (`provider/model`); requests carry the `memory-consolidation` use case for accounting. When it is unset, the conversation's model is used.
- **Fences:** the existing deletion cutoffs, memory epochs, config revisions, and inline deduplication stay.

### Versions are commits

Versions follow the shape of commits, stored in tables rather than a git repository.

- **Commit.** Every write batch creates one commit row: one inline tool call, one consolidation pass, or one builder or user edit. It records:
  - the source: explicit, automatic, consolidation, builder, or user;
  - the evidence reference;
  - a short message giving the reason.
- **File version.** Each changed file gets a version row under the commit, holding the prior and new value.
- **History and rollback.**
  - Users and builders can see the history per file or per commit, and diff any two versions.
  - They can roll back a file or a whole commit. Rolling back a consolidation commit undoes all of its edits together.
- **Undo.** The current 24-hour undo becomes "roll back the previous commit", available for as long as versions are retained.
- **Retention.** Version content is kept for 30 days; commit and version records follow the existing audit window (180 days). Clearing all memory purges the relationship's version content immediately, and permanent deletion and erasure remove the history with the relationship.
- **Encryption.** Version content uses the same encryption as values.

### Rendering and budget

Pinned files are rendered as:

```
<memory>
  <file path="/user/preferences.md" owner="user" chars="…/limit" read_only="…">
    <description>…</description>
    <content>…</content>
  </file>
</memory>
```

From phase 2, the section ends with a one-line listing of unpinned directories and their file counts, so the model knows what it can `memory_view` or `memory_search`.

The rendered section sits inside the existing untrusted-context framing. The render budget becomes the smaller of 10% of the model's input limit and 8,000 characters. When it is exceeded:

- Blocks are kept in definition order.
- The last block that does not fit is truncated with an explicit marker; it is not dropped silently.
- A consolidation run is scheduled to shrink the overflow, and in phase 2 to move it into the archive.

Rendering per partition:

- **`person` files** keep the untrusted-user-context framing. They are injected as today.
- **`shared` and `conversation` files** are retrieval-first, as DigitalHuman memory is today. They are unpinned unless a builder explicitly pins a small file such as a persona.
- **Separate sections.** `person` files and consumer files are never rendered in the same section, so a model reading context can always tell personal memory from the consumer's own.

## Phase 1: Agent

The Agent moves first. It has the smallest blast radius and the most constrained model today.

### Storage

We keep the relationship store and extend it; we do not create a parallel one.

- **Block definitions.** A new `memory_block_definitions` table, with owner `consumer_type` + `consumer_id` and no workspace column (consumers can move between workspaces), holds the fields above. A definition's ID is stable per consumer and label: removing a slot marks its definition deleted, and a slot that returns under the same label (including after a published-version rollback, which recreates slots) gets the same ID back. The slot editor keeps definitions in sync in the same transaction.
  - `agent_memory_slots` rows are migrated one-to-one:
    - `key` → `label`;
    - `max_chars` → `limit`;
    - `enabled` and `sort_order` carry over;
    - `partition` = `person`, `owner` = `user`;
    - `auto_write` = the Agent's current automatic-capture setting.
  - Published versions gain contract version 2 with definition IDs.
  - Version-1 snapshots keep loading through an adapter until republished.
- **Values.** `user_memory_items` stays the value table.
  - It gains a nullable `definition_id`. For slot consumers this is backfilled from `(consumer, memory_key)`.
  - Values are then looked up by `definition_id`, which makes labels renamable.
  - DigitalHuman `dynamic` items keep using `memory_key` until phase 2.
- **Versions.** New `memory_commits` and `memory_file_versions` tables, both removed with their relationship (`ON DELETE CASCADE`), so permanent deletion, end-user erasure, and workspace teardown leave no history behind even when they delete rows directly. Existing values get no backfilled version: every version row keeps the content before and after its change, so history starts with the first change.
- **Container.** `user_memory_relationships` remains the `person` partition instance for `owner: user` files, with its settings, epoch, cutoff, and status.

### Runtime

- **Tools.** `memory_edit` and `memory_view` replace `mutate_user_relationship_memory` for Agents. Paths are limited to `/user/<label>.md`.
  - The explicit-request fast path, the 2-second timeout, and the single retry stay.
  - For models without verified function calling, the existing fallback stays: they chat normally and cannot claim a write.
- **Consolidation.**
  - `agentmemoryworker` becomes the consolidation pass described above, using the new `memory-consolidation` model use case.
  - Inputs widen to user plus assistant turns.
  - Validation is per operation.
- **Rendering.** The new block rendering and budget apply, still injected as untrusted user-role context at the same position.

### Product

- **Builder.** The Agent memory section becomes a block editor:
  - label, name, description with an inline example, limit, `read_only`, `auto_write`, retention;
  - an Agent-level memory instructions field;
  - the template catalog;
  - the preview panel.
  - The hidden auto-maintenance switch is replaced by per-block `auto_write`.
  - As built in step 3, two parts differ:
    - The Agent-level automatic-memory switch stays as the gate, shown only where memory blocks are on, with per-block `auto_write` under it. A per-block `auto_write: true` therefore cannot turn automatic writes on for an Agent that never enabled them.
    - Label renames and `retention` are deferred. Labels stay fixed after creation until values are looked up by `definition_id`.
  - The preview runs the consolidation pass only. Inline `memory_edit` calls happen inside a live chat, so the editor's chat preview already shows them.
- **End user.** WebApp "My Memory", `/profile/memory`, and `/api/v1/agents/memory` show blocks with history:
  - each commit shows its source quote and reason;
  - users can edit, clear, export, and roll back.
  - The external API keeps its existing paths and `user` parameter. Responses add `definition_id`, `label`, `path`, `limit`, and `commits`, and the old fields stay.

### Rollout

- **Switch.** A new fail-closed switch `ZGI_MEMORY_BLOCKS_ENABLED` selects the block runtime for Agents.
  - While it is off, migrated data is still served through the current slot runtime, so we can roll back without losing data.
- **Auto-write gate.** The existing switches keep gating automatic writes.
  - The release gate is unchanged: at least 95% automatic-write precision in the bilingual evaluation, and 100% rejection of secrets, assistant-only content, and illegal clears.
  - The evaluation set gains paraphrase, confirmation, cross-turn, and contact-number cases.
- **Migration checks.** Migration is idempotent and verified by row counts and checksums per Agent before the switch is enabled in a test environment.

## Later phases

These are summarized here and detailed in follow-up ADRs.

- **Phase 2: Archive, documents, and recall.**
  - Add archive entries under `/archive/` and `conversation_search`.
  - Add unpinned document files under `/me/` for the DigitalHuman's own memory and team knowledge.
  - Add `memory_create`, `memory_delete`, `memory_rename`, and `memory_search`.
  - Move DigitalHumans to the three partitions: `self` and `workspace` memory become `shared` files (with `audience`), `channel` memory becomes `conversation`, and `instance` memory becomes consumer-owned `person` files. Types become tags. Storage stays in the DigitalHuman tables; only the projection and the rules change.
  - Amend ADR 0004 (scopes and trust levels) and ADR 0006 (where instance memory lives in the model) accordingly, and remove the unused `project`, `customer`, and `campaign` scopes.
  - Move DigitalHuman `dynamic` relationship items into `owner: user` blocks in `person`, plus `owner: user` archive entries for overflow. They stay in the relationship store, never in consumer-owned files.
  - Preload the top-k relevant archive entries at run start. The model may still search explicitly, so recall no longer depends only on the model calling a tool.
- **Phase 3: Consolidation everywhere.**
  - DigitalHuman and AIChat use the consolidation pass.
  - The AIChat per-turn synchronous planner call moves off the answer path.
  - Retention and expiry apply to instance memory and the archive.
  - Automatic capture defaults on per consumer once the evaluation gate passes.
- **Phase 4: Convergence.**
  - AIChat account memory becomes a consumer of its own, with `owner: user` blocks in its `person` partition.
  - Legacy tables and APIs are retired.
  - `shared` read-only files (for example a team glossary) can be attached to several consumers.
  - Version history UI everywhere.

## Consequences

- One memory concept, one tool vocabulary, and one management UI across Agents, DigitalHumans, and AIChat.
- Builders get real control through block descriptions, memory instructions, and templates, with a preview to check the effect. The safety layer stays ours.
- Fine-grained edits and versions remove silent fact loss and make every change explainable and reversible.
- Models get the file-editing interface they handle best, without git storage and its conflicts with erasure, encryption, and multi-tenant scale.
- Costs:
  - another migration of Agent values;
  - a new definitions table plus commit and file-version tables;
  - more tool definitions in context;
  - a consolidation model call per idle conversation, as today but on a cheaper model.

### Risks

- **Weak tool-calling models** may misuse `str_replace` edits or browse unpinned files poorly. Mitigation:
  - per-model capability flags that fall back to `insert` edits only, leaving restructuring to consolidation;
  - from phase 2, preloading the top-k relevant unpinned content so recall does not depend on the model navigating the tree.
- **Memory poisoning through builder text or user content.** Mitigation: the fixed system layer, untrusted-context framing, and trust admission. Builder instructions are evaluated in the preview, and they cannot override evidence or sensitivity rules.
- **Concurrent writes.** Mitigation: revision CAS per file, as today. Rewrites are only allowed for consolidation, and they are serialized per partition instance. Free-text merging is never attempted.
- **Blurring personal and DigitalHuman memory.** One tree and one consolidation pass make it easy to write a personal fact into DigitalHuman memory, or the reverse. Mitigation:
  - the rules that do not simplify, enforced in the validator, not only in prompts;
  - few inline write points, hints on wrong targets, and the routing-accuracy gate;
  - tests that every cross-owner write, copy, and commit is rejected.
- **Tree drift.** Unpinned files can sprawl. Mitigation: consolidation owns restructuring, and writable directories have per-partition file and size caps.
- **Asserted external identity.** The external API `user` value is still asserted by the integration. Unifying stores widens what that identity reaches, so whether to require signed end-user identity is an open question, linked to ADR 0010.

## Open questions

1. Should phase 1 also allow `shared` (Agent-wide) files for persona and read-only knowledge, or keep those in the Agent's system prompt until phase 4?
2. What are the defaults for the consolidation model per deployment, and what is the per-workspace spend cap?
3. Should the external memory API also expose the file view (paths and commits) as its primary shape, or keep the block shape with paths as extra fields?
