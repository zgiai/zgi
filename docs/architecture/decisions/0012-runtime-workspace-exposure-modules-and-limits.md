# ADR 0012: External Access Exposure Modules, Scale, and Limits

- Status: Accepted
- Date: 2026-10-23
- Builds on: ADR 0010, ADR 0011

## Context

External access (Runtime Workspaces) was built for a few exposed apps. Integrators now expose many Agents and DigitalHumans, and knowledge bases and databases may be exposed later. A review of the module found:

- **Two app types assumed everywhere.** Bindings live in `runtime_workspace_agents` with `CHECK (app_type IN ('agent', 'digital_human'))`, and code across the services, the external router, and the console treats "not a DigitalHuman" as "an Agent". A third type would be routed to the Agent handler silently.
- **Lists that do not scale.** `GET /connect/v1/apps`, the console app list, and the app candidates load everything; the console filters on the client.
- **A hidden app limit.** At most 100 bound apps per Runtime Workspace, all types combined. The limit is hardcoded, refused with a generic error, and neither shown nor documented.
- **Per-app settings in the wrong place.** Per-app quota caps sit in the workspace policy JSON keyed by app key, are edited in a list unrelated to the app, and are ignored by the quota-state analytics.
- **Leftovers.** Bindings of deleted apps stay and block saving the selection. Some lists are cut off without paging (credit ledger, change history, webhook deliveries). Access events are never purged. Credentials are unlimited and listed whole. The end-user export truncates without continuation. The rate limit is a single fixed window per workspace that fails open.

## Decision

### Exposure modules

- External access exposes **modules**. Today there is one, **Apps**: Agents and DigitalHumans, served under `/connect/v1/apps`.
- Knowledge bases and databases, when exposed, are their own modules: their own binding tables, routes (`/connect/v1/knowledge-bases`, `/connect/v1/databases`), capabilities, permissions, and metering. They are never app types and never share `/apps`.
- The console groups modules under External access next to Apps; only Apps exists now.
- The points quota of ADR 0010 and ADR 0011 is the **model usage** quota. Other modules meter separately.
- Code handles app types explicitly: an unknown type is "app not found", never an Agent.

### Scale

- App lists and candidates are paged and searched on the server.
- The Apps page is a table with type, status, and usage, and each app opens a detail with its own settings, including its per-app quota cap. Per-app caps stay in the policy, next to the plans that override them, so there is one place for quota rules and no data migration; they are edited from the app and count in quota states.
- Bound apps are managed with add and remove changes, so the console never sends the whole selection; candidates are matched and ordered by name on the server and authorized a chunk at a time until a page is full.
- The bound-app limit stays, but is configurable (`ZGI_RUNTIME_WORKSPACE_MAX_APPS`, default 500), refused with its own error, shown in the console, and documented.

### Limits and leftovers

- Bindings of deleted apps are reported as removed and are dropped when the selection is saved.
- Lists that were cut off are paged.
- Active credentials per Runtime Workspace are limited (50); revoked ones are filtered on the server.
- Request bodies that change the policy are bounded.
- Access events are kept for a retention period.
- The end-user export becomes an asynchronous job with a download.
- Rate limiting adds a per-end-user limit; failing open when the limiter is unavailable is documented.
- Deliberate v1 limits stay and are documented: windows of 24 hours, 7 days, and 30 days; "near" at 80%; one webhook and one Runtime Workspace per workspace; batch changes of up to 200 end users, applied one by one.

### Delivery

1. Explicit app types, the configurable app limit with its error and console counter, removed-app bindings, paging of cut-off lists, credential limits, bounded policy bodies, and documentation fixes.
2. The Apps table, server-side paging and search, the per-app detail, and per-app caps in quota states.
3. Access-event retention, the asynchronous export, per-end-user rate limits, and credential scopes.
4. Knowledge base and database modules, when they are built.

## Consequences

- A new exposed resource adds a module instead of widening the app-type enum, so app code stays specific to apps.
- Integrators see limits in the console and the documentation instead of generic errors.
- Per-app caps remain keyed by app key in the policy; a cap of an app that is no longer bound is ignored.
