# Multi-entry Linux execution delivery

## Scope and rollout

This work extends the existing conversation-owned Linux environment to authenticated
Agent WebApp/API and DigitalHuman interactive conversations. Anonymous chat,
background tasks, channels and Runtime Workspace credentials do not gain Linux or
platform CLI permissions. Application editors continue to configure only
`execution_runtime.enabled`.

Published Agent and DigitalHuman WebApps show tool execution status and duration
without expandable arguments, results or technical diagnostics. Console debugging
retains those detail panels. Approval context and confirmation/rejection actions,
process narration and authenticated artifact cards remain available.

Every stage is gated by the preceding stage's tests. No external entry is enabled
until authenticated file access and approval continuation are complete. Retained
files and cleanup must remain available during an admission rollback.

| Stage | Deliverable | Status |
| --- | --- | --- |
| P0 | Versioned identity, Worker negotiation, additive persistence, compatibility | Complete; unit, PostgreSQL, race and vet gates passed |
| P1 | Shared execution factory, live authorization, end-user admission | Complete; shared factory, regression, race and vet gates passed |
| P2 | Protected file ownership and authenticated preview/download | Implemented, including CLI business outputs; real entry acceptance remains P5 |
| P3 | Published Agent and DigitalHuman interactive script integration | Implementation complete: published adapters, DigitalHuman opt-in UI and request-scoped tools; deployment acceptance remains P5 |
| P4 | Published-resource CLI and frozen, idempotent approval continuation | Implementation and automated gates complete; real dual-backend acceptance remains P5 |
| P5 | Real local/E2B entry acceptance and rollback verification | In progress; Agent API/WebApp and DigitalHuman interactive execution/files verified on both backends; approval and rollback matrix still incomplete |

## Identity contract

The authenticated internal Worker API advertises `execution_identity_version: 1`
on `/v1/agent-runtime/release`. An API must negotiate that exact version before
sending identity-scoped requests, including queries and file transfers. Missing or
unsupported capabilities must not fall back to the historical console account.

The internal `X-ZGI-Execution-Identity` header carries a validated JSON object:

| Field | Meaning |
| --- | --- |
| `version` | Identity contract version, currently `1` |
| `workspace_id` | Resource workspace, not the credential's personal workspace |
| `application_type` | `agent` or `digital_human` |
| `application_id` | Actual application ID; DigitalHuman never uses an Agent ID |
| `entry` | `console`, `webapp` or `external-api` |
| `subject_kind` | `account` for login entries; `api_end_user` for API entries |
| `conversation_id` | Conversation that exclusively owns the environment |
| `publication_ref` | Frozen publication/snapshot reference supplied by the API |

Organization and stable end-user ID remain in the existing internal scope headers.
These headers require the dedicated Worker credential; they are not public API
authentication and must never be copied from client-controlled execution fields.
API end-user IDs use the existing trusted application-backend identity contract;
the identifier is not independent proof of the person's identity. Billing and
resource authorization remain separate from execution ownership.

Bindings persist immutable identity JSON. Legacy rows retain JSON `null`, their
existing account ownership, idempotency hashes, workspace names and checkpoint
keys. New identity scopes cannot adopt legacy bindings. Binding, idempotency,
Job, output and file access all require the complete matching scope.

Migration `2026100803` adds the identity and a capacity index. It does not rename
existing resources, infer historical external users, delete data or require a
destructive rollback. Backup/recovery preserves the explicit identity, including
DigitalHuman bindings with no Agent ID.

## Validation record

The P0 validation uses Go 1.26.2 in Linux containers. Unit coverage includes
missing/invalid identities, unsupported versions, anonymous/background rejection,
application/entry/user/conversation isolation, legacy hash compatibility and CLI
grant identity pinning. PostgreSQL tests cover migration idempotency, immutable
identity persistence and checkpoint recovery. Real backend and UI acceptance is
reserved for P5; these tests do not claim to replace it.

P0 passed:

- Sandbox `agentexecution`, `storage`, `checkpointbackup`: tests with race and vet.
- API `agentexecution`, `runtimeplatform`, `agents`: tests and vet; race for the
  two identity/grant capability packages.
- Storage tests used a dedicated ephemeral PostgreSQL 16 instance with no
  published host port. Existing application databases were not migrated.

The initial shared-database run hit its connection limit. Validation was repeated
successfully against the isolated database rather than changing live limits.

## End-user admission

Identity-scoped applications default to two active environments and one queued
or running Job per application/end user. Operator settings are
`ZGI_AGENT_RUNTIME_MAX_BINDINGS_PER_END_USER` and
`ZGI_AGENT_RUNTIME_MAX_JOBS_PER_END_USER`; both require positive integers.
Existing backend, organization, binding and timeout limits continue to apply.

Admission is a PostgreSQL transaction under the existing shared advisory lock,
not an in-process counter. Counts span backends, entries and publications.
Suspended/deleted bindings release compute capacity; resume reserves it again.
Queued Jobs consume capacity immediately, terminal Jobs release it, and
idempotent retries do not consume a second slot. The historical console contract
is unchanged until migrated through the shared factory.

Concurrent two-store PostgreSQL tests verify the 2/1 defaults, cross-backend
counting, independent users/applications, repeated creates, resume at capacity
and terminal-Job capacity release. Storage, Worker and control-plane tests pass
with race detection and vet.

## Shared execution factory

`capabilities/agentexecution/runtime` owns Shell, dependency preparation, file
transfer, Skill placement and CLI relay. Application adapters supply verified
identity, live authorization, publication and file persistence. The console Agent
adapter retains the historical binding and artifact contract. DigitalHuman scope
does not manufacture an Agent ID, and CLI queries, file transfers, cancellation
and background reclamation preserve the full execution identity.

`ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON` is an operator-only mapping from
`application:entry` to organization UUIDs, for example
`{"agent:webapp":["00000000-0000-4000-8000-000000000001"]}`. An absent setting
allows only the existing console Agent path; an explicit empty object denies
all entries. Wildcards and malformed mappings are rejected. This is an admission
gate, not a substitute for application permissions or final-user authentication.

New identity-scoped callers require an explicitly supplied protected artifact
persister. Without it the factory returns no tools; it never falls back to public
signed ToolFile URLs. P2-P4 must finish before external adapters can be enabled.

P1 validation includes shared factory and CLI identity tests, existing Agent and
DigitalHuman tests, race checks and vet. Tagged real-backend test files compile
after relocation. Delivery scripts now run those tests from the shared package;
their script contract tests pass. These are not P5 real-backend acceptance.

## Protected execution results

New execution artifacts store immutable organization, resource workspace,
application type/ID, entry, final-user kind/ID, conversation and message ownership.
Migration `202610080900000000` adds nullable ToolFile columns and a protected-file
index/immutability trigger. Historical files remain on their existing contract.
The migration does not delete or rewrite historical rows and has no destructive
down migration.

Identity-scoped factories require a protected persister bound to the prepared
message. File bytes are saved with ownership in the initial record, not made
public and subsequently protected. CLI exports and `workspace_export_file` use
the same persister. Both live and historical artifact events retain
`access_mode: authenticated` and omit `url` and `download_url`.

Authenticated file reads use the existing entry authentication and live
application/conversation permission checks:

| Entry | GET route |
| --- | --- |
| Console Agent | `/console/api/agents/{agent_id}/runtime/conversations/{conversation_id}/files/{file_id}` |
| Agent WebApp | `/console/api/webapps/{web_app_id}/runtime/conversations/{conversation_id}/files/{file_id}` |
| Agent API | `/api/v1/agents/conversations/{conversation_id}/files/{file_id}` |
| Console DigitalHuman | `/console/api/digital-humans/{digital_human_id}/runtime/conversations/{conversation_id}/files/{file_id}` |
| DigitalHuman WebApp | `/api/v1/digital-human-webapps/{web_app_id}/conversations/{conversation_id}/files/{file_id}` |
| DigitalHuman API | `/api/v1/digital-humans/conversations/{conversation_id}/files/{file_id}` |

API reads require the same existing final-user identity as conversation requests.
They do not substitute the credential owner. Anonymous WebApps, tasks, channels
and Runtime Workspace credentials cannot read these artifacts. The legacy public
ToolFile route denies protected records even with a valid permanent or expiring
signature. Unscoped ToolFile binary, stream and presigned URL readers also deny
them. Successful reads return private, no-store bytes rather than a storage URL;
preview and attachment/range requests use the same ownership check.

File reads deliberately do not depend on compute admission or Profile readiness.
Disabling Linux leaves authorized retained results downloadable; revoking current
application or conversation access denies subsequent reads. Changing a published
version does not erase ownership of historical results.

The chat card obtains preview bytes through its authenticated transport and
releases the temporary Blob URL when closed. Each download makes a new
authenticated request. Protected HTML stays in the isolated preview rather than
opening an application-origin Blob in a new tab. No package/provider credentials,
host path or public URL is included in the card.

P2 validation so far: protected-file ownership and signed-URL bypass tests,
Agent/DigitalHuman read-route and cross-scope tests, live/history Skill event
tests, real isolated PostgreSQL migration/idempotency/immutability tests with race
detection, changed API package tests and vet, frontend transport/card tests,
lint (existing warnings only), type-check and production build. Upload/staging
ownership contracts now have additional unit, race and PostgreSQL validation as
described below. New entry factory wiring and real UI/backend acceptance remain
outstanding; new external execution adapters have not been enabled.

The DigitalHuman API documentation contract and the protected-file documentation
checks pass in both locales. The broader Agent documentation test currently
fails on missing descriptions in the pre-existing `MemorySettings` schema (also
present at the baseline); this unrelated contract has not been rewritten as
part of the execution-file change.

## Execution input ownership

Identity-scoped execution requires a turn-scoped attachment reader as well as a
protected artifact persister and live application authorization. The reader
freezes only the prepared turn's admitted IDs, names and sizes. It rechecks file
metadata and current authorization before reading bytes. Every Shell invocation
also rechecks all admitted inputs before reaching the Worker, including when
staging was cached. CLI input reads use that same reader, not the generic file
service. Missing authorizers, missing ownership stores or mismatched scope fail
closed. Revocation does not remove files already present in an environment; it
prevents subsequent authorized execution/read requests from using them.

New Agent WebApp/API uploads for a Linux-enabled published Agent are bound to
organization, resource workspace, application, entry and stable final user before
the upload ID is returned. Migration `202610081000000000` creates an additive
ownership table and immutable trigger without rewriting historical uploads. A
binding failure attempts to delete the newly uploaded file and returns no file
ID. API credential IDs and billing accounts are not upload owners; credential
rotation preserves the existing external-user derivation. Generic temporary-file
metadata may contain a zero tenant or nil workspace, so immutable ownership is
checked separately against the published resource workspace.

Historical unbound Agent uploads are not automatically adopted into new execution
entries: upload them again through the appropriate authenticated application when
Linux is enabled. Explicitly attaching an authorized upload to another conversation
of the same application/user remains possible; no environment inherits input
files implicitly. Ordinary chat when Linux is disabled retains its previous
upload contract. DigitalHuman uses its existing external-file binding and a live
execution input authorizer rather than duplicating that domain's records. Its
console, anonymous, task and channel upload rules are not broadened.

Input validation passed for revoked/deleted files, mismatched metadata, arbitrary
file IDs, cross-user/application/entry ownership, missing stores and authorizers,
failed-upload cleanup, explicit API users and preserved billing identity. Real
isolated PostgreSQL tests cover repeated migrations, immutable ownership and two
concurrent claims producing exactly one owner. Changed backend packages passed
tests with race detection and vet. These are contract tests, not P5 end-to-end
acceptance. P3 must supply the protected reader from each verified entry adapter;
P4 approval continuation remains required before external execution is opened.

## DigitalHuman execution configuration

DigitalHuman drafts accept the optional `execution_runtime: {"enabled": true}`
application opt-in. Profile, policy, backend, network and Home Snapshot are not
accepted in this object. Omitting it (or sending null on an update) leaves the
draft unchanged; `{"enabled": false}` clears it. New versions without an opt-in
remain disabled.

Publication, the next draft, version restore and sealed execution snapshots copy
the setting independently. Later draft edits cannot change a published or
already resolved execution. Disabled settings are normalized to absence so old
version and v1 execution-snapshot hashes remain unchanged. The additive migration
`202610081100000000` leaves historical rows null and rejects unsupported JSON
shapes. No runtime tool or new entry permission is granted by persisting this
setting; entry adapters, operator approval and P4 remain separate gates.

The frontend service contract and DigitalHuman editor expose this optional
boolean. Version preview is read-only and renders a missing value as disabled.
Operator deployment gates still determine whether execution is available.

P3 configuration validation passed: DigitalHuman and migration package tests with
race detection and vet; real isolated PostgreSQL migration idempotency, JSON
constraints and model serializer/clear-and-reenable checks; frontend type-check
and lint (73 existing warnings, no errors). No live application database was
migrated. This is not an execution or dual-backend acceptance result.

Operator Profile/Home approval resolution now lives in the shared execution
capability. The Agent adapter preserves its existing public error code,
configuration compatibility and diagnostics; new application adapters can use the
same environment defaults without importing the Agent module. The shared resolver
accepts only the application's enabled boolean, not client-selected environment
settings. Existing Agent approval/publication tests and the shared runtime and
DigitalHuman packages passed race detection and vet. This refactor does not enable
new entries or grant DigitalHuman Shell access.

The shared factory also validates identity-scoped requests against the prepared
conversation and message: organization, runtime-data and resource workspaces,
application, entry/WebApp, user, billing owner, message/conversation relationship
and deletion state. Every invocation remains bound to its original message.
Agent API execution uses the explicit external user without rewriting the billing
account; DigitalHuman uses its separately verified interactive UserSubject. Runtime
Workspace credentials are rejected even if an adapter supplies a plausible
identity. Legacy console bindings keep their existing identity contract.

Unit isolation tests cover both application types across all three planned
interactive entries, missing/mismatched ownership, revoked/deleted conversation
state, changed messages and separate resource/data workspaces. Runtime, Agent and
DigitalHuman tests passed with race detection and vet. These checks defend the
shared boundary; they do not authenticate an anonymous visitor or replace entry
authentication. The verified published-entry adapters, DigitalHuman runtime
injection and dedicated P4 approval routes are implemented below. P5 real
local/E2B acceptance is still outstanding.

## Entry readiness gate

Identity-scoped factories now require an explicit server-supplied readiness
callback, independently of the operator's organization/entry allowlist. A missing
or negative callback produces no execution tools. Readiness is rechecked for
every tool invocation, so withdrawing it denies calls before contacting the
Worker. The legacy console factory does not change its identity contract.

Readiness must only be wired when the entry's protected files and approval
continuation are implemented. It is not a client field or an environment-variable
override. Unit tests exercise missing/negative readiness and withdrawal across
both application types and all three planned interactive entries. Runtime, Agent
and DigitalHuman package tests passed with race detection and vet.

## Published Agent execution adapters

WebApp chat/regeneration configuration and API chat/continuation configuration
attach the shared factory through a verified published-entry adapter. P4 supplies
the server readiness callback for the implemented interactive entries; publishing
`execution_runtime.enabled` alone does not grant admission. Ordinary
authenticated and anonymous chat keeps working without an execution gateway.

The adapter freezes application, organization, resource/data workspaces, verified
final user, billing account and publication UUID as values rather than retaining
request-owned identity pointers. WebApp execution requires a verified login;
API execution requires the existing resolved external user and authenticated
credential ID. Runtime Workspace credentials never acquire this factory.

Every tool invocation rechecks current WebApp access or API credential
status/expiry/application/workspace and the current API surface policy. It also
rechecks the published Linux opt-in and operator approvals. A publication change
fails the existing invocation instead of silently swapping its configuration;
a new chat turn can select the new publication. Pending CLI approval calls seal
their original publication and owner. The CLI relay additionally requires the
operator platform-CLI switch and live authorization.

Input files use the P2 application/end-user upload authorizer; result files use
the protected, message-bound persister, never public signed URLs. Only operator
defaults choose the backend/Profile/network/Home. Enabled Skill IDs come from the
published configuration, and dependency preparation is offered only for approved
compatibility Profiles, not standard package-install Profiles.

Validation used Go 1.26.2 in Linux containers. Runtime, Agent and DigitalHuman
package tests passed with race detection and vet. Adapter tests cover frozen
identity/publication, cross-owner/workspace/entry rejection, explicit final users,
missing/expired/revoked API credentials, live surface/opt-in revocation, failed
authorization stores, protected tool wiring and readiness withdrawal. Handler
contract tests verify that opt-in/allowlist alone cannot release WebApp/API chat
or API continuation, and that anonymous chat remains available. The API liveness
queries use isolated SQLite fixtures; no new schema or live database migration
was introduced. This is not real Skill execution or P5 dual-backend/UI acceptance.

The DigitalHuman injection described below completes the next backend adapter.
Configuration UI and P4 published-resource CLI/frozen approval continuation are
implemented below. P5 remains outstanding; no
runtime deployment or remote push accompanies these commits.

## DigitalHuman interactive execution adapter

RuntimeService prepares the optional Linux factory only after the durable
invocation, RunSpec and execution snapshot pass their existing integrity checks.
Preparation failure records a safe failed-run message and stops before preparing
chat. Existing DigitalHuman billing and model selection are unchanged.

The adapter freezes organization, workspace, DigitalHuman, publication UUID,
run, entry and explicit UserSubject as values. API runs intentionally have no
persisted ActorID: their verified API-end-user UserSubject owns execution, not
the API key or its owner. Console and logged-in WebApp require an account subject
matching their authenticated principal; WebApp also pins its application ID.
Prepared conversation/message/run ownership is checked again by the adapter and
shared factory. Anonymous WebApp chat, scheduled tasks and channels receive no
Linux tools. Legacy snapshots without the opt-in keep their old behavior.

Every invocation rechecks the current published version and its Linux opt-in.
WebApp/API surface withdrawal and API-key revocation/expiry deny execution;
console additionally rechecks workspace run permission. A publication change
fails the old invocation instead of silently selecting a new version. These
checks do not increment API-key usage counters or rewrite billing identity.

Enabled Skills come from the sealed binding snapshot. Inputs use the existing
console ownership or external application/end-user upload authorizer, with live
execution authorization repeated before staging. Artifacts use the protected
message-bound persister. Operator approvals alone choose Profile, backend,
network and Home; compatibility-only dependency preparation follows the shared
factory contract. CLI relay and approval use the dedicated P4 routes below.

Production wiring supplies EntryReady for console, logged-in WebApp and API
interactive entries. Operator allowlists, application opt-in, verified identity
and Worker negotiation remain independent gates. The deferred ToolFile manager lookup avoids
depending on initialization order; an absent manager denies enabled preparation
instead of falling back to publicly signed result files.

Validation used Go 1.26.2 in Linux containers:

- DigitalHuman, shared execution runtime and Agent package tests passed with
  race detection; focused DigitalHuman route tests passed with race detection.
- Vet passed for all four packages.
- Tests cover all three interactive entries, first-turn conversation allocation,
  immutable publication/subject values, cross-run/message/application/workspace
  rejection, missing identity, anonymous/background exclusion, live readiness
  withdrawal, unavailable dependencies and failure-before-chat ordering.
- Isolated SQLite fixtures exercise real publication, surface and credential
  liveness queries, including opt-in disablement, expired/revoked keys, disabled
  applications and replaced publications. Production wiring tests prove that
  operator configuration does not grant tools while readiness is unset.

The preceding adapter validation was performed before P4 readiness was wired.
It did not constitute real Skill or dual-backend acceptance.

## P3/P4 interactive CLI closure

DigitalHuman draft and version-preview UI exposes only the Linux enable switch;
missing fields default to disabled. Agent WebApp/API and DigitalHuman console,
logged-in WebApp and API use the same server-owned execution factory. Model and
business-tool CLI calls reenter the current configured model, billing, task
budget, loaded-Skill and live resource-binding boundaries. No personal connection
discovery or alternate billing identity is introduced.

`ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED` enables CLI only within already-admitted
Linux entries. `ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON` remains default-closed for
new entries. Application enablement, organization/Profile approvals, execution
identity negotiation and live publication/surface authorization are all still
required. Configuration alone is not a claim of backend acceptance.

Dedicated `tool-governance/{correlation_id}/continue` routes are registered under
the existing authenticated Agent and DigitalHuman conversation/message routes.
Agent API uses the existing external-user header or `user` query contract;
DigitalHuman API requires the existing `user` body field. WebApp uses verified
login identity. Runtime Workspace, anonymous, task and channel entrypoints do
not acquire these capabilities.

Pending CLI approvals seal organization, resource workspace, application, entry,
final user, conversation, message and publication into the frozen invocation
hash. Confirmation checks these coordinates and current resource permissions
before recording the decision and immediately before invoking the business tool.
Publication replacement rejects the old call rather than silently using a newer
configuration. DigitalHuman continuation restores the sealed original run; a
rotated API credential is used for live authorization without rewriting that run
or its final-user owner.

Confirmation executes only the frozen business invocation; rejection executes
none, including after compute admission is disabled or the invocation expires.
Neither decision reruns the original Shell or restores its revoked Job grant.
CLI decisions cannot create reusable session grants, even if requested by the
client. Completed decisions replay stored events instead of repeating business
effects. Concurrent claims are transactionally exclusive; uncertain write
results are reported, never automatically retried.

CLI business tools producing raw or remote ToolFiles inherit the verified final
user, application, entry and message owner. File, chart, diagram and generated
image results use authenticated file metadata, not legacy signed links. Legacy
unprotected outputs retain their prior URL contract. Protected files remain
unreadable through the legacy signed reader even with a valid signature.

Automated validation (Go 1.26.2 in Linux containers):

- Targeted tests and race checks for shared runtime, chat service/loop,
  governance, Skills, Agent, DigitalHuman, ToolFile, image assets, file/chart/
  diagram/image generators and both route packages; targeted vet passed.
- Six application/entry combinations cover frozen confirm-once/reject-zero,
  no Shell replay, duplicate result replay, publication/owner mismatch, revoked
  authorization and rejection without compute. Isolated PostgreSQL 16 tests
  verify one successful continuation claim among twelve concurrent requests.
- Actual HTTP handler/SSE tests and production route registration/authentication
  checks supplement OpenAPI and transport contracts. Missing identities,
  anonymous login and Runtime Workspace requests fail closed.
- Frontend approval transports, protected files and DigitalHuman configuration
  contracts passed. Both localized API guides match 26 operations and 58 SSE
  event definitions. Type-check, lint and production build passed; lint has
  existing warnings, no errors.

No application database migration or remote deployment was performed for P4.
P5 must still run real local gVisor and E2B tests for every new entry, with two
final users, file download/content inspection, actual model usage, controlled
business side effects and admission rollback. Mock and PostgreSQL results above
do not replace that acceptance. Rollback disables new environment/CLI admission
while retaining authenticated file reads, existing queries and cleanup; it does
not delete files, reverse additive tables or replay pending writes.

## P5 initial WebApp correction and focused acceptance

The initial WebApp chat handler omitted execution-tool attachment even though
continuation contexts attached the tools. Initial chat now reuses the same
verified scope, caller, published configuration and authorization helper before
preparing the turn. No anonymous or Runtime Workspace permissions were added.
Actual handler tests cover enabled, application-disabled, server-disabled,
anonymous, entry-denied and application-access-denied requests. The Agent package
tests, targeted published-entry race checks and vet pass with Go 1.26.2.

Real local gVisor and E2B acceptance verified initial WebApp Shell execution,
stderr diagnostics, export, exact authenticated download bytes and a second-turn
file modification in the same conversation. Cross-entry file access returns 404.
Retained files remain downloadable after suspending compute; no active Jobs
remain. Browser-submitted Shell execution and persisted file cards were checked.
The embedded browser showed a blank TXT Blob preview and did not deliver a
download automation event; those UI checks remain unresolved and are not counted
as successful browser download acceptance. They do not invalidate the separately
verified authenticated binary download responses.

This is focused regression acceptance, not P5 completion. DigitalHuman, the full
standard-Skill/two-user matrix, real CLI approvals and admission rollback still
require real-backend acceptance. Raw evidence, credentials and screenshots stay
outside Git in the ignored acceptance directory.

The status-only WebApp presentation has rendering regression coverage for running,
successful and failed Shell/export/legacy-script events, including retained
console details. It is included in the Web delivery gate. The status-only build
was deployed to the dedicated local gVisor and real E2B candidates with matching
strict release manifests, preserving the API/Worker binaries, checkpoints and
rollback containers. Browser-submitted success and deliberate exit-23 Jobs show
non-expandable status/duration rows; neither WebApp mounts technical-detail
controls, while existing console events still expose their detail panel.

Both backends restored the prior conversation file, generated a 27-byte CSV and
rendered its values in the protected preview. Authenticated download bytes match
exactly on both backends, and cross-entry access returns 404. The Linux WebApp
download button also produced a local file with the exact bytes. E2B preview and
authenticated transport pass, but its browser download click has not produced an
observed local file; that UI check remains open. The prior blank TXT preview also
remains unresolved. A browser download-event timeout alone is not treated as
proof of failure: the Linux file was independently verified on disk.

Each deliberate failure executed once with exit code 23 and stderr present; no
artifact was created by that negative test. Both owned backend environments were
suspended after acceptance, with no queued/running Jobs; retained CSV downloads
still pass. Four focused Web regression suites pass. This verifies the WebApp
presentation change and its adjacent execution/file path, not the remaining P5
DigitalHuman, two-user, CLI approval or admission-rollback matrix.

## P5 focused approval lifecycle correction

DigitalHuman platform CLI approvals now have a dedicated persisted
`waiting_approval_origin=platform_cli`. Migration
`202610090100000000_digital_human_platform_cli_approval` adds that internal field
and extends the waiting constraint without changing existing Connector receipts.
Only a persisted frozen invocation matching the original application, entry,
publication, final user, conversation and message can enter this waiting state.
Connector receipt-based approvals retain their existing continuation lifecycle.

The current SSE connection ends at a waiting boundary; `run_end.status=waiting`
is resumable, not a terminal result. Approval/rejection completes the original
host run after the message completes. Repeated confirmation reuses its result
without resubmitting Shell. Cancellation/failure clear waiting-only fields.
Deploy the additive migration before the new API; do not remove the column or
reinstate the old constraint while platform approvals remain pending.

Focused real acceptance additionally verified:

- DigitalHuman console, authenticated WebApp and API Shell execution, file
  content and authenticated reads on local gVisor and E2B. Separate API users
  generated separate files; cross-user, missing-user and cross-entry reads were
  rejected.
- E2B Agent API confirmation/rejection and repeated decisions; E2B Agent WebApp
  confirmation; local Agent API confirmation and local Agent WebApp rejection.
  Confirmed samples produced one SVG each; rejected samples produced none.
- The corrected E2B DigitalHuman API flow reached persisted waiting, closed the
  SSE stream, then reached succeeded after confirmation. Duplicate confirmation
  retained one SVG and one original Shell Job. Authenticated SVG bytes were
  independently downloaded and parsed.
- A real Agent CLI model call returned its result and recorded model usage in
  the organization billing lane. Cross-Job CLI status access was denied.

Go 1.26.2 changed-package tests/vet, focused race tests, isolated PostgreSQL
constraint/migration tests and documentation contracts cover the correction.
Local candidate deployment retains rollback containers. Test-only always-ask
catalog fixtures are acceptance inputs, not changes to the shipped Skill catalog.

P5 is still open: complete the remaining entry/backend approval, live revocation,
frozen-publication and admission-off retained-download cases before wider rollout.
The E2B browser CSV preview renders correct values, but no browser download file
has been observed in the latest check. Authenticated HTTP download success is
recorded separately; it is not presented as browser-download completion.
