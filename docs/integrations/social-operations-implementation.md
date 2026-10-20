# Social operations SOP implementation plan

> Execute with superpowers:subagent-driven-development. The approved contract is `social-operations-sop.md`.

**Goal:** Run the complete configurable social SOP through ZGI digital employees and Automation with durable local content, scoped unattended authority and verified external effects.

**Architecture:** Preserve the native Connector dispatch path and Automation runtime. Add server-owned background grants and a scoped durable work journal; expose their admitted functions through normal employee tools. A reusable SOP skill coordinates product/research/review/social/image/team capabilities. Final acceptance includes actual runtime/provider evidence, not just prompt assertions.

**Stack:** Go, GORM/PostgreSQL, existing local storage and tool runtime, native social MCP bridge, existing Automation, Skills.

## 1. Bounded background grants

- [ ] Create `connectors/backgroundauth` service/model/tests. Owner-issued grant binds organization, owner, workspace, employee published version, binding, connection, credential/policy revision, operation allowlist and catalog contract; requires expiry and a total invocation limit. Store allowed destination fields as explicit exact values. Reject wildcard identities and empty constraints. Consumption is transactional and deduplicated by native logical operation identity; revocation prevents further consumption. Preserve audit correlation without storing credentials.
- [ ] Write failing tests first for valid use, default deny, cross-owner/version/account/operation, expiry/revocation, destination substitution, exhausted limit and concurrent consumption. Run `go test ./internal/modules/connectors/backgroundauth -count=1`.
- [ ] Add migration and native execution integration. Recheck existing target/binding/current task eligibility before grants, then consume before credentials/provider. Preserve per-call behavior without a matching grant; never accept session/caller flags as background authority. Regression-test existing DigitalHuman confirmation and unknown outcomes. Add authenticated owner grant management endpoints and container wiring.

## 2. Durable journal and local content

- [ ] Create `socialops` service/model/tests with server-scoped organization/workspace/owner/employee identity. State has CAS revision, artifact references, content IDs, platform/account binding, phase and durable source/receipt IDs. Use configured local files for text/images with immutable digest references; deny path traversal/symlinks. Database reservations and transitions are authoritative; no locks held across remote network calls.
- [ ] Write failing tests for persistence across service restart, cross-scope denial, CAS conflict, concurrent reservation, uncertain-write blocking, exact parent linkage, complete archive and seven-day sample eligibility. Add migration and test it on SQLite/PostgreSQL-compatible schema.
- [ ] Implement publication transitions draft → reviewed → reserved → dispatching → remote_created → verified → archived, with outcome_unknown as a stop for writes. Review binds immutable bundle hash; a changed bundle needs review again. Archive references verified actual content, not draft assumptions.

## 3. Reusable employee tools and SOP skill

- [ ] Add a focused built-in journal tool provider, registered in `internal/container/builtin_tools.go`, with server-restored employee identity. Add a system `social-operations` skill and reference contracts under `skills/catalog/`. Catalog discovery and runtime projection tests must prove tools are available only in the proper scoped execution.
- [ ] Skill covers all ten stages, original-source evidence, product lifecycle/license checks, topic similarity, English/Chinese parity, image/logo rendering and review, link checks, platform-specific formats, replies, escalation, actual archive and metrics-driven next actions. Inputs describe a product generically; ZGI is an example configuration only.
- [ ] Use existing product/public search/fetch, image and Connector tools. If an adapter cannot supply the needed image or original source, stop that content item with an actionable result rather than fabricate an artifact or silently omit the step.

## 4. Automation and capability preflight

- [ ] Add setup/preflight and durable task wiring for daily planning/publishing, periodic inbox processing, seven-day samples and weekly review. Reuse Automation models/services. Setup returns actual enabled/unsupported/missing capabilities and exact account bindings; cannot mark LinkedIn company DM ready.
- [ ] Enforce bounded work and one active lease for each work item. Persist pagination checkpoints after successful handling; handoff IDs correlate source and existing Feishu/DingTalk/mail delivery. Unknown sends never trigger blind retry or source acknowledgement.
- [ ] Exercise clock-driven schedules and employee restarts through the real Automation dispatch interface with controlled model/provider fixtures. Assert persisted artifacts and actual provider invocation counts for every numbered SOP stage.

## 5. Integration and acceptance

- [ ] Add frontend setup/authorization controls using existing connector and employee settings patterns; do not start Next.js during development unless requested.
- [ ] Verify package tests, native bridge tests, affected TypeScript checks, secret safety, migrations and backend build. Review spec coverage separately from code quality.
- [ ] Execute actual registered-account acceptance for authorization/renewal, publication/readback/media/comment parenting, inbox/DM, analytics, webhook and team delivery. Report unavailable provider entitlements explicitly. Keep the goal active if any required functionality lacks authoritative evidence.

Progress checkboxes track verified completion only. Dependencies: native background integration follows grant core; employee tools and Automation follow journal core. Independent core services can be implemented concurrently without editing shared container/router files.
