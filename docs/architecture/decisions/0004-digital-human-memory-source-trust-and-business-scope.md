# ADR 0004: DigitalHuman Memory Source Trust and Business Scope

- Status: Accepted
- Date: 2026-10-19
- Accepted: 2026-10-21
- Amends: ADR 0002
- Amended by: ADR 0014 (business scopes are workspace, channel, and instance; trust levels group into three tiers)
- Amended by: ADR 0015 (business scopes are removed; memory has a visibility and an origin instead)

## Context

ADR 0002 makes long-term memory owned by the DigitalHuman rather than partitioned by user. That remains the intended model: a DigitalHuman is one subject that keeps its own continuity across collaborators and channels.

Two gaps remain in the current implementation:

- Write admission is keyed on the invocation origin kind. A private console conversation needs an authorized promotion before its content can become memory, but API, WebApp, and channel conversations can write `self` memory whenever automatic extraction is enabled. External end users therefore shape memory that every other caller later retrieves.
- The schema already supports `business` memory with `scope_type`, `scope_id`, and `restricted` visibility, but the runtime stores every proposal as `self` memory. There is no way to keep knowledge about one customer, project, or conversation out of unrelated runs.

## Decision

### Ownership is unchanged

Memory stays owned by `digital_human_id` within its organization and workspace. The initiating principal remains provenance only. Relationship memory stays a separate store.

### Every run has a source trust level

The server classifies each run once, at admission, from verified run data: surface, principal kind, and the actor's live workspace permission. Model output and client-supplied fields never influence it. The level is stored on the run and never raised during it.

- A console run is `authorized_operator` when the actor holds the DigitalHuman memory management permission, otherwise `workspace_member`.
- A WebApp or API run is `external_end_user`, including WebApp users who are also workspace members.
- A channel run is `channel_participant`. The channel installer's permissions are not used.
- A scheduled run inherits from its task's origin surface and is never `authorized_operator`.
- Runs admitted before classification existed derive their level from the surface in the same way, except that console runs resolve to `workspace_member`.

| Trust level | Typical source | Memory write admission |
|---|---|---|
| `authorized_operator` | Workspace member with memory promotion permission | Explicit and proactive `self` writes in the conversation; never background extraction |
| `workspace_member` | Console conversation without promotion permission | Proposals only; `self` writes require promotion |
| `channel_participant` | Bound channel direct or group conversation | Writes land in the conversation's `channel` business scope; `self` writes require promotion |
| `external_end_user` | External API and public WebApp | Never writes DigitalHuman memory |

Durable facts about an external end user belong to relationship memory. A later decision on per-user instances may give external end users a memory partition of their own; it does not relax this table.

One admission function applies this policy to both inline memory tools and the background extraction worker. Background extraction is queued only for channel participants. Console conversations stay excluded from background extraction even for operators, so their memory changes are always explicit. Rejections are recorded with a reason code and the trust level.

### Business scope is resolved by the server

Each run receives a resolved set of readable and writable business scopes. The scopes come from verified bindings, not from the model:

- a channel conversation resolves to its `channel` scope;
- a scheduled task resolves to the scope recorded on that task;
- project, customer, and campaign scopes resolve from the DigitalHuman's configured resource bindings and the run's context. *(Retired by ADR 0014; see the amendment below.)*

The memory tool exposes only the writable scopes of the current run. Retrieval returns `self` memory plus `business` memory in readable scopes. `restricted` memory is returned only when its exact scope is in the readable set.

In the first implementation:

- every source except `external_end_user` can read `workspace`-scoped business memory;
- a channel run, and a scheduled run of a channel-originated task, can also read its own `channel` scope;
- no project, customer, or campaign entity exists yet, so those scopes are never resolved at runtime. They remain visible in the console memory list. *(Retired by ADR 0014.)*

Vector recall over-fetches up to the retrieval service's page limit before filtering, so a scoped recall can return fewer results than requested when most nearest matches are out of scope.

### Audit

The trust level and resolved scopes are stored on the run and on every memory event. They contain identifiers and enum values only, never memory content.

## Consequences

- External callers can no longer change what the DigitalHuman remembers for everyone.
- Channel conversations stop leaking working context into unrelated groups by default. Identity-level continuity across channels is now built through promotion instead of automatic writes. This trade-off is deliberate.
- Existing memories remain `self` memory. No backfill is required. Operators review existing entries and archive any that should not be shared; re-scoping existing entries needs its own management operation.
- Scope resolution becomes a shared server-side concept. ADR 0005 reuses the same trust levels for task visibility.
- The new admission policy first shipped behind `ZGI_DIGITAL_HUMAN_MEMORY_TRUST_POLICY_ENABLED`. After acceptance the switch was removed (2026-10-22): the policy always applies and the origin-based checks are gone.
- Per-user instances, an artifact library, and an approval center are outside this decision. Memory references artifacts by identifier and never copies their content, as ADR 0002 already requires.

## Amendment (2026-10-25, ADR 0014)

### Business scopes

Business memory has three scopes: `workspace`, `channel`, and `instance` (ADR 0006).

The `project`, `customer`, and `campaign` scopes are retired. The runtime never resolved them.

- **Moved rows.** ADR 0015 removes the remaining scopes as well; its migration `202610261200000000_digital_human_memory_without_scopes` moves rows in the retired scopes into the DigitalHuman's own memory together with the others.
- **API and console.** The enums and the operator proposals API no longer accept the retired scopes. The console no longer offers them.

### Trust tiers

ADR 0013 groups the four trust levels into three tiers:

| Tier | Trust levels |
|---|---|
| operator | `authorized_operator` |
| member | `workspace_member`, `channel_participant` |
| external | `external_end_user` |

The table in this ADR still decides what each level may write. ADR 0014 shows the same rules as directories (`/me/`, `/conversation/`, `/user/notes/`, and the user's own `/user/`), and grants no new write permission.

## Amendment (2026-10-26, ADR 0015)

Business scopes are removed. A DigitalHuman used directly, in the console or in channels, has one memory of its own; an external end user's run keeps notes on that person in the person's relationship memory.

- **Visibility instead of scope.** Each entry of the DigitalHuman's memory is visible to `everyone` or `internal` only. An external end user's run never reads `internal` entries.
- **Origin.** Each entry records where it came from: `operator`, `member`, `channel` (with the channel conversation), or `end_user`.
- **Writes by trust level.** The levels in this ADR still decide who may write:

  | Trust level | Writes | Visibility |
  |---|---|---|
  | `authorized_operator` | the DigitalHuman's memory | `everyone` by default, or `internal` |
  | `workspace_member` | the DigitalHuman's memory | `internal` |
  | `channel_participant` | the DigitalHuman's memory | `internal` |
  | `external_end_user` | notes on that person, in relationship memory | — |

- **Guard.** A run below `authorized_operator` cannot change an `everyone` entry. Only an operator changes what external users see.
- **Server-resolved targets.** As before, the model never names a scope or visibility; the run's trust level decides where a write lands.
- **Migration.** `202610261200000000_digital_human_memory_without_scopes` folds every scoped row into the DigitalHuman's memory as `internal`, or into the person's relationship memory as a note, with an audit event for each fold.
