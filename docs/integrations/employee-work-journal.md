# Durable employee work and connector operations

## Contract

Employee instructions and scheduled-task prompts define the business process. The runtime does not require social platforms, fixed stages, two languages, a sampling age, or four schedules. Existing DigitalHuman tasks remain the scheduler; users choose prompts, cadence and task count there.

A reusable work_journal capability provides owner-scoped records and external operations. Record kinds, business state, content and sources are supplied by the scenario. Records use compare-and-swap revisions and immutable version history. Record content is work evidence, never server authorization.

External operations freeze the exact connector, connection, published employee version, operation, JSON input and stable business work key. Their server-controlled lifecycle is pending -> executing -> succeeded / failed / unknown. A provider receipt proves connector execution, not recipient reading or business completion. Business state remains separate.

Only live server-restored scheduled employee identities can use the tools. Scope is organization/workspace/owner/employee, never model-supplied. All dispatches use the existing native connector router, binding checks and background grants. Generic journal claims are private context values validated against persisted scope, exact input digest, executing state and single dispatch attempt. Existing social-specific task handling remains a compatibility path, not the default UI or prerequisite for new work.

An executing or unknown operation is never sent again. Recovery reads the native execution audit using its stable logical ID; models cannot supply success receipts or reset state. Process termination leaves a visible executing operation recoverable without resending. Confirmed pre-submission rejection and provider-declared failure are failed. Ambiguous results are unknown. Same unresolved request bytes must not be bypassed by changing the work key. No claim of exactly-once delivery is made for providers that lack it.

Social and customer-support prompts are editable examples using the same provider. Scenario-specific image review, post readback, metric cadence and archive formatting belong to instructions and connector tools. Old social journal/history/routes remain readable and old schedules retain their existing protections; no automatic deletion or migration of their side effects.

## Implementation

- [x] Add domain-independent workjournal records/history and operation lifecycle, migration and behavioral tests.
- [x] Register work_journal tools with server-restored ordinary employee-task scope and native connector dispatch/recovery.
- [x] Allow generic claims through existing background authorization without weakening grants or legacy social checks.
- [x] Replace mandatory social setup in employee configuration with editable prompt examples using existing persona/task editing.
- [x] Add reusable work-journal Skill and social/customer-support prompt examples; retain legacy social Skill compatibility.
- [x] Exercise both scenarios, CAS conflicts, tenant isolation, repeated sends, unknown recovery and authorization denial; validate affected backend/frontend packages without starting Next.js.

## Compatibility and validation

The new tables are additive. Existing social records/files and schedules are not silently converted. New scenarios do not need a social configuration. A fixture connector verifies shared mechanics for social publishing and mail replies; real account acceptance remains separate from local integration tests.

## Using the capability

1. Add **Work Journal (`work-journal`)** in the employee's existing capability picker. Bind the native connectors that the SOP needs, then save and publish the employee.
2. Write the business process in persona/rules or the scheduled-task prompt. The configuration panel can append an editable social-operations or customer-support example without replacing the current draft.
3. Create as many existing Automation Tasks as needed with the chosen prompts and schedules. The journal does not create schedules. These tools currently require an active scheduled employee run; ordinary interactive chat is not a journal execution surface.
4. Issue existing background grants for the exact published connector bindings, operations, expiry, invocation limit and destinations. A prompt and a prepared operation grant no authority. Dispatch checks current binding, account, contract and grant again.
5. View the current user's records, version history and operation results in the employee configuration panel. This requires execution-log view permission. Refresh to load new worker output. The read-only APIs do not expose retry, reset or caller-authored receipt endpoints.

## Tools and data

The builtin provider `work_journal` exposes nine tools: `save_record`, `read_record`, `list_records`, `record_history`, `prepare_operation`, `dispatch_operation`, `read_operation`, `list_operations`, and `recover_operation`. Each accepts a single JSON-string `input`. The Skill supplies schemas and runnable prompt examples; native connector schemas remain authoritative for operation arguments.

Business records have arbitrary bounded `kind`, `key`, `state`, JSON `content` and a JSON-array of `sources`. Create with `expected_revision: 0`, then update using the last observed revision. Each successful write appends an immutable version in the same database transaction. A business record called an archive or approval is still business data, never execution authority.

Prepare external operations using a stable source/action/revision `work_key`, `connector_id`, `connection_id`, native `operation_id`, and JSON-object `arguments`. The journal UUID returned as `operation.id` is what dispatch/read/recover tools call `operation_id`. Reusing the work key with different frozen arguments or a different employee version conflicts. No automatic retry of failed operations is provided. Pending requests from an old published version cannot be dispatched through a new version. Same-owner, same-employee runs may recover old-version executing/unknown operations by reading their original native audit evidence.

JSON payloads are bounded to 48 KiB per value after normalization. Frozen operation input uses JSON text so PostgreSQL JSONB cannot rewrite numeric lexemes; the native route copies numbers without float64 conversion. Records and receipts use JSONB. This freezes JSON arguments, not mutable remote URLs or files: native connector adapters own their attachment/media contracts and limits. No generic binary asset renderer or media adapter is implied by this journal.

Tool pages use `next_after_id` or `next_after_revision`. Follow returned cursors even if a page contains fewer than the requested row count: large records may exhaust the bounded tool response budget first. HTTP lists are limited to 100 rows.

## Mainline artifact center integration

Finished documents and generated files use the existing [artifact center](../architecture/decisions/0009-artifact-center.md). Its runtime tools remain `publish_artifact`, `list_artifacts`, `read_artifact` and `archive_artifact`. They are injected by the existing artifact runtime provider, independently of the nine Work Journal tools. Runtime-exported files reuse their returned artifact references; they are not published a second time.

Work Journal stores business state and evidence, with returned artifact references in arbitrary record content:

```json
{
  "deliverable": {"artifact_id": "<UUID returned by the artifact tool>", "version": 2},
  "publication_url": "<verified platform URL>",
  "share_id": "<ID returned by the existing share API, when requested>"
}
```

These references do not grant artifact access or validate existence; reads continue through the artifact center's existing scope checks. An artifact version is independent of the journal's CAS revision. If publishing fails or tools are unavailable, retain a pending/blocked business state rather than claiming a saved deliverable.

Sharing uses the existing platform interface/API, not a second share service or new Work Journal tools. Shares pin a version and follow the existing ownership, expiry, archive and sharing-disabled checks. Bearer URLs/tokens are not copied into business records or sources. Creating a share does not prove a social post or message was delivered; connector writes still use the operation lifecycle. Existing legacy records, artifacts and files are not migrated or deleted.

## Source collection SOP

The editable social-operations prompt and [research reference](../../api/internal/modules/skills/catalog/work-journal/references/research-collection.md) describe a configurable collection process:

1. Restore each source checkpoint and pending items through existing journal tools; check actual read/search tools and approved sources.
2. Read changed product sources first, then search industry topics and open original evidence. A commit is not a deployment; planned and unconfirmed capabilities stay distinct from released capabilities.
3. Preserve source IDs/URLs, article/event/observation times, excerpts and verification state. Unknown dates remain unknown; conflicting claims stay pending.
4. Deduplicate exact sources and repeated core claims against material records and recent topics. Save evidence using the observed CAS revision before advancing completed source progress.
5. Retain unresolved positions and incomplete coverage on failed reads, partial pagination, exhausted budgets or failed writes; continue other independent sources.
6. Select useful, supported candidates and save the material brief through the existing artifact center. Keep its returned artifact ID/version, source coverage and pending work in the journal.

This is an instruction template using existing infrastructure, not a new hardcoded collector. Source lists, queries, windows, cadence and record fields remain user choices. It does not create connections, grant access or configure a running employee automatically. Full collection acceptance requires configuring an employee and exercising its actual source tools; infrastructure tests do not prove an LLM followed this procedure against real sources.

## Read-only APIs

All paths are beneath `/console/api/digital-humans/{employeeId}/work-journal` and restore the authenticated organization, workspace and owner. The permission is `digital_human.logs.view`.

- `GET /records?kind=&after_id=&limit=20`
- `GET /records/{recordId}`
- `GET /records/{recordId}/history?after_revision=&limit=20`
- `GET /operations?state=&after_id=&limit=20`
- `GET /operations/{operationId}`

The additive `202610201300000000_create_work_journal` migration creates `employee_work_records`, `employee_work_record_versions` and `employee_work_operations`. Apply it through the existing migration procedure before enabling this capability. Existing social-journal tables/files remain untouched. When replacing specialized schedules, pause the superseded tasks through existing task controls; adoption does not automatically create or retire schedules.

## Development verification

Local verification passed for the work-journal capability and tools, native connector execution, background grants, provider packages, Skill catalog/runtime, service container, migrations and HTTP routes. The cross-scenario tool test uses social post creation and a mail reply; the native-route composition verifies exact-number inputs reach the journal claim check. Tests use controlled gateways/providers and do not publish to live accounts.

The final backend build passed. Frontend type-check, the work-journal contract script and lint passed; lint reports 71 pre-existing warnings and zero errors. Independent review covered the generic contract, scope/permission boundaries, unknown-state recovery and UI isolation. No Next.js server was started.

PostgreSQL migration execution, deployed service restart, real social/mail account acceptance and browser visual acceptance remain deployment checks. The migration was validated by SQL-shape tests and SQLite domain tests, not applied to a live PostgreSQL instance during development.
