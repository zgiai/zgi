# Social operations SOP implementation plan

> Execute with superpowers:subagent-driven-development. The approved contract is `social-operations-sop.md`.

**Goal:** Run the complete configurable social SOP through ZGI digital employees and Automation with durable local content, scoped unattended authority and verified external effects.

**Architecture:** Preserve the native Connector dispatch path and Automation runtime. Add server-owned background grants and a scoped durable work journal; expose their admitted functions through normal employee tools. A reusable SOP skill coordinates product/research/review/social/image/team capabilities. Final acceptance includes actual runtime/provider evidence, not just prompt assertions.

**Stack:** Go, GORM/PostgreSQL, existing local storage and tool runtime, native social MCP bridge, existing Automation, Skills.

## 1. Bounded background grants

- [x] Create `connectors/backgroundauth` service/model/tests. Owner-issued grant binds organization, owner, workspace, employee published version, binding, connection, credential/policy revision, operation allowlist and catalog contract; requires expiry and a total invocation limit. Store allowed destination fields as explicit exact values. Reject wildcard identities and empty constraints. Consumption is transactional and deduplicated by native logical operation identity; revocation prevents further consumption. Preserve audit correlation without storing credentials.
- [x] Write failing tests first for valid use, default deny, cross-owner/version/account/operation, expiry/revocation, destination substitution, exhausted limit and concurrent consumption. Run `go test ./internal/modules/connectors/backgroundauth -count=1`.
- [x] Add migration and native execution integration. Recheck existing target/binding/current task eligibility before grants, then consume before credentials/provider. Preserve per-call behavior without a matching grant; never accept session/caller flags as background authority. Regression-test existing DigitalHuman confirmation and unknown outcomes. Add authenticated owner grant management endpoints and container wiring.

## 2. Durable journal and local content

- [x] Create `socialops` service/model/tests with server-scoped organization/workspace/owner/employee identity. State has CAS revision, artifact references, content IDs, platform/account binding, phase and durable source/receipt IDs. Use configured local files for text/images with immutable digest references; deny path traversal/symlinks. Database reservations and transitions are authoritative; no locks held across remote network calls.
- [x] Write failing tests for persistence across service restart, cross-scope denial, CAS conflict, concurrent reservation, uncertain-write blocking, exact parent linkage, complete archive and seven-day sample eligibility. Add migration and test it on SQLite/PostgreSQL-compatible schema.
- [x] Implement publication transitions draft → reviewed → reserved → dispatching → remote_created → verified → archived, with outcome_unknown as a stop for writes. Review binds immutable bundle hash; a changed bundle needs review again. Archive references verified actual content, not draft assumptions.

## 3. Reusable employee tools and SOP skill

- [x] Add a focused built-in journal tool provider, registered through the `ServiceContainer` tool manager, with server-restored employee identity. Add a system `social-operations` skill and reference contracts under `skills/catalog/`. Catalog discovery and runtime projection tests must prove tools are available only in the proper scoped execution.
- [x] Skill covers all ten stages, original-source evidence, product lifecycle/license checks, topic similarity, English/Chinese parity, image/logo rendering and review, link checks, platform-specific formats, replies, escalation, actual archive and metrics-driven next actions. Inputs describe a product generically; ZGI is an example configuration only.
- [x] Use existing product/public search/fetch, image and Connector tools. If an adapter cannot supply the needed image or original source, stop that content item with an actionable result rather than fabricate an artifact or silently omit the step.

## 4. Automation and capability preflight

- [x] Add setup/preflight and durable task wiring for daily planning/publishing, periodic inbox processing, seven-day samples and weekly review. Reuse Automation models/services. Setup returns actual enabled/unsupported/missing capabilities and exact account bindings; cannot mark LinkedIn company DM ready.
- [ ] Enforce bounded work and one active lease for each work item. Persist pagination checkpoints after successful handling; handoff IDs correlate source and existing Feishu/DingTalk/mail delivery. Unknown sends never trigger blind retry or source acknowledgement.
- [ ] Exercise clock-driven schedules and employee restarts through the real Automation dispatch interface with controlled model/provider fixtures. Assert persisted artifacts and actual provider invocation counts for every numbered SOP stage.

## 5. Integration and acceptance

- [x] Add frontend setup/authorization controls using existing connector and employee settings patterns; do not start Next.js during development unless requested.
- [x] Verify package tests, native bridge tests, affected TypeScript checks, secret safety, migration SQL contracts and backend build. Review spec coverage separately from code quality.
- [ ] Execute actual registered-account acceptance for authorization/renewal, publication/readback/media/comment parenting, inbox/DM, analytics, webhook and team delivery. Report unavailable provider entitlements explicitly. Keep the goal active if any required functionality lacks authoritative evidence.

Progress checkboxes track verified completion only. Dependencies: native background integration follows grant core; employee tools and Automation follow journal core. Independent core services can be implemented concurrently without editing shared container/router files.

## Verification and remaining acceptance

Implemented: scoped grants and atomic call quotas; immutable local artifacts; review/publication/outbox state machines and exact native dispatch claims; 21 admitted employee tools; image composition and server-enforced vision approval; explicit research/account/handoff selection; configuration preflight and four idempotently provisioned Automation schedules. Mail uses a separately versioned DigitalHuman projection while retaining its V5/V6 credential dialect.

Controlled tests cover the real Automation task/action executor with a scripted employee runner, journal/provider tools and fixture platform responses. They are not a live LLM or registered social-account acceptance run. The runtime implementation and its limits are documented in [social-operations-runtime.md](social-operations-runtime.md).

Remaining acceptance items:

- Apply and verify migrations on the deployment PostgreSQL database. Current validation covers migration SQL contracts and SQLite-backed domain tests.
- Authorize actual accounts and required vendor permissions, then verify posting/readback/media/comments, X/Facebook messaging, metrics, token renewal, Meta callbacks and the selected team handoff. The current public integration does not offer LinkedIn company messaging.
- Run the installed cron schedules with the configured employee/model and verify restart recovery and content/source quality. The controlled Automation test uses RunNow rather than claiming a live clock-driven production cycle.
- Connector call quotas and execution timeouts are enforced; the parent agent uses the existing cumulative model token stop. Page/record/safe-read-attempt plan metadata are guidance plus each tool's own bounds, and vision inspection has a separate bounded request. They are not cumulative server meters.

The partially unchecked work/acceptance bullets above are intentional. Passing development tests does not close these live acceptance items.

Development verification completed on 2026-10-20:

- All Connector module tests, `socialops`, the 21-tool provider, Skill catalog/runtime, Automation task/action modules, DigitalHuman, ChatRuntime, container, v1 routes and migration tests passed. The full run exposed two clock-related failures and outdated preflight fixtures; these were corrected and the affected packages rerun successfully.
- The mail DigitalHuman projection additionally passed catalog, binding, runtime authorization and execution regression tests; container and route tests passed after its integration.
- Native social MCP bridge: 45/45 tests passed, including stdio startup.
- Web TypeScript check, OAuth/social/background-grant/form contract scripts and i18n checks passed. ESLint completed with 0 errors and 71 existing warnings.
- Backend server build passed. No Next.js server was started.

These are development checks with controlled fixtures. The live account, production database, real cron/restart and content-quality acceptance items remain open.
