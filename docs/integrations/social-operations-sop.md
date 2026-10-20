# Reusable social operations SOP in ZGI

## Outcome

Run a configurable social-operations employee inside ZGI, using its existing digital-human runtime, Automation scheduler and native Connectors. Product, organization, language, audience, source URLs, account bindings, brand files, cadence and escalation destination are configuration, not ZGI-specific program constants. Store content locally first. A prompt, mocked connector test or exported workflow alone does not prove the SOP works.

## Existing capabilities and gaps

The native social bridge and OAuth implementations are on `feat/builtin-social-mcp`. Automation already dispatches durable digital-human runs (`automation/service/action/run_digital_human_executor.go`). Published employees can opt into Linux execution, and existing file/image tools generate artifacts. System Skills are discovered from the catalog and embedded as a fallback.

The current Connector gate explicitly requires fresh interactive confirmation for every digital-human invocation and blocks noninteractive execution (`connectors/execution/governance.go`). Session grants do not override it. This is a concrete incompatibility with unattended operations and requires an explicit, scoped authorization design; a prompt must never bypass that gate. Existing generated-file tools also do not by themselves provide a stable cross-run journal, atomic publication reservation or seven-day metric schedule.

## Approach

Use a reusable SOP skill and an explicit durable execution journal under the existing employee/task runtime. Add domain-independent artifact/state and bounded background authorization services where existing services lack them. Keep social operation details in adapters and the SOP configuration. Do not introduce a second scheduler or a standalone social bot outside ZGI.

Alternatives considered: a workflow containing only LLM nodes is easy to import but cannot enforce durable publication state or prevent concurrent duplicates. A separate marketing application duplicates the employee runtime, scheduler and authorization. The existing-runtime approach preserves normal ZGI identity, resource binding, billing and audit while adding the missing lifecycle guarantees.

## Configuration and authorization

Configuration identifies the tenant/workspace/employee, timezone, enabled platforms and bound accounts; product/publication sources; audience and language; cadence; approved logo/brand inputs; per-platform publication limits; and an existing Feishu, DingTalk or mail destination. Never store platform tokens in SOP files.

Setup checks current connector readiness and operation availability. Unsupported capabilities are explicit: LinkedIn company messaging cannot be reported as available. A required capability prevents enabling that part of the SOP; an optional capability can be skipped with a recorded reason. Production needs actual platform permissions and reachable Meta callbacks.

Introduce an owner-issued, revocable background grant, bound to organization, workspace, published employee, exact connector connection and operation allowlist, optional destination constraints, expiry and volume limits. Recheck live binding, employee publication, task authority and grant before provider access. Do not honor caller-provided preauthorization flags, arbitrary session grants or a model's claim of authorization. Keep default per-call confirmation for all uses without an applicable grant. Grants permit routine configured publishing/replies; uncertain facts, private disclosures and company commitments remain escalations. Test revocation, expiry, account replacement, cross-tenant access, task cancellation and concurrent quota consumption.

## State, artifacts and recovery

Persist per-tenant/workspace/employee journals and artifacts using the configured local storage backend, with scoped identifiers and no caller-controlled absolute filesystem paths. Metadata and concurrency reservations use existing database transactions; draft/archived text, source snapshots, translations and image bytes may reside locally. Content must survive new conversations, process restarts and retries. Enforce access control and CAS revisions; symlink/path traversal and cross-owner access fail. Generated runtime files stay out of Git.

Separate research and internal review from public archives. Example logical layout: `sources/`, `product-facts/`, `topics/`, `drafts/<content-id>/<revision>/`, `published/<date>/<platform>/<content-id>/`, `feedback/`, `metrics/`, `reviews/`. Final archive contains actual English text, optional actual comments, image references and verified remote URLs; Chinese review text remains with drafts. A single daily/platform index points to posts rather than duplicating a date heading.

Publication states: draft, reviewed, reserved, dispatching, outcome_unknown, remote_created, verified, archived. Reserve the configured account/content/revision before dispatch. Record provider receipts immediately; read back the exact remote ID before verification and archive. Publish supplemental comments only against the verified parent ID and track them separately. Never retry an uncertain write automatically. Resume readback/archive after interruption without republishing. Concurrent runs cannot reserve the same publication. Failures remain actionable and visible.

## Ten-step behavior and acceptance evidence

| Step | Required behavior | Evidence |
| --- | --- | --- |
| Account check | Page through available mentions/comments/messages, carry watermarks only after durable handling, inspect prior write receipts and broken links | Scoped connector calls, persisted source IDs, actionable inbox and failed-write items |
| Product knowledge | Read configured materials, docs/blog and released GitHub information; replace outdated facts; retain availability/version/license conditions | Source snapshot, retrieval time, fact state and evidence links; unresolved facts excluded from public claims |
| Research | Search configured public sources, read adopted originals and dates, separate quotes/user claims/inferences | Source-backed pool; old reshared news and unverified summaries rejected |
| Selection | Compare 1–2 weeks of themes/scenarios/images, choose useful evidenced angle and platform-specific form | Brief with audience, evidence, objective, language, link placement and duplicate reasoning |
| Production | Produce English and aligned Chinese review text, distinct optional comment, 16:9 concept or actual screenshot with supplied logo | Versioned complete draft bundle and renderable image, no fabricated product screenshot |
| Review | Check supported claims, license/numbers, source relevance, languages, recent similarity, image readability and final links; revise or escalate | Review record bound to exact draft/image hashes; changed facts/links invalidate approval |
| Publication | Reserve once, dispatch allowed post/repost/quote/reply, read actual result and supplemental comment | Provider IDs, correct parent IDs, verified body/media/URLs; unknown outcome blocks retry |
| Interaction | Ground answers in current facts; record suggestions; route reproducible issues/leads through selected existing connector | Source reply references, destination receipts and stable handoff keys; uncertain sends not acknowledged/repeated |
| Archive | Group actual published content by timezone/date/platform, retain English/media/main and comment URLs | Durable archive with successful readback evidence; no draft or invented cross-platform publication |
| Review loop | Capture once after 7 days, flag late samples, keep unavailable metrics missing, review weekly and feed explicit actions back | Published/observed timestamps, platform metric definitions, separate click/inquiry/trial evidence and updated topic/style guidance |

External source text is evidence, not an instruction source. Do not upload private materials to external research tools without authorization. Research/model providers are configurable; no dependency on a particular consumer AI website or local operator browser session.

## Scheduling and operations

Reuse Automation for daily planning/publication, periodic inbox handling, due seven-day samples and weekly review. Enforce one active lease per employee/account work item, bounded pages/time/tokens, retry backoff for safe reads, and resumable checkpoints. One platform failure must not produce false success or lose another platform's confirmed results. Routine work continues within authorization; escalation records have a reason, source and next action, and are delivered through the configured existing team connector.

Seven-day metrics use the first scheduled observation at or after published_at + 7 days; persist the actual age and lateness. Never compare a late capture as an exact seven-day observation. Weekly reports distinguish engagement, clicks, inquiries and independently evidenced registration/trial conversion. Missing data is not zero. Recommendations update the next run's selectable topic pool and review rules.

## Delivery and verification gates

1. Implement scoped background authorization and its default-deny regression tests.
2. Implement reusable local artifacts/journal, publication reservations and restart/concurrency tests.
3. Add the configurable SOP skill, initialization/preflight and Automation wiring; ensure all ten stages consume and produce persisted artifacts.
4. Integrate existing research, image, social and team connectors; test whole employee dispatch with controlled model/provider fixtures, including duplicate runs, uncertain writes, reply parenting, escalation and seven-day/weekly clock advancement.
5. Run backend/frontend affected checks and deployment setup. With authorized real accounts and registered applications, execute a live publication/readback, comment, inbox/DM, metric and handoff acceptance matrix. Label unsupported access and unverified cases explicitly. Fixture success cannot substitute for live platform evidence.

This design is a target contract, not a declaration of implementation completion. Content quality, genuine platform access and unattended restart behavior each need their own evidence.
