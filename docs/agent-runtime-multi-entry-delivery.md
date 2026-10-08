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
| P2 | Protected file ownership and authenticated preview/download | Pending |
| P3 | Published Agent and DigitalHuman interactive script integration | Pending |
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
