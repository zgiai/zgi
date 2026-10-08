# Multi-entry Linux execution delivery

## Scope and rollout

This work extends the existing conversation-owned Linux environment to authenticated
Agent WebApp/API and DigitalHuman interactive conversations. Anonymous chat,
background tasks, channels and Runtime Workspace credentials do not gain Linux or
platform CLI permissions. Application editors continue to configure only
`execution_runtime.enabled`.

Every stage is gated by the preceding stage's tests. No external entry is enabled
until authenticated file access and approval continuation are complete. Retained
files and cleanup must remain available during an admission rollback.

| Stage | Deliverable | Status |
| --- | --- | --- |
| P0 | Versioned identity, Worker negotiation, additive persistence, compatibility | Complete; unit, PostgreSQL, race and vet gates passed |
| P1 | Shared execution factory, live authorization, end-user admission | Complete; shared factory, regression, race and vet gates passed |
| P2 | Protected file ownership and authenticated preview/download | File contracts, upload provenance and scoped staging implemented; new entry wiring remains P3 and real acceptance P5 |
| P3 | Published Agent and DigitalHuman interactive script integration | In progress: DigitalHuman versioned opt-in; execution adapter wiring and UI pending |
| P4 | Published-resource CLI and frozen, idempotent approval continuation | Pending |
| P5 | Real local/E2B entry acceptance and rollback verification | Pending |

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

The frontend service contract includes this optional field, but the DigitalHuman
editor switch is not yet exposed. P3 execution integration and P4 approvals must
be wired before the UI advertises this as an available capability.

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
authentication. The next P3 work is the verified published-entry adapters and
DigitalHuman runtime injection. New entries remain disabled until P4 is complete;
P5 real local/E2B acceptance is still outstanding.
