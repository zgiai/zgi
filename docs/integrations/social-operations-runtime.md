# Social operations employee runtime

This runtime extends the existing DigitalHuman, Automation and Connector modules. Its target behavior is defined in [the SOP contract](social-operations-sop.md). The implementation is not accepted as a complete live SOP until the configuration/schedule setup and account acceptance matrix have also passed.

## Identity and background authority

The `social-operations` system Skill is available to digital employees. Its tools restore the active scheduled run, organization, workspace, employee, published version and task creator from database records. Tool arguments cannot choose an owner or workspace. Canceled tasks, inactive actions, superseded publications and inaccessible resources fail before credentials are opened.

Background grants are separate from conversation session grants. A connection owner with employee-update permission issues a grant for an exact published binding, selected operations, expiry and total invocation limit. Optional destination constraints match exact top-level Connector input fields. A value may match a string or an exact singleton string array (for mail recipient schemas); extra array recipients never match. Constraints apply only to named fields, so a complete mail handoff policy must also address other recipient fields supported by that operation. The server freezes binding/connection/credential revisions and the accepted catalog contract; changing those requires a new grant. Revocation and the current task/publication are checked on each invocation. Consumption is transactional and a logical invocation cannot consume another grant to repeat a dispatch. SOP runs also atomically consume their server-planned per-run native Connector call budget in the same transaction. The task's parent model uses the existing cumulative token-budget stop after actual model usage is reported; it can exceed the threshold by the final model response. Image inspection has a separate bounded vision request. Page, record and safe-read-attempt metadata remain guidance plus each tool's bounds, rather than cumulative server meters.

Authenticated APIs use the current organization/workspace/account context:

- `POST /console/api/connector-target-bindings/{bindingId}/background-grants`, with `operations`, optional `destinations`, `expires_at` (RFC3339, at most 90 days ahead), and `max_invocations` (1–100000).
- `GET /console/api/connector-background-grants` lists the current owner's grants within the workspace.
- `DELETE /console/api/connector-background-grants/{grantId}` revokes an owned grant.

Every background Connector invocation carries a server-owned exact run/message identity. Public social writes additionally require a private dispatch claim bound to the journal publication ID/revision and canonical input digest; SOP private replies and team sends require the corresponding outbox claim. Raw native tools cannot use the same grant to bypass these journal checks.

Use the canonical published Connector target-binding ID, not the employee resource-binding ID. The request never supplies employee/version/owner/revision snapshots. Missing grants retain the existing confirmation boundary. A trusted-low risk classification alone does not grant unattended access.

## Local content and publication recovery

Local artifacts live in `social-operations` alongside the configured OpenDAL base directory; with the default `./storage/opendal`, this is `./storage/social-operations`. They are outside the OpenDAL file root. Filenames are server-generated UUIDs. Database records index the organization/workspace/owner/employee, MIME type, size and SHA-256; text and images are not committed to Git. Back up the database and this directory together. Preserve both across restarts and make them available to all workers assigned this journal. A container ephemeral filesystem is insufficient for durable operation.

`save_record` stores versioned source/fact/topic/draft/review/feedback/checkpoint/weekly-review/writing-rule documents. Canonical configuration is managed through the authenticated setup API; model tools cannot modify it or fabricate archive records. `read_record`, `list_records` and `read_artifact` retrieve them under the restored scope. Updates require the expected revision. An artifact ID grants no cross-owner access. Local reads check actual bytes, size and hash.

The authenticated `POST /assets/images` setup route accepts bounded PNG/JPEG/WebP uploads into the private journal for real brand inputs. Image decoding checks dimensions before allocating pixels and limits inputs to twelve million pixels. `compose_image` uses approved logo pixels and literal text to render a consistent 1600x900 asset, with an optional scoped background. It checks font glyph availability and text bounds; unsupported scripts or overlong text fail explicitly. It does not regenerate a logo or silently clip text, and it does not itself prove that a screenshot or claim is factually accurate.

`inspect_image` restores the same active employee scope, reads the actual image and approved logo bytes privately, and calls the organization's available vision model. It saves the returned findings in a scoped review artifact and returns its ID, without exposing base64, local paths or a public image URL. Its server-only inspection index binds image ID, digest, scope and configuration revision; configured publication review and reservation reject missing or failed approval. When more than three brand references exist, select the approved `logo_artifact_id` explicitly so the actual logo reference is included. Model review is not proof of product facts. Required-image preflight checks the vision model and the current compositor's English text support. Internal Chinese review remains supported.

`import_image` accepts an accessible generated file ID from the current account/conversation. It validates real JPEG/PNG/WebP bytes and prepares a bounded image before storing a local artifact. It does not fetch arbitrary URLs or accept filesystem paths. Protected Linux execution artifacts use a separate verified-subject reader that rechecks the live task/publication and exact run message before any binary read. They never fall back to the billing account identity.

`prepare_publication` accepts the exact configured `account_id`, connection, platform, content key and immutable draft artifact. Configured publications bind the current configuration revision. Reservation serializes on the configuration row, verifies the exact connection/account, enforces the configured daily publication limit in its timezone, and rejects the same normalized public text/action/target within fourteen days. Replies do not consume the daily post limit; connector grants still bound their total calls. These exact text checks complement editorial similarity review and do not claim semantic deduplication. Configuration changes block old draft/reservation admission. An unsubmitted previous-day reservation can safely return to review and obtain a fresh daily reservation; dispatching and unknown states never use that path.

A JSON draft contains `action` (`post`, `reply`, `quote`, `repost`), `english`, `chinese`, optional `target_id`, image artifact IDs and source artifact IDs. Chinese review text is never sent publicly. `review_publication` binds review to the actual draft hash. `replace_draft` clears review and is forbidden once submission starts. Supplemental comments use `parent_publication_id`; the parent must be verified under the same scope, account and connection. General discussion replies use a target independently checked through the platform.

`dispatch_publication` takes only the publication ID. It reserves the content, calls the native Connector once, stores the provider ID before readback, verifies actual author/text/parent/media/linkage, then archives actual public text and media references. The archive timezone is frozen from the configuration when preparing the publication and cannot be supplied by the model. The gateway remains on the ordinary native authorization/audit path and never opens platform tokens itself. Current image publication supports one image; unsupported combinations fail explicitly.

Recovery behavior:

- A reserved operation may resume with a compare-and-swap transition to dispatching.
- A proven pre-submission rejection may return to reviewed and use a fresh native attempt after repair.
- A timeout or missing response is not proof of rejection. It leaves dispatching or outcome_unknown and is never resent automatically.
- A known provider ID resumes readback/archiving without repeating the write.
- `reconcile_publication` can verify an explicit known remote ID through a fresh read for uncertain text publications. Missing media-upload receipts and uncertain reposts require investigation; similarity of text is not sufficient proof.
- An archived record can rebuild its date index from actual artifacts even if its internal draft is unavailable.

`write_daily_archive` rebuilds one date/platform index from verified actual content, with separate public body, image and main/comment links. It uses one versioned date record, excludes Chinese review text and research, and retains local/remote image references. Every platform has its own actual entry.

## Private replies and team handoffs

The separate outbox uses `prepare_outbound`, `review_outbound`, `dispatch_outbound`, and `read_outbound`. Final reply bodies are immutable artifacts; DM bodies include internal Chinese review text that is never sent. Handoff connection, operation and destination come from the canonical configuration. A stable source-based work key, CAS transitions and a durable dispatching state prevent an ordinary retry from repeating the send. A missing receipt or transport uncertainty becomes outcome_unknown and requires investigation. Only authoritative not-submitted evidence permits another attempt. Completed means the provider returned a checked acceptance receipt, not recipient read status. Private content stays outside the public daily archive.

Gmail, QQ Mail and 163 Mail are admitted through separately versioned DigitalHuman surface projections. Their original V5/V6 mail credential dialect, authentication and operations remain intact. Existing mailbox credentials can be reused; the employee still needs an explicit published binding and a bounded grant with an exact recipient. Feishu and DingTalk likewise use their existing native connectors.

## Metrics and next-cycle guidance

`due_metrics` lists verified posts at least seven days old with no captured observation. `capture_metrics` uses the native per-post metric operation, saves the actual observation time and age, and preserves the first sample on later calls. Any lateness is recorded; missing metrics remain null or absent, never zero. Count data is available in `values_json` to preserve integer representation. Counters remain platform-specific. Neither a click nor an inquiry is treated as a registration, trial or sale.

`list_publications` pages scoped verified publication history and the preserved metric samples over an explicit publication-time range of at most 31 days. It supplies the durable IDs required for review without refreshing counters; missing samples remain absent.

The Skill instructs weekly review to persist concrete next actions into topic and writing-rule records. Those records are inputs to the next run. Guidance alone is not evidence that a weekly run occurred: require the scheduled run result, observed metric records and updated artifacts.

## Setup and recurring work

Digital employee settings include a configuration form, capability preflight and explicit setup action. Authenticated APIs use `/console/api/digital-humans/{id}/social-operations`:

- `GET /accounts` returns only provider-verified accounts from the current owner’s published bindings. LinkedIn choices use observed organization evidence; old connections may need re-verification.
- `GET /configuration` returns the current scoped configuration and CAS record, or null when not configured.
- `PUT /configuration` accepts `{expected_revision, configuration}`. Settings omit owner/workspace/employee/version authority; the server restores those from the account and current published employee. Source/brand artifacts must belong to that scope.
- `POST /preflight` accepts `{}` and reports current account, operation and grant readiness, plus prerequisites. Preflight does not consume grant quota or guarantee future provider entitlement.
- `POST /setup` accepts the exact configuration revision and employee version. It checks current preflight before each task creation and reuses stable operation IDs across retries.

Four existing Automation tasks cover daily content, frequent inbox processing, due seven-day observations and weekly review. Their source snapshots store the configuration record/artifact/revision, published employee version, stage list and budgets as server-owned metadata. The runtime rejects a changed configuration/version rather than using prompt text as authority. Setup pauses superseded SOP schedules before creating the replacement schedules; existing run history remains available. Existing task screens provide run history, pause and cancellation. A partial setup retry preserves already-created tasks without duplicates; setup is not a single transaction across all four task creations.

The canonical `research` setting selects one exact published Connector ID and connection ID. Preflight requires live search/fetch operations and bounded grants on that connection; another authorized research connection is not a fallback. The published employee model is checked through the organization's existing model resolver rather than merely checking that its name is filled in.

LinkedIn managed company account IDs are observed through the native bridge during connection verification, retained with credential-scoped provider evidence, and checked again through live ACL reads before writing. Existing connections need a successful re-test or reauthorization to populate that evidence. No model-entered company ID becomes ownership evidence.

## Acceptance

Unit, runtime-projection, bridge-fixture and gateway/journal composition tests validate specific code paths. They do not grant provider access or prove real account behavior. Before live acceptance, apply migrations, reconnect accounts for newly admitted operations, issue explicit grants, configure reachable Meta callbacks, configure the research connection, published employee model, organization vision model and approved brand inputs, and install the SOP task schedules. An image generator is optional for additional backgrounds; the deterministic compositor does not require it. Verify actual posts, comments, inbox access, metrics and the selected Feishu/DingTalk/mail handoff against the corresponding account.

LinkedIn company messaging remains unavailable in the public integration. Platform permissions, supported forms and metrics differ. Preflight must report those differences and cannot label an unsupported required capability ready.
