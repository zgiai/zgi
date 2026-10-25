# ADR 0014: DigitalHuman Memory as Files, Recall, and Archive (Unified Memory Phase 2)

- Status: Accepted
- Date: 2026-10-25
- Builds on: ADR 0013 (phase 1 shipped for Agents), ADR 0002, ADR 0004, ADR 0006
- Amends: ADR 0004 (scopes and trust levels) and ADR 0006 (where instance memory sits in the model's view). Ownership and storage in ADR 0002, ADR 0004, and ADR 0006 do not change.

## Context

ADR 0013 phase 1 moved Agent memory to blocks: pinned files, `memory_edit`/`memory_view`, background consolidation, builder controls, and end-user history with revert. Phase 2 is only sketched there. This ADR details it.

How DigitalHuman memory works today:

- **Storage.** `digital_human_memories` holds every entry with a `memory_class` (`self`, `business`), a `scope_type` (workspace, channel, instance, and the never-resolved project, customer, campaign), a `memory_type` (identity, experience, decision, fact, open_loop, reflection), a key, content, revision, and evidence. Vectors live in a hidden per-DigitalHuman dataset, synced on every recall.
- **Writes.** One tool, `manage_digital_human_memory(action, mode, key, memory_type, content, evidence, …)`. The server picks class and scope from the run's source trust (`admitMemoryWrite`): operators write `self`, channel participants write their channel (automatic only), external end users write their own instance, workspace members are rejected. A background extraction worker writes the same targets.
- **Reads.** Nothing of the DigitalHuman's own memory is preloaded. The run gets a hint to call `retrieve_digital_human_memory(query)`, a vector search filtered to the run's readable scopes. Recall therefore depends on the model choosing to call it.
- **Relationship memory** (`owner: user`) for DigitalHumans uses `dynamic` items (free keys in four categories, at most 30), preloaded up to 4,000 characters, written with `mutate_user_relationship_memory`. There is no background extraction for it.
- **No recall of past conversations.** Conversation search exists for the UI (SQL `LIKE` over titles, queries, and answers) but not as a model tool, for DigitalHumans or Agents.
- **Dead scopes.** `project`, `customer`, and `campaign` exist in the schema, the enums, and the proposals API, but are never resolved at runtime.

So DigitalHumans have three tool sets, two retrieval styles, a recall that only happens when the model asks, and no way to look back at earlier conversations.

## Decision

### 1. One file view over the existing tables

Storage does not move. Each readable entry is projected to a path, and the model works with paths only.

| Today | Path | Owner | Audience | Default rendering |
|---|---|---|---|---|
| `self` memory | `/me/<key>.md` | consumer | everyone | unpinned |
| `business/workspace` memory | `/me/team/<key>.md` | consumer | members | unpinned |
| `business/channel` memory | `/conversation/<key>.md` | consumer | — | unpinned |
| `business/instance` memory (ADR 0006) | `/user/notes/<key>.md` | consumer | — | unpinned |
| Relationship `dynamic` items | `/user/<key>.md` | user | — | pinned (as today) |

- **Types become tags.** `memory_type` is shown as `tags="decision"` and can be set on create. It no longer routes anything.
- **Two `/user/` namespaces keep owners apart.** They hold different things about the same person:
  - `/user/` is the user's own facts and preferences ("allergic to peanuts", "keep answers short"). The user views, edits, exports, deletes, and rolls it back; operators cannot read it.
  - `/user/notes/` is the DigitalHuman's record of its work with that person ("promised a quote by Friday", "their company is evaluating the Enterprise plan"). Operators manage it and may promote it into `/me/`; the user can view it but not edit it. It exists only for external end users (ADR 0006).
  - Erasing the person removes both.

  Details: `/user/<key>.md` is always the user's (relationship store). `/user/notes/` holds what the DigitalHuman noted about its work with this person (instance memory). A key can never be both, so rule 1 of ADR 0013 (owners never mix) is a property of the path.
- **Visibility follows the run**, exactly as `memoryReadScopeForRun` does today. Per ADR 0013, an external end user sees `/me/` (everyone) and their own `/user/`; a member sees `/me/` including `/me/team/` and their `/user/` when the identity is verified; a channel run sees `/me/` and its own `/conversation/`. Paths outside the run's view do not exist for that run.
- **Identity files** (`memory_type = identity`) render with `read_only="true"`. They still need operator approval to change.

### 2. Recall no longer depends on a tool call

At run start the DigitalHuman memory section contains, in this order, within one budget (default 6,000 characters):

1. **Pinned files**: the user's relationship items, as today.
2. **Recalled files**: the top 5 files most relevant to the current message, found by the same vector search that `retrieve_digital_human_memory` runs today, rendered in full.
3. **Index**: one line per other readable file (path, tags, first line cut to 80 characters), newest first, until the budget is used, followed by a count of files left out.

The model can read any listed file with `memory_view`, and search further with `memory_search`. When the vector backend is down, recall falls back to the newest files, as retrieval does today, and says so in the section.

### 3. File tools for DigitalHumans

Behind the switch in §8, the DigitalHuman tool set becomes the phase 1 file tools, extended:

| Tool | Effect |
|---|---|
| `memory_view(path?)` | Lists the run's directories with file counts, or returns one file with line numbers. |
| `memory_search(query, path?)` | Semantic search over the run's readable files (and archive, §5), optionally within one directory. Returns paths, tags, and snippets. Replaces `retrieve_digital_human_memory`. |
| `memory_edit(edits[])` | Commands: `create` (path, text, tags), `insert`, `str_replace`, `clear`, `rename` (path, new_path, same directory only), `delete`. Each edit carries `evidence` and `mode` and is checked on its own. Replaces `manage_digital_human_memory` and, for DigitalHumans, `mutate_user_relationship_memory`. |
| `conversation_search(query)` | §4. |

**The path decides the target, and admission decides whether the path is allowed.** `admitMemoryWrite` keeps returning the one class and scope a run may write. A write to any other directory is rejected with a hint that names the allowed directory, as ADR 0013 specifies. No new write permission is granted by this ADR:

| Trust tier (ADR 0013) | Today's level | May write |
|---|---|---|
| operator | `authorized_operator` | `/me/` (explicit always, automatic when enabled), its own `/user/` |
| member | `workspace_member` | its own `/user/` (relationship rules) |
| member | `channel_participant` | `/conversation/` (automatic only, as today) |
| external | `external_end_user` | `/user/notes/` (instance rules) and its own `/user/` |

`/me/team/` stays operator-managed through the console, as today.

Validation keeps every existing check: evidence from the user's messages (phase 1 rules: any user message in the window, quotes normalized), the sensitive-content policy, revision fences, instance capacity, idempotent operation IDs, and the audit events and undo records of each store. `rename` is a new operation on the DigitalHuman store: it changes `memory_key` under a revision check and records an event. It is offered only to operators and only inside one directory.

### 4. Conversation search

`conversation_search(query, limit?)` returns earlier messages of conversations the run is allowed to see, with conversation ID, time, and a snippet of the user message and answer:

| Run | Searches |
|---|---|
| DigitalHuman, verified person (console member, WebApp, API end user) | That person's conversations with this DigitalHuman |
| DigitalHuman, channel | Every group conversation of this DigitalHuman across its channels |
| DigitalHuman, scheduled task | Nothing (the tool is not offered) |
| Agent (console, WebApp, API) | That user's conversations with this Agent |

A DigitalHuman is one individual, so everything said in the groups it has joined belongs to its own memory: a channel run may recall another group's discussion, the way a person remembers what was said in every meeting they attended. Direct (one-to-one) channel chats are excluded, because they belong to the person in them, not to the groups. `/conversation/` memory stays per conversation (ADR 0013 rule 6); only search spans groups.

It reuses the existing conversation search (`LIKE` over titles, queries, and answers), scoped server-side by the run's subject and consumer, and never crosses users. A semantic index over conversations is out of scope here.

### 5. Archive

The archive is memory that is kept and searchable but not listed in the index.

- **Consumer-owned archive**: DigitalHuman entries with `status = archived` (today: instance entries past their TTL). They appear under `/<directory>/archive/` in `memory_search` results only.
- **User-owned archive**: a new table, `user_memory_archive_entries`, cascading with its relationship and using the relationship store's content codec, never preloaded, found by `memory_search`. A separate table keeps an append-only, growing, search-oriented store out of the small set of resident items, so item limits, rendering, export, and slot reads need no exclusions. Versions use the existing history tables with keys `archive/<id>`. Two writers fill it:
  - Agent consolidation moves overflow there instead of only truncating a block, as ADR 0013 planned.
  - DigitalHuman relationship items beyond the 30-item limit go there instead of being rejected.
- The user sees, edits, exports, and deletes archive entries like other relationship items, with history.

### 6. Scope cleanup

- `project`, `customer`, and `campaign` are removed from the enums, the proposals API, and the check constraint. A migration first moves any existing rows with those scopes to `business/workspace` with an audit event, so nothing is lost.
- ADR 0004 is amended to the three tiers above, and ADR 0006 to say that instance memory is shown as `/user/notes/`. Their rules do not change.

### 7. What does not change in this phase

- Storage per owner, encryption, epochs, erasure, export, and the end-user tombstone.
- The DigitalHuman extraction worker. Moving it to consolidation is phase 3.
- Operator console APIs. They gain paths and tags in responses; their requests stay the same.

### 8. Rollout

- A fail-closed switch, `ZGI_DIGITAL_HUMAN_MEMORY_FILES_ENABLED`, selects the file view and tools for DigitalHumans. With it off, the current tools and rendering stay.
- **Evaluation before the switch.** The phase 1 suite gains DigitalHuman scenarios run through the DigitalHuman external API: recall without asking, routing between `/user/` and `/user/notes/`, refusal of other directories, conversation search, and isolation between end users. A baseline is recorded on the current tools first, as in phase 1.
- Routing accuracy for the two confusable pairs (ADR 0013) must pass before the switch is enabled in a test environment.

## Steps

1. **Evaluation and baseline.** DigitalHuman scenarios in `api/evals/memory`, run against the current tools.
2. **Read path.** Path projection, recalled files and index at run start, `memory_view`, `memory_search`.
3. **Write path.** `memory_edit` for DigitalHumans with create, rename, and delete, path admission and hints, and the store operations it needs.
4. **Conversation search** for DigitalHumans and Agents.
5. **Archive.** The user-owned archive kind, Agent overflow, and DigitalHuman item overflow, plus archive results in `memory_search`.
6. **Scope cleanup** migration and the ADR 0004 and ADR 0006 amendments.
7. **Console and end-user views** show paths and tags; rerun the evaluation and compare.

## Consequences

- DigitalHumans and Agents share one memory interface. The model sees the same tools and file framing, and DigitalHuman recall happens without a tool call.
- Earlier conversations become reachable for both, inside each user's own boundary.
- Ownership stays where it is: the path encodes it, and admission still decides every write.
- Cost: the run-start search adds one vector query per DigitalHuman run, which today happens only when the model asks. The index and recalled files take up to 6,000 characters of context.

### Risks

- **Index noise.** A long index can crowd the context. Mitigation: the budget, newest-first ordering, and the count of omitted files.
- **Recall latency.** Projection sync runs on every search today. Mitigation: sync stays incremental, and recall at run start uses the existing fallback on timeout.
- **Wrong directory.** The model may try to write `/me/` from an end-user run. Mitigation: rejection with a hint, measured by the routing cases.
- **Conversation search leaks.** Mitigation: scope by subject and consumer on the server; isolation cases in the evaluation.

## Decisions on the open points

- Channel search spans every group conversation of the DigitalHuman (§4); direct chats are excluded.
- `/me/team/` stays operator-managed and read-only to the model. Member writes are not opened. Whether workspace memory is kept long term is revisited with production usage data in step 6.
- Recall starts at 5 files within 6,000 characters and is tuned by the evaluation.
