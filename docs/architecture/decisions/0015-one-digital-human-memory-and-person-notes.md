# ADR 0015: One DigitalHuman Memory, and Person Notes in Relationship Memory (Unified Memory Phase 3)

- Status: Accepted
- Date: 2026-10-26
- Builds on: ADR 0013, ADR 0014, ADR 0002, ADR 0004, ADR 0006
- Amends: ADR 0004 (business scopes and write admission), ADR 0006 (where instance memory is stored), ADR 0014 (directories `/me/team/` and `/conversation/`)
- Amended by: ADR 0019 (the person's own memory under `/user/` keeps no categories)

## Context

After ADR 0014, a DigitalHuman's memory has five places:

| Directory | Store | Written by | Read by |
|---|---|---|---|
| `/me/` | `digital_human_memories`, class `self` | operators | every run |
| `/me/team/` | same table, business/`workspace` | operators, by hand in the console only | every run except external end users |
| `/conversation/` | same table, business/`channel` | channel participants, automatically | that group or direct chat only |
| `/user/notes/` | same table, business/`instance` | external end users' runs | that end user only |
| `/user/` | relationship memory (`user_memory_*`) | the user and the model | that user only |

Three of them are now hard to justify:

- **`/me/team/` is almost unused.** It differs from `/me/` only in being hidden from external end users. That is an attribute of an entry, not a place.
- **`/conversation/` splits the DigitalHuman by group.** The product decision is that a DigitalHuman used directly (in the console or in channels) is one individual with one memory. Conversation search already spans every group (ADR 0014 §4).
- **`/user/notes/` duplicates the person partition in a second store.** Facts about a person live in relationship memory. Commitments the DigitalHuman made to that person live in `digital_human_memories`, with their own capacity, TTL, erasure, export, and operator views. They lack the history, revert, archive, and forgotten-line filtering that relationship memory gained in phases 1 and 2.

None of this is in production yet. Phase 1 and 2 migrations are unreleased, and the instance scope (on `main`) holds only local and test data. Changing the model now costs a migration of test data only. Later it would cost a migration of user data.

## Decision

### 1. One DigitalHuman memory: `/me/`, with visibility and origin

`digital_human_memories` holds only the DigitalHuman's own memory. Each entry gains two attributes:

- **`visibility`**: `everyone` or `internal`. An `internal` entry is never read by an external end user's run (API, WebApp) and never shown to them. The existing column is reused with these two values.
- **`origin`**: where the entry came from: `operator`, `member`, or `channel`, plus the channel conversation ID for `channel`. This lets operators filter, review, and bulk-undo, for example everything learned in one group.

Write admission by trust level (ADR 0004 levels; no new level):

| Trust level | Writes | Visibility |
|---|---|---|
| `authorized_operator` | `/me/` (explicit, and proactive when enabled) | `everyone` by default; may choose `internal` |
| `workspace_member` | `/me/` (explicit, and proactive when enabled) | `internal`, always |
| `channel_participant` (group and direct chats) | `/me/` (proactive when enabled, and background extraction) | `internal`, always |
| `external_end_user` | the person's notes (§2) and their own `/user/` | — |

Guards:

- **Only operators change what external users see.** A run below `authorized_operator` may create and edit `internal` entries only. An `everyone` entry is read-only to it: the edit is rejected with a hint, so a group member cannot rewrite what customers are told.
- **Operators promote.** An operator makes an `internal` entry `everyone` (or the reverse) in the console. This replaces "move to workspace memory" and rescope.
- **Identity stays operator-only.** Entries of type `identity` keep their current rules.
- **Background extraction** runs for channel and member runs and writes `/me/` `internal` entries. Operator runs stay explicit, as today.

Reads: every run except an external end user's reads all of `/me/`. An external end user's run reads `/me/` entries with `visibility = everyone`. The file view shows `audience="everyone"` or `audience="members"` and the origin on each file.

### 2. Person notes move into relationship memory

The DigitalHuman's notes on a person become items of that person's relationship memory with that DigitalHuman, kept by the DigitalHuman:

- **Store.** A note is a `user_memory_items` row whose key starts with `notes/`; its category is the note's memory type. `/user/<key>.md` is any other item; `/user/notes/<key>.md` is `notes/<key>`. The `notes/` namespace needs no new column: the person's own items can never use it, and notes do not count toward the relationship's item limit.
- **Who may change notes.**
  - The model writes them from external end-user runs, under today's evidence and mode rules.
  - Operators manage them in the Runtime Workspace end-user panel and may promote one into `/me/` as `internal`.
  - The person sees them read-only and may delete them, so the right to erasure covers them; they cannot edit them.
- **Limits.**
  - Capacity: `ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_MAX_ENTRIES` keeps its meaning per relationship.
  - TTL: `ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_TTL` moves an expired note into the person's archive (ADR 0014 §5) instead of a hidden status, so it stays searchable.
- **What notes gain** from the relationship store: version history with revert, the archive, forgotten-line filtering of conversation search, encryption, export, and erasure.
- **Who keeps notes.** Only external end users have notes, as in ADR 0006. Console members and channel participants share `/me/`.

This removes the instance scope from `digital_human_memories`. The end-user instance (ADR 0006) stays the operator's panel of an end user. Its memory section reads the relationship store.

### 3. The scope system is removed

With §1 and §2, every row of `digital_human_memories` is the DigitalHuman's own memory:

- **Removed:** the memory class and scope (`memory_class`, `scope_type`, `scope_id`) from code and contracts, the scope check, the scope-aware supersedes trigger, the instance index and erase allowance, and the read-scope predicate. Up migrations never drop columns, so the three columns stay behind as retired columns with defaults that no code reads or writes.
- **Unique key:** becomes (organization, workspace, DigitalHuman, key).
- **Contracts:** `memoryruntime` drops the class and scope types.
- **Operator API:** the proposals API loses `memory_class` and `scope`, and gains `visibility`.
- **Unreleased migrations** of phases 2 and 3 that changed scopes are replaced by one migration, `202610261200000000_digital_human_memory_without_scopes`.

The migration:

- moves `workspace`, `channel`, and the never-resolved `project`, `customer`, and `campaign` rows to `/me/` as `internal`, with their origin and an audit event each;
- moves active `instance` rows into the person's relationship memory as notes, creating the relationship when missing, archived ones into the person's archive, and drops invalidated ones; the moved rows leave with their events, undo records, and ledger rows;
- adds a suffix naming the old place when a key collides;
- leaves rows it cannot move (of a deleted DigitalHuman, or of an instance that no longer exists) as `internal` entries, so every check holds every row.

### 4. Memory pages

Memory pages are reworked for many entries:

- **Console, DigitalHuman memory** (`/me/` only):
  - a compact list grouped by type, with counts and collapsible groups;
  - filters for visibility, origin (operator, member, channel and which group), and status;
  - search;
  - bulk "make everyone / internal" and invalidate over selected entries (`POST /memory/bulk`); undo by origin is a filter by origin or channel, select all, and invalidate, and each change stays undoable on its own;
  - the detail opens in the existing side sheet.
- **Runtime Workspace end-user panel:** each end user's notes for each DigitalHuman, with delete and promote.
- **Profile page and WebApp "My Memory"** (one relationship at a time): tabs for Memory, Notes (DigitalHumans only), Archive, and History, instead of one long page. Items are compact rows with inline edit and search.

### 5. Memory instructions

Operators customize a DigitalHuman's memory the way builders customize an Agent's (ADR 0013): with guidance, never with the safety layer.

- **Store.** `memory_instructions` is one free-text field of up to 2,000 characters, kept as `instructions` in the version's `memory_policy`. It is published, versioned, and restored with the rest of the configuration, and needs no migration.
- **Use.** The text goes into a reserved section below the fixed rules in two places: the in-conversation memory guidance of runs that may write memory, and the background extraction prompt. It is framed as the operator's preference for what to keep, what to ignore, and where it belongs. It cannot override the rules on evidence, sensitive content, trust admission, or visibility.
- **Preview.** `POST /digital-humans/:id/memory/preview` runs the background proposer over a pasted sample conversation, with the draft's model and either the saved instructions or unsaved ones. It reports each proposal and whether the memory policy would accept it, and writes nothing.

### 6. What does not change

- Agents: phase 1 and 2 behaviour.
- The relationship store's rules for the person's own items (keys outside `notes/`).
- The external end-user instance, quotas, and erasure (ADR 0006, ADR 0010).
- Conversation search boundaries (ADR 0014 §4) and forgotten-line filtering.

## Steps

1. **ADR and evaluation.** New DigitalHuman scenarios run through the external API:
   - an operator's `internal` entry is never revealed to an external end user;
   - notes still route correctly after the move;
   - an expired note is found again through the archive.

   Member and channel writes are covered by integration tests, because the harness has no channel.
2. **One `/me/`:** visibility and origin, the write and read rules, the guard on `everyone` entries, the worker, the file view, vector metadata refresh on a visibility change, and console promotion.
3. **Notes in relationship memory:** the `notes/` namespace, the DigitalHuman note editor, capacity, TTL into the archive, operator panel and promotion, and the user's read-only view with delete.
4. **Scope removal and migration**, with the rewritten unreleased migrations and the amendments to ADR 0004, 0006, and 0014.
5. **Memory pages** (§4).
6. **Evaluation rerun** of the DigitalHuman and Agent suites, compared with phase 2.
7. **Memory instructions** (§5): the field, its use in runs and extraction, the preview, the configuration editor, and evaluation scenarios that run with and without instructions.

## Consequences

- Two stores, one per owner:
  - `digital_human_memories` is the DigitalHuman.
  - The relationship store is everything about a person: the person's own items, and the notes a consumer keeps on them under `notes/`.
- Group knowledge is shared across groups and the console, as the product intends. Exposure to customers needs an operator.
- **Risk: one group can mislead the DigitalHuman for other groups and the console.** Mitigations:
  - internal-only visibility, so customers are unaffected;
  - the origin filter with bulk undo;
  - the read-only guard on `everyone` entries.
- **Risk: conflicting statements from different groups overwrite each other.** Mitigations: origin and history on every entry; the model sees where an entry came from.
- Operators lose per-group memory. Facts that must not cross groups should not be stored as memory.
