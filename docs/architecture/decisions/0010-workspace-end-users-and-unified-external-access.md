# ADR 0010: Workspace End Users and Unified External Access

- Status: Accepted
- Date: 2026-10-23
- Builds on: ADR 0001, ADR 0006, ADR 0007

## Context

Agents and DigitalHumans are both published to external integrations, but through different paths:

- **Runtime Workspace** (`/api/connect/v1`) is the workspace-level external access. One credential reaches every Agent bound to it, with rate limiting, per-request audit, external files, and one end-user identity per credential and `user`. Today it binds Agents only (`runtime_workspace_agents`).
- **Per-application API keys.** Each Agent has its own key, and each DigitalHuman has its own keys, routes, file bindings, audit table, and OpenAPI document. The DigitalHuman path has no rate limiting and no per-request audit.

End users are identified three ways, all in `usersubject`: per Agent, per DigitalHuman, and per Runtime Workspace. Nobody can see an end user across applications, limit what they spend, suspend them, or erase them in one place.

ADR 0006 planned per-instance quotas and end-user management inside the DigitalHuman domain. Agents need the same control, and two implementations would drift.

## Decision

### Runtime Workspace is the unified external access

- A Runtime Workspace can bind DigitalHumans as well as Agents. The binding table gains an application type; existing rows are Agent bindings.
- `/api/connect/v1/apps/{app_key}/...` serves a bound DigitalHuman with the DigitalHuman's own operations (chat, conversations, runtime inputs, tasks, reports, files, memory) behind the same credential, lease, audit, rate limit, and subject middleware Agents use.
- Moving a DigitalHuman to another workspace relocates its runtime surfaces, as for Agents. Its Runtime Workspace bindings stay, so integrations keep calling it, and its end users keep their identities.
- The per-application API keys of Agents and DigitalHumans stay as compatibility entry points. They keep working but gain no new features: no end-user quotas and no end-user management.
- The WebApp is unchanged by this decision.

### One end user per Runtime Workspace

- Through a Runtime Workspace, an end user is the existing subject derived from the Runtime Workspace and `user`. The same person is one end user across every application bound to that Runtime Workspace.
- Each application still keeps its own state for them. A DigitalHuman's end-user instance (ADR 0006) is the pair (DigitalHuman, subject), so instance memory and tasks never cross DigitalHumans even though the identity is shared.
- End users who arrived through a DigitalHuman's own API key keep their existing subjects. They are not merged with Runtime Workspace subjects: an integrator moving to a Runtime Workspace starts those end users afresh.

### Quotas belong to the Runtime Workspace

Quotas are billing in points, configured only in the Runtime Workspace, never in an Agent's or DigitalHuman's own settings.

- **Policy.** The Runtime Workspace policy holds, next to its request rate limit:
  - a default points quota per end user over a rolling window (24 hours, 7 days, or 30 days). Points may be fractional down to 0.001, since a model call often costs less than a point. Unset means unlimited, the default;
  - optional per-application caps over the same windows.
  - Per-end-user overrides are set in end-user management.
- **Metering.** The LLM gateway records the end-user subject on each usage bill, and the Runtime Workspace when the caller knows it, with an index, and changes nothing about deduction. An end user's usage is the sum of their bills' points in the window; the subject alone identifies the Runtime Workspace that issued it. Bills of calls without a Runtime Workspace end user are unchanged.
- **Enforcement** is in the chat runtime, shared by Agents and DigitalHumans:
  - at run start, a run of an end user whose usage has reached a quota is rejected before any conversation turn is written;
  - before every model round and every platform model call, the same check stops the run;
  - DigitalHumans also check before queueing a run, so queued and scheduled runs stop early.
  - Bills settle after each model call, so a run can exceed a quota by at most the call in progress.
- The error is the catalog's `quota.exceeded` (HTTP 429), the same for both application types.
- Organization balances and plan limits still apply. A quota only narrows what an end user may spend.

### End-user management

A workspace-level module manages end users of Runtime Workspaces:

- list end users per Runtime Workspace, with first and last activity, the applications they used, usage per application and window, and the remaining quota;
- set a per-end-user quota, suspend and resume, export, and erase. Suspension rejects new runs; erasure deletes the end user's conversations, files, memory, instances, and connections in every bound application and keeps a tombstone with identifiers only;
- application-specific panels: a DigitalHuman shows the end user's instance memory, tasks, and per-user connections (ADR 0006, ADR 0007);
- every change is an audit event with identifiers and enum values only.

The module keeps a registry of each Runtime Workspace's end users: the subject, the External User ID the integrator sent (no other table stores it, and erasure clears it), status, an optional quota of their own, and first and last activity. A request records its end user at most once a minute. A per-end-user quota replaces the Runtime Workspace's per-end-user limits for that end user, as its own limits or no limit; per-application caps still apply. Suspension is enforced by the same chat runtime check as quotas and returns `end_user.suspended` (HTTP 403); a suspended end user can still read their data. Changes are events with an action (`suspended`, `resumed`, `quota_set`, `quota_reset`), the actor kind (console account or credential), and the windows a quota uses.

The integrating server manages the same end users with its credential: `GET`, `PATCH` (quota, status), `POST .../export`, and `DELETE` under `/api/connect/v1/end-users/{user}`.

### Delivery

1. DigitalHumans bind to Runtime Workspaces and serve `/api/connect/v1`, with rate limiting and audit.
2. Metering on usage bills, the Runtime Workspace quota policy, and chat runtime enforcement.
3. End-user management in the console and the external end-user endpoints.
4. Per-user Connector credentials (ADR 0007) owned by the end user's instance.

Runtime Workspaces are still behind `ZGI_RUNTIME_WORKSPACE_ENABLED`. The switch is removed once this delivery is accepted.

## Consequences

- One credential, one end-user identity, one quota, and one audit trail cover Agents and DigitalHumans alike.
- DigitalHumans stop growing a parallel external access stack. Their own keys, files, and audit tables become compatibility paths.
- Quotas need a bound Runtime Workspace, so integrations that keep per-application keys, and the WebApp, have no end-user quotas.
- The gateway gains two nullable attribution columns and an index; deduction and remote billing are unchanged.
- Sharing an identity across applications in a Runtime Workspace replaces ADR 0006's "one identity per DigitalHuman"; isolation of application state is unchanged.
