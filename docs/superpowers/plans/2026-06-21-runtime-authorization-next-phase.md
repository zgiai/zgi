# Runtime Authorization Next Phase Decision Brief

Date: 2026-06-21
Branch: `px/permission-system-overhaul`
Status: Decision brief for post-MVP review

Related:

- `docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md`
- `docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md`

## Current Locked Contract

The permission MVP keeps these runtime surfaces intentionally separate:

- `webapp`: public-compatible published webapp runtime. It can be enabled or disabled, but account/department audience grants are not enabled yet.
- `api`: public-compatible API-key runtime. The API key is the bearer capability once the surface is enabled.
- `builtin_app`: organization product surface. It supports organization, department, and account grants.
- `internal`: platform invocation surface for workflows, scheduled tasks, and internal calls. It remains enabled for compatibility and does not follow webapp/API exposure.

The current implementation rejects non-public `webapp` and `api` grants at the frontend, service, and storage boundaries. That is deliberate: enabling private webapp/API audiences before the identity handshake is chosen would make config visibility, runtime execution, migration, and API caller semantics disagree.

## Product Decisions Needed

### Webapp Audience

Decision question: should a published webapp be private to selected users/departments, or should webapp remain public while `builtin_app` controls internal app-center visibility?

Recommended direction: use a gated config/capability flow for private webapps, while preserving public config compatibility for existing public webapps.

Why:

- The current public config endpoint is used before a stable authenticated runtime identity exists.
- Optional auth on the same config endpoint is cheaper, but creates two meanings for the same response: anonymous users see less than authenticated users.
- A protected capability/config endpoint gives the frontend a clear state machine: public shell, login required, no access, offline, or runnable.

Minimum technical work:

- Keep `/console/api/webapps/:web_app_id/config` public-compatible for public/offline shell metadata.
- Add a protected runtime capability or config endpoint behind `WebAppAuthMiddleware`.
- Evaluate `runtimeauth.SurfaceAuthorization` for `webapp` against account and department audience.
- Add frontend states for login-required and no-access private webapps.
- Add regressions for anonymous public webapp, anonymous private webapp, authorized account grant, authorized department grant, denied account, and disabled surface.

### Webapp User Migration

Decision question: when anonymous webapp usage is migrated to an authenticated user, how should the system prove the migration belongs to the target webapp?

Recommended direction: add `POST /console/api/workflows/:web_app_id/migrate-user` and keep the existing global route as a compatibility bridge.

Why:

- The existing global `POST /console/api/workflows/migrate-user` route has no resource id and cannot evaluate a private webapp policy.
- Signed virtual-account proof can avoid a route change, but it introduces replay and expiry rules that still need to be bound to a resource.
- A resource-scoped route makes policy evaluation explicit and keeps the old route available during migration.

Minimum technical work:

- Register a resource-scoped migration route.
- Resolve the active webapp agent from `web_app_id`.
- Check `webapp` runtime authorization for the authenticated caller before migration.
- Preserve old-route behavior for public-compatible webapps until callers migrate.
- Add regressions for missing virtual/authenticated context, wrong webapp, denied private webapp, allowed private webapp, and same-account no-op compatibility.

### API Audience

Decision question: should API access remain a bearer capability, be scoped to the API key owner, or require an explicit caller claim per request?

Recommended direction: keep API keys as bearer capabilities for MVP+1, and postpone account/department API grants until the product is ready to require caller identity in API requests.

Why:

- Binding an API key to an owner account/department is easy to implement but ambiguous for shared keys and department changes.
- Requiring a caller claim is the strongest model, but it changes SDKs, external integrations, docs, signing, and error semantics.
- The current API-key path already has strong agent/workspace/key scoping. Changing audience semantics should be a planned API version or explicit compatibility mode.

Minimum technical work for a future private API model:

- Define whether the runtime audience comes from key owner metadata or a per-request caller claim.
- Add request/SDK documentation for caller claim format if claim-based.
- Decide department membership lookup timing: request-time live lookup or key-issued snapshot.
- Add regressions for disabled API surface, active legacy key, wrong key/agent scope, owner transfer, department removal, and caller-claim mismatch.

## Low-Risk Preparation Allowed Before Decisions

These changes can be made without changing current public-compatible behavior:

- Add read-only decision docs and route inventory updates.
- Add tests that prove non-public `webapp` and `api` grants are still rejected until the decision is made.
- Add small runtimeauth helper functions that are unused by public flows but covered by unit tests, as long as they do not broaden accepted grant subjects.
- Add frontend copy or disabled UI affordances that explain why webapp/API audience remains public-only.
- Improve browser/manual regression scripts for the current MVP surface contract.

Progress note, 2026-06-21:

- Store-level and agent-service-entry regressions now prove account/department-style grants for `webapp` and `api` are rejected before persistence. The management contract remains public-only for those surfaces until the private webapp/API decisions above are approved.
- The agent publication-access UI now states that account and department audience grants apply only to built-in app visibility. `pnpm test:route-access` also asserts the note remains rendered and that `webapp`/`api` updates keep sending public grants only.
- `runtimeauth` now has an explainable audience evaluation helper that returns stable allow/deny reasons while preserving the existing boolean `Allows` contract. It is covered by unit tests only and is not wired into webapp/API private access until the decisions above are approved.
- `GET /console/api/webapps/:web_app_id/capability` is registered behind `WebAppAuthMiddleware` as a public-compatible skeleton. It returns `public_only=true`, `private_audience_enabled=false`, and `supported_subject_types=["public"]`; it reuses the existing published webapp config gate and does not evaluate account/department webapp grants yet.

Avoid these until decisions are made:

- Accepting account or department grants for `webapp` or `api` writes.
- Returning private webapp config from the public config endpoint.
- Requiring login for existing public webapps.
- Binding existing API keys to a synthetic owner audience without a migration policy.
- Removing or changing the legacy global webapp migration route.

## Suggested Implementation Sequence After Approval

1. Webapp private capability skeleton:
   - Add protected webapp capability/config endpoint.
   - Keep public config response compatible.
   - Add backend regressions for public/private/offline/no-access.

2. Frontend private webapp states:
   - Add login-required/no-access/offline states.
   - Keep public webapp direct URL behavior unchanged.
   - Add route/browser smoke for anonymous and authenticated cases.

3. Resource-scoped user migration:
   - Add `/:web_app_id/migrate-user`.
   - Keep the global route as compatibility bridge.
   - Add dual-route regressions.

4. API audience design freeze:
   - Choose bearer-only, owner-audience, or caller-claim model.
   - If caller-claim is chosen, implement as a versioned API contract rather than changing existing keys silently.

## Completion Evidence For The Next Phase

The next phase should not be considered complete until all chosen decisions have direct evidence:

- Backend tests for accepted and denied audience cases.
- Frontend route/access tests for new private webapp states.
- Browser smoke covering public webapp, private allowed user, private denied user, and existing app-center visibility.
- API compatibility tests for existing keys.
- Updated endpoint inventory describing the chosen webapp/API audience contract.
- Docker zgi-c migration/health check if a schema or route behavior changes.
