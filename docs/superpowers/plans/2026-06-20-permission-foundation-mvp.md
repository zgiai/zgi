# Permission System Foundation MVP Plan

Date: 2026-06-20
Branch: `px/permission-system-overhaul`
Status: MVP foundation ready for first backtest

Related inventory: `docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md`

## Goal

Build the minimum durable permission foundation needed for the product-user and builder split.

The MVP should make organization membership, workspace asset access, and published runtime access explicit and separate. It should also make "organization member without a workspace" a supported state, so later product features can be added without piling more workspace-context assumptions into routes, services, and UI state.

## Current Problems

1. Organization membership and workspace access are conflated.
   - The data model can represent organization members without workspace membership, but services and frontend state often treat missing workspace as an error.
   - `account_contexts.current_workspace_id` is nullable, yet context repair code tries to repopulate it whenever an accessible workspace exists.

2. Frontend console access is globally gated by workspace.
   - The console shell requires an active workspace before rendering children.
   - `/console/work` also requires `currentWorkspace` and `workspace.view`, which blocks `chat`, `image`, `app`, and settings-like product usage for organization-only members.
   - `useJoinedWorkspaces` can automatically select and persist the first workspace, which prevents an intentional no-workspace mode.

3. Workspace permissions are not consistently enforced.
   - Some paths use `WorkspacePermissionCode`; others use creator checks, owner/admin checks, or simple membership checks.
   - Some compatibility branches allow access when organization context is missing or cannot be found.
   - Workspace permission filtering accepts a requested permission type but currently behaves mostly like "is member of workspace".

4. Asset detail APIs rely too much on bare IDs.
   - Lists often filter by workspace, but details and subresources frequently load by `agent_id`, `dataset_id`, `document_id`, `data_source_id`, `table_id`, or `file_id` and rely on later checks.
   - High-risk areas are datasource tables/records, file preview/delete/folder moves, and dataset document/segment/status/retry paths.

5. Published runtime access is not modeled as its own permission surface.
   - Web app access is mainly `web_app_status`.
   - API service access has `enable_api` and API keys, but API key validation does not fully act like a published surface policy.
   - Built-in app visibility is not controlled per user or department.
   - Internal invocation should remain based on published availability, not webapp/API exposure, but this boundary is not explicit.

6. Naming and legacy compatibility increase risk.
   - `tenant_id`, `workspace_id`, `organization_id`, `group_id`, and department concepts are mixed across modules.
   - `workspace_members.current` and `account_contexts.current_workspace_id` can diverge.

## Target Permission Boundaries

1. Organization scope
   - User is an active member of an organization.
   - Allows product-level surfaces such as chat, image, app runtime, and personal/account settings.
   - Does not imply access to any workspace asset.

2. Workspace scope
   - User can access one concrete workspace in one organization.
   - Required for builder and asset-management surfaces: agents, workflows, knowledge bases, databases, files, prompts, and workspace settings.
   - Must always validate organization, workspace, account, and required permission code.

3. Published runtime scope
   - User or caller can invoke a published surface.
   - Web app, API service, built-in app, and internal invocation must be separate surfaces.
   - Internal invocation remains compatible with existing published agent/workflow behavior and is not disabled by turning off webapp or API exposure.

## MVP Scope

### Backend Foundation

- Add a central authorization layer with explicit methods:
  - `RequireOrganizationMember`
  - `RequireOrganizationRole`
  - `CanUseOrganizationFeature`
  - `CanWorkspace`
  - `ListWorkspaceIDsByPermission`
- Introduce clear scope structs or equivalent service contracts:
  - `OrganizationScope`
  - `WorkspaceScope`
  - `AssetAccessScope`
- Make missing organization or missing workspace fail closed in workspace checks.
- Replace or wrap weak permission entry points:
  - workspace permission filter must honor requested permission code
  - shared resource permission service must delegate to the new authorization layer
  - organization admin access to workspace must not depend on `workspace_members` rows

### Account Context

- Treat `current_workspace_id = null` as a valid explicit state.
- `GET /console/api/account/context` returns a derived `mode` field:
  `workspace`, `organization`, or `none`.
- `PUT /console/api/account/context` accepts explicit `mode`:
  - `mode=workspace` requires `current_workspace_id` and preserves legacy workspace switching compatibility.
  - `mode=organization` requires `current_organization_id` and clears `current_workspace_id`.
  - `mode=none` clears both organization and workspace context for recovery/fallback flows.
- Stop automatic workspace fallback when the user or profile is in organization-only mode.
- Clear and sync both modern and legacy current workspace fields when leaving workspace mode.
- Preserve organization context without forcing workspace context.

### Account Capabilities Contract

- `GET /console/api/account/capabilities` is the frontend route-guard contract for the current account context.
- The response separates:
  - `organization.product_surfaces` for chat, image, app, and settings.
  - `workspace.permissions` for concrete workspace asset permissions.
  - `routes.organization_scope_allowed`, `routes.workspace_scope_allowed`, and `routes.workspace_required` for guard decisions.
  - `runtime_surfaces.webapp|api|builtin_app|internal` as compatibility mode metadata for the published-runtime model.
  - `runtime_surfaces.*.grant_subject_types` and `runtime_audience.subject_types` as the backend/frontend contract for public, organization, account, department, and internal published-runtime grant decisions.
- Organization members without a current workspace receive organization product capabilities, an empty workspace permission list, and `workspace_required=true`.
- Workspace routes are allowed only when the selected workspace is accessible and `workspace.view` is present, or the account is an organization admin/owner.
- `useAccountPermissions` remains a workspace permission hook and no longer fabricates workspace view permissions in organization-only mode.

### Asset Authorization Hardening

Add scoped load or authorization helpers before expanding feature behavior:

- `GetAgentScoped` or `CanAgent`
- `GetDatasetScoped`, `GetDocumentScoped`, or `CanDatasetDocument`
- `GetDataSourceScoped`, `GetTableScoped`, or `ValidateDataSourceTable`
- `GetFileScoped` or `AuthorizeFile`

MVP priority:

1. Datasource detail/table/record/prompt/import paths.
2. File preview/delete/metadata/folder move/archive/favorite/resource paths.
3. Dataset document/status/retry/segment paths.
4. Agent detail/config/update/published grant paths.

### Frontend Foundation

- Add a central route/action access map.
- Split guards:
  - product use guard
  - workspace resource guard
  - organization admin guard
- Allow organization-level product routes to render without current workspace:
  - `/console/work/chat`
  - `/console/work/image`
  - `/console/work/app`
  - account-level settings
- Keep workspace asset routes blocked without workspace.
- Stop `useJoinedWorkspaces` from auto-persisting a fallback workspace.
- Make runnable apps request an organization/published-access list instead of requiring `workspaceId`.

### Publication Boundary Prep

The MVP should define the contract for published surfaces, but does not need the full management UI.

- Add or design the compatibility layer for:
  - web app enabled
  - API service enabled
  - built-in app enabled
  - target grants by organization, department, or account
- Preserve compatibility:
  - `web_app_enabled` maps from `web_app_status == active`
  - `api_service_enabled` maps from existing `enable_api`
  - existing valid API keys keep working after migration
  - internal invocation is not gated by webapp/API surface state

## Not In MVP

- Full department/user audience restriction UI for webapp/API published surfaces.
- Complete replacement of every legacy `tenant_id` name.
- Full marketplace/plugin publication redesign.
- Broad visual redesign of console navigation.
- Public documentation updates, unless behavior is already implemented and stable.

## Implementation Tasks

- [x] Write initial route and endpoint permission inventory.
- [x] Add backend scope and authorization contracts.
- [x] Update account context behavior to support explicit no-workspace mode.
- [x] Make workspace permission checks fail closed for missing organization/workspace context.
- [x] Update workspace permission filtering to use real permission codes.
- [x] Add targeted regression tests for workspace self-routes resolving the route workspace organization before update/statistics permission checks.
- [x] Route legacy workspace member handler permission checks through workspace-derived organization scope.
- [x] Reject cross-organization workspace IDs before organization workspace management permission checks.
- [x] Make legacy workspace permission service checks fail closed for empty or missing organization scope.
- [x] Harden datasource detail/table/record/import scoped access foundation.
- [x] Harden datasource prompt/audit remaining scoped access.
- [x] Harden file preview/delete scoped access foundation.
- [x] Harden MinerU/document image preview storage-key and local-path validation.
- [x] Harden remaining file folder move/archive/favorite/resource scoped access foundation.
- [x] Harden dataset document/segment scoped access foundation.
- [x] Harden dataset folder/move scoped access foundation.
- [x] Allow organization-level product routes to render without current workspace.
- [x] Add targeted service regression tests for AIChat product conversations in organization mode and scoped history/message access.
- [x] Add frontend access metadata and split route guards.
- [x] Stop frontend automatic workspace fallback.
- [x] Add explicit account context mode API and frontend organization-mode switcher entry.
- [x] Add backend account capabilities contract and make frontend work route guard consume it.
- [x] Add compatibility contract for published runtime surfaces.
- [x] Add minimal published runtime surface/grant storage migration and read contract.
- [x] Add minimal agent runtime surface write API and make webapp/API runtime consume persisted surface state.
- [x] Add minimal built-in workflow runtime surface read/write API for organization-admin managed builtin app grants.
- [x] Add minimal frontend management entry for built-in workflow `builtin_app` grants.
- [x] Add minimal frontend management entry for agent `webapp`/`api`/`builtin_app` runtime surfaces.
- [x] Add reusable organization member search and department-tree selector for runtime account/department grants.
- [x] Add targeted regression tests for account context, workspace permission filtering, authorization scope, and datasource scope foundation including prompt/audit paths.
- [x] Add targeted regression tests for file preview/delete, file folder/favorite visibility, and dataset document/segment/folder scoped access.
- [x] Add targeted regression tests for remaining publication and frontend access-map paths.
- [x] Add targeted regression tests for published workflow webapp conversation continuation ownership checks.
- [x] Add targeted handler regressions for published workflow webapp conversation detail/delete ownership checks before message/delete operations.
- [x] Harden async workflow webapp conversation title generation behind agent/caller scoped conversation access.
- [x] Harden workflow conversation metadata helpers for parent-message and dialogue-count reads behind agent/caller scoped conversation access.
- [x] Add targeted handler regressions for the legacy webapp user-migration endpoint contract.
- [x] Add targeted regression tests for advanced-chat run conversation continuation ownership checks.
- [x] Add targeted regression tests for generic draft/published conversation workflow run `sys.conversation_id` ownership checks.
- [x] Add targeted regression tests for AGENT runtime run step `message_id` ownership checks.
- [x] Add targeted regression tests for workflow-run list denying missing `agent.view` before query binding or service work.
- [x] Add targeted regression tests for builder agent history `runtime-logs` denying missing `agent.view` before JSON body binding.
- [x] Add targeted regression tests for console-agent and published-webapp AGENT runtime event/regeneration/continuation caller-scope checks.
- [x] Add targeted regression tests for console-agent and published-webapp AGENT runtime conversation update caller-scope checks before body binding.
- [x] Add targeted regression tests for console-agent and published-webapp AGENT runtime conversation delete, message-list, and stop caller-scope checks before lower-level runtime service calls.
- [x] Add targeted regression tests for console-agent and published-webapp AGENT runtime event stream caller-scope checks before `message_id` query validation.
- [x] Add targeted regression tests for AGENT runtime-runs builder log views requiring workspace `agent.view` before chat-runtime queries.
- [x] Add targeted regression tests for AGENT runtime-runs list denying missing `agent.view` before query binding.
- [x] Route draft workflow management handlers through the unified route-agent workspace permission preflight and lock body/service-order regressions.
- [x] Add targeted regression coverage for console AGENT draft chat requiring route-agent management before chat body binding.
- [x] Add targeted regression coverage for workflow-test mutation routes requiring route-agent management before request body binding.
- [x] Harden prompt optimize/playground builder support tools behind current-workspace permissions before request body binding or model setup.
- [x] Harden content-parse developer playground behind current-workspace `workspace.view` while preserving internal HMAC routes.
- [x] Harden agent API key management routes behind route-agent workspace `agent.view/manage` and resolved workspace repository scope.

## Current Foundation Notes

- `interfaces.AuthorizationService` is available as the central contract for organization and workspace scope checks.
- `shared/service.NewAuthorizationService` currently delegates to the existing organization service and fails closed for missing or archived workspace scope.
- `ServiceContainer.GetAuthorizationService` exposes the shared service for incremental adoption by asset modules.
- Datasource creation, management, prompt, operation-log, and SQL-audit paths now use explicit organization/workspace/account scope. Datasource detail/delete/table/record/import foundations validate organization scope, non-empty workspace scope, table-to-datasource ownership, and Excel import job-to-datasource ownership before returning or mutating data. Datasource table listing/detail/columns/record-query/template-download GETs require route datasource `database.view` before table/record/template lookup/listing, while datasource delete and table delete require route datasource `database.manage` before mutation. Datasource create pre-reads only `workspace_id` and requires target-workspace `database.manage` before full create DTO binding; SQL-audit workspace/detail routes use the shared workspace database permission helper before query binding/list/count/detail work. Datasource update, table create/update, table columns update, table prompt upsert, analyze-file, and Excel import analyze/recognize/confirm mutation handlers now require `database.manage` before binding full JSON bodies; file parse/text extraction/single-file ingest/batch ingest pre-read `table_id`, resolve table-to-datasource, and require `database.manage|database.data_edit` before full JSON binding or service work. Excel import job read requires `database.view` before job lookup, and import error listing requires `database.manage`; record add/update/delete/import requires `database.manage|database.data_edit` before binding record payloads or validating record-id/file query shape, keeping permission denial ahead of datasource-update/datasource-delete/datasource-create-shape/analyze-file-shape/table-list/table-detail/table-columns-read/table-record-query/table-template-download/table-create/table-update/table-delete/table-columns-update/prompt-upsert/record-add/record-update/record-delete/record-import/table-file-ingest-shape/excel-import request validation.
- Datasource service intentionally keeps published/runtime database tool compatibility separate from HTTP workspace permission checks; binding-grant runtime paths still need an explicit `internal_runtime` scope before all read/write service methods can enforce user workspace permissions directly.
- File download, metadata, original preview URL, text preview, and delete HTTP paths now share a file scoped authorization helper. Workspace files require `file.view` or `file.download` for previews and `file.manage` for deletion; organization-scoped files remain readable by org members for compatibility, while delete requires creator or organization admin/owner. The MinerU/document image preview route now rejects arbitrary storage keys and arbitrary local paths before storage or filesystem reads; only known parser image key prefixes and absolute MinerU image paths remain compatible. Newly generated parser image URLs are signed with the storage key or local path plus timestamp and nonce; tampered or partial signatures are denied before storage/filesystem reads while legacy unsigned URLs remain constrained by the same key/path allowlist.
- File folder create-under-parent, get, patch, delete, move-files, move-folder, archive/unarchive, favorite mutation/list visibility, file statistics, related-documents, related-datasets, related-resources, and folder permission-list HTTP paths now share file or folder scoped authorization before calling bare-ID service methods. File statistics resolve visible workspaces before service calls, return zero counts when the current account has no visible file workspace, and count only workspace plus folder-visible files. Folder patch now checks current-folder `file.manage` before JSON binding, keeping permission denial ahead of update request validation. File batch move now has regressions for target-folder `file.manage` denial and guessed-file `file.manage` denial before folder-join mutation. Folder management allows creator/admin or `file.manage` in the folder workspace; folder view honors creator/admin, shared folder permission, partial workspace grants, and `file.view|file.manage|file.download`.
- Dataset document, indexing status/progress, retry, batch status, segment, child chunk, segment-question, and async batch-hit-testing task HTTP paths now use dataset scoped authorization before calling bare-ID service methods. Document patch now checks route document `knowledge_base.manage` before JSON binding, retry checks route dataset `knowledge_base.manage` before binding `document_ids`, batch status checks route dataset `knowledge_base.manage` before validating action/document UUID query shape, and segment-question create/batch-create/generate/update checks route segment `knowledge_base.manage` before binding JSON bodies or validating generate `count`. This keeps permission denial ahead of update/retry/status/question-create/generate request validation. Dataset access validates organization, non-empty workspace scope, account, and `knowledge_base.*` permission through `AuthorizationService`; document/segment/child chunk helpers additionally validate path ownership before returning or mutating data. Segment-question read/update/delete also compare the bare `question_id` result to the route dataset/document/segment before returning data or mutating the row. Batch-hit-testing task status/report/stop/save now verify task dataset/account/organization ownership and re-check current dataset access before returning status, building reports, stopping tasks, or saving results; save denies foreign task access before request-body binding.
- Dataset folder create-under-parent, get, patch, delete, folder-list parent lookup, datasets-by-folder, and move-dataset paths now use dataset folder scoped authorization before calling bare-ID service methods. Folder reads require `knowledge_base.view|knowledge_base.manage|knowledge_base.folder_manage` plus folder visibility; folder writes require `knowledge_base.folder_manage`; moving a dataset into a folder additionally requires `knowledge_base.manage` on the dataset and same-workspace ownership between dataset and target folder.
- `web/src/routes/access.ts` is the first frontend access metadata entry point for console organization-scope versus workspace-scope routes.
- `GET/PUT /console/api/account/context` now exposes `mode` as the account-context contract boundary. The old field-only payload remains accepted for compatibility, while new callers can request `workspace`, `organization`, or `none` explicitly.
- `GET /console/api/account/capabilities` now exposes the current account context as a single route-guard contract. It allows organization members to use organization product surfaces without workspace permissions, returns empty workspace permissions in organization-only mode, and requires concrete workspace access plus `workspace.view` for workspace asset routes. Handler-level regressions now also lock the HTTP JSON shape for no-workspace organization mode, including `context`, `organization.product_surfaces`, `workspace`, `routes`, `runtime_audience`, and all four `runtime_surfaces`.
- `GET /console/api/account/capabilities` also exposes runtime grant metadata: webapp/API surfaces are public-compatible until their audience handshake is decided, builtin app grants support organization/account/department subjects, internal remains internal-only, and the current account runtime audience includes organization/account plus active department memberships when present. The backend contract is regression-covered for all four surfaces: `webapp` exposes `public`, `api` exposes `public`, `builtin_app` exposes `organization|department|account`, and `internal` exposes `internal`; when no organization context is available, the same metadata is returned disabled so frontend guards do not infer unsupported grant subjects.
- AIChat product conversations now have service-level regressions for the organization-mode contract: an active organization member with no current workspace can create a product chat conversation with `workspace_id=NULL`, while conversation history, conversation detail, message listing, and bare-message actions for delete/stop/regenerate remain scoped by `organization_id + account_id` and return no data for another account's guessed conversation or message ID. `DeleteMessage` now also performs an explicit scoped message preflight before opening the subtree-delete transaction.
- `AccountService.UpdateAccountContext` now synchronizes `workspace_members.current` after context updates: selecting a workspace sets the legacy current row, while organization/none mode clears stale current workspace flags.
- `GET /console/api/organizations/current/members` now lets organization admin/owner users search current organization members without requiring any managed workspace, while preserving the legacy managed-workspace permission path for workspace managers. This supports no-workspace organization admins managing account-level runtime grants.
- `GET /console/api/organizations/current/members/:member_id` is now registered on the organization handler and uses the existing organization-scoped member detail path instead of the legacy current-workspace member detail route. This gives runtime grant management a no-workspace-safe way to hydrate saved account grant IDs into member names and emails.
- Workspace self-routes under `/console/api/workspaces/:workspace_id/*` now have handler-level regressions proving update/statistics use the route workspace's real organization for `workspace.manage` or `workspace.view`, not an ambient account context organization. Statistics are denied before the service call when permission is missing, while organization admin/owner users can update organization workspaces without a `workspace_members` row.
- Legacy workspace member routes now centralize their permission checks through `MembersHandler.requireWorkspacePermission`, which resolves the route or current workspace's real organization before calling the legacy permission service. Regressions cover current-workspace member listing, route workspace member-extension listing, and cancel-invite denial before target member lookup.
- Organization workspace management routes now share a route workspace-to-organization guard before legacy permission checks. Assets, member removal, member role updates, and batch member add routes reject cross-organization workspace IDs before permission checks or target account/workspace service calls.
- `OrganizationService.CheckWorkspacePermission` and `CheckWorkspaceOrganizationAnyPermission` now fail closed when organization scope is empty or the organization cannot be found. Focused service regressions keep those paths returning `false` before workspace organization or membership lookups.
- `web/src/components/console/team-switcher.tsx` now provides an organization-mode entry. Entering organization mode calls `mode=organization`, clears workspace-scoped caches, refreshes profile context, and redirects workspace-required routes to `/console/work/app`. `pnpm test:route-access` now locks this switcher contract plus the `useEnterOrganizationMode` capability refresh/broadcast behavior so future route-guard changes cannot silently remove the first-class organization-mode path.
- `web/src/customer/default.tsx` and `web/src/app/console/work/layout.tsx` now consume account capabilities for both organization-scope and workspace-scope guard decisions instead of directly reading the workspace permission hook or only trusting local workspace-store state. The shell distinguishes missing-workspace state from a backend `workspace_scope_allowed=false` denial, so non-work workspace routes such as console dashboard pages stay aligned with the same `/account/capabilities` contract as `/console/work/*`. `pnpm test:route-access` also asserts the shell guard, work layout guard, and organization product page entry files for chat, image, app, app detail, app layout, and settings do not directly consume workspace permissions, render workspace-required states, or disable product data loading on empty workspace. The same regression now derives the actual `src/app/console/work` page tree, so future work-route additions must be explicitly classified as organization-scoped product UI or workspace-scoped helper/asset UI.
- `web/src/components/console/console-sidebar.tsx` now uses the shared route access metadata for desktop and mobile nav visibility. In organization mode, only organization-scoped entries such as chat, image, app, and settings remain visible; workspace-scoped asset and management entries stay behind workspace context and permissions.
- `web/src/app/console/page.tsx` now treats workspace resource widgets as workspace-scoped content. In organization mode it keeps the primary next action on `/console/work/chat`, skips the recent-work workspace query, hides workspace resource cards, and offers organization-scoped secondary actions instead of linking users into blocked asset pages.
- `GET /console/api/dashboard/recent-work` is now a workspace-scoped endpoint. It requires a current account-context workspace and `workspace.view` before any recent-work service call, and the service only returns recent agents, datasets, conversations, and data sources from that current workspace. This closes the direct-API leak that could otherwise expose organization-wide workspace asset names and IDs even when the frontend skipped the query.
- `web/src/hooks/organization/use-account-permissions.ts` now fails closed when no usable workspace context exists; it no longer derives workspace view permissions from organization role during organization-only mode. `web/src/store/workspace-store.ts` permission helpers now use the same workspace-only floor and return false unless the workspace context is ready, preventing future direct store consumers from reintroducing synthetic organization-mode workspace permissions.
- `runtimeauth.PublishedRuntimePolicy` defines the minimum published surface contract for `webapp`, `api`, `builtin_app`, and `internal` invocation. Existing `web_app_status` maps to webapp exposure, existing `enable_api` maps to external API exposure, and internal invocation remains independent from webapp/API exposure.
- `published_runtime_surfaces` and `published_runtime_surface_grants` provide the minimal persistence foundation for published runtime authorization. The migration seeds existing agents into `webapp`, `api`, `builtin_app`, and `internal` surfaces, preserving `web_app_status`, `enable_api`, and internal invocation compatibility. For app-center compatibility, an active legacy webapp with a seeded `builtin_app=false`, `compatibility_source=legacy_agent_fields`, and no grants is treated as visible until a builder saves an explicit `builtin_app` grant policy.
- `runtimeauth.Store.SaveResourceAuthorization` now repeats the surface/grant subject contract at the persistence boundary: webapp/API grants must stay `public`, built-in app grants must target organization/account/department subjects, and internal grants must stay `internal`. This keeps lower-level storage writes aligned with the current public-compatible webapp/API execution semantics until a product decision defines non-public webapp/API audience behavior.
- `GET /console/api/agents/:agent_id/runtime-surfaces` exposes the current published-surface authorization contract for an agent. It requires workspace `agent.view`, overlays persisted surface/grant rows when present, and falls back to legacy agent fields while older data is migrating.
- `PATCH /console/api/agents/:agent_id/runtime-surfaces` is the minimal management API for agent surfaces. It requires workspace `agent.manage`, writes `published_runtime_surfaces`/`published_runtime_surface_grants`, syncs `web_app_status` and `enable_api` for compatibility, and rejects disabling `internal` to preserve internal workflow/scheduled-task semantics. The backend contract now rejects non-public grants for webapp/API until the audience handshake decision is made, non-internal grants for `internal`, non-organization/account/department grants for `builtin_app`, organization grants for another organization, and account/department grant IDs that do not belong to the current organization.
- `POST /console/api/agents` now pre-reads only `workspace_id` from the body and, when present, checks the target workspace `agent.manage` before full DTO binding. This keeps missing-name and other create-agent request-shape feedback behind create permission while preserving the existing body contract.
- `AgentsService.RequireAgentManageAccess` is now the shared handler preflight for AGENT builder mutation and draft-run routes that need the route agent's workspace before parsing request bodies. Runtime-surface writes, runtime config updates, console draft chat, publish, webapp-status updates, suggested-question generation, memory slot/value mutations, published-version rollback, and legacy `PUT /agents/:agent_id` updates deny missing `agent.manage` before JSON binding, keeping permission failure ahead of request-shape feedback.
- Agent API key management routes now resolve the route agent's workspace in the current organization before API key repository or service work. List/get/usage/stats require `agent.view`; create/update/delete/revoke require `agent.manage`; create/update deny missing permission before JSON binding, and subresource routes deny missing permission before API-key-ID validation. This also fixes the legacy scope mismatch where the current organization ID from auth middleware was used as `agent_api_keys.tenant_id`; management queries now use the resolved route-agent workspace ID while external API key runtime validation keeps its API-key-owned runtime scope.
- Draft workflow management handlers now share `WorkflowHandler.requireAgentWorkspacePermission` instead of handwritten `enterpriseService` checks. Draft read requires route-agent `agent.view`; draft sync, draft run, draft precheck, publish, draft node run, advanced-chat draft run, manual diagnosis, and suggested-question generation require route-agent `agent.manage` before body binding or service work. Focused regressions keep malformed JSON from being parsed before permission denial and keep denied callers out of workflow service methods.
- `web/src/components/agents/api/runtime-access-tab.tsx` adds the first frontend management entry for agent published surfaces. Workflow and chatflow agents keep the existing API key/docs tabs plus a Publication Access tab; AGENT-mode apps can open the same page directly to manage runtime access. The UI saves public webapp/API grants, builtin organization/account/department grants through typed selectors, and keeps `internal` read-only/enabled.
- `GET/PATCH /console/api/built-in-workflows/:scenario/runtime-surfaces` is the minimal management API for built-in workflow catalog exposure. It requires organization admin/owner, returns only `builtin_app` and `internal` surfaces, writes `builtin_workflow` resource rows, supports organization/account/department grants for `builtin_app`, and rejects disabling `internal`. Built-in account/department grant writes validate the target active member or department belongs to the current organization before persisting.
- `web/src/components/dashboard/organization/built-in-workflow-runtime-section.tsx` adds the first frontend management entry for built-in workflow catalog exposure on the organization permissions page. It can enable/disable the `builtin_app` surface per built-in scenario and save multiple organization/account/department grants through the shared grant selector while showing `internal` invocation as read-only compatibility state.
- `web/src/components/runtime-auth/runtime-grant-subject-row.tsx` provides the shared runtime grant subject picker: organization-wide grants remain explicit, department grants use the existing department tree selector, and account grants use current-organization member search. Saved account grant IDs are hydrated through the organization-scoped current member detail route. Empty account/department rows, lookup failures, and stale account or department grants now show explicit inline states with the retained raw ID where applicable, so admins can fix the row instead of silently seeing an opaque fallback label.
- Published webapp config, legacy workflow webapp config/run/precheck/conversation routes, the legacy workflow webapp direct service path, and external API key authentication now resolve surface policy through persisted runtime authorization first and legacy fields as fallback. Existing valid keys keep working when the API surface remains enabled; persisted `api=false` denies external API even if old `enable_api` still reads true. Persisted `webapp=false` now denies both agent-runtime webapp routes and legacy workflow webapp runtime routes while preserving internal invocation. AGENT webapp upload-config and file-upload handlers return webapp-offline before authenticated-file-access, multipart parsing, or file validation can leak later-stage feedback.
- The legacy `POST /console/api/workflows/migrate-user` endpoint remains a compatibility migration path for anonymous webapp identity handoff. It has no `web_app_id` route parameter, so it cannot yet evaluate a resource-specific webapp surface policy. Handler regressions now lock the current contract: migration is denied before service work unless `WebAppAuthMiddleware` has set both virtual and authenticated account context, valid migration calls pass only those middleware-derived account IDs to the service, and same-account migration still maps to the existing response code.
- External API key published workflow and chat runtime handlers now use the API key runtime scope after middleware validates the `api` surface, instead of requiring the API key ID to be a workspace member with `agent.view`. Console-side published run/precheck handlers keep their route-agent workspace permission preflight, and mismatched API key agent context is denied before workspace permission checks.
- External API workflow/chat file inputs now require `upload_file_id` records to be owned by the current API key before extraction or execution. Temporary files uploaded through the external API remain usable by their creating key even though they use the temporary file tenant; guessed file IDs from another key are rejected. The specific-workflow route `/api/v1/workflows/:workflow_id/run` now shares this validator with latest published workflow and chat execution.
- External API workflow task stop now uses the API key's workspace/agent/key scope and the existing workflow run stop service instead of returning a fixed 501. This keeps external stop on the API service surface while preserving the service-level run workspace/agent ownership check before cancellation.
- External API workflow run detail and task stop now share an API-key-owned run preflight: the run must match the key's workspace, agent, and API key ID before detail lookup or cancellation. Console builder run detail/stop remains governed by workspace `agent.view/manage`, so builders can still manage agent runs without becoming the runtime caller.
- `runtimeauth.RuntimeAudience` and `SurfaceAuthorization.Allows` now provide the shared evaluator for public, organization, account, department, and internal runtime grants. A surface with no grants remains open when enabled for legacy compatibility; explicit grants narrow access.
- Agent list, detail, config, publish, webapp-status, update, and delete paths use `agent.view` or `agent.manage` workspace checks in the service layer. The MVP regression suite now locks the detail path against returning an agent when the caller lacks `agent.view`; create-agent also checks target workspace `agent.manage` before full DTO validation when `workspace_id` is present, and the management write preflight locks runtime-surface/config/publish/webapp-status/suggested-question/rollback/update body parsing behind `agent.manage`.
- Agent memory builder subresources under `/console/api/agents/:agent_id/memory/slots` and `/memory/values` use the same AGENT draft authorization chain as runtime config. Slot/value list and mutations require `agent.manage` plus editor capability before memory service calls, focused regressions now cover all five memory endpoints, and mutation handlers deny missing `agent.manage` before request body binding.
- Workflow variable CRUD methods remain service-level legacy placeholders with no public route registration. Environment and conversation variable data flows through draft workflow read/sync payloads, which stay guarded by the route agent's `agent.view` and `agent.manage` checks.
- Workflow run list, detail, node-execution detail, and node-log listing now require `agent.view` and validate the requested run belongs to the route `agent_id` before returning run data. The list path uses the same workspace permission checker abstraction as the detail paths and denies missing `agent.view` before query binding or service work. Node-log listing now checks route-agent workspace `agent.view` before looking up `run_id`, so missing-permission callers cannot use run IDs to distinguish 404 from 403. System workflow detail/node-execution access keeps compatibility with list semantics by allowing only the run creator account. Non-system console builder run detail intentionally keeps `agent_id` as the stable run boundary after route-agent workspace permission passes, because historical `workflow_run_logs.tenant_id` can be either caller workspace or app workspace; raw tenant equality needs a migration/backfill decision before it can be tightened. Draft workflow node single-run now requires `agent.manage` before request body binding or validation, and manual node diagnosis requires `agent.manage` plus run/node/agent ownership before request body binding or diagnosis invocation.
- Workflow run event streaming requires workspace `agent.view` or matching system-run creator before query validation and before opening the SSE response, so callers without run access cannot get `after`-parameter feedback or a long-lived stream.
- Non-runtime agent conversation history, conversation detail, chat messages, and legacy runtime-log listing now share the same builder `agent.view` gate before hitting bare `conversation_id` or historical log queries. Builder chat-message reads also re-check the requested conversation against the route agent before querying messages, so a guessed conversation ID from another agent cannot reach message lookup after `agent.view` passes. The builder `runtime-logs` dispatch path denies missing `agent.view` before JSON body binding, so malformed log-filter payloads cannot leak request-shape feedback ahead of workspace permission. `RuntimeLogHandler` also repeats list access when used directly: workspace workflow logs require `agent.view` before querying log rows, and system workflow log lists inject the current account as `created_by` into `WorkflowRunLogFilter` before repository lookup. AGENT runtime history remains on the runtime caller-scope path so product/runtime users are not forced through builder workspace membership; legacy runtime-log listing is also scoped to the current organization, account, and agent caller before returning runtime messages.
- Published workflow webapp conversation detail and delete routes now scope `conversation_id` by the active `web_app_id`'s agent before checking runtime caller ownership. This keeps same-agent cross-version compatibility while blocking same-account cross-agent/webapp ID reuse. Focused handler regressions now lock that detail denies foreign-account and other-agent conversations before message lookup, and delete denies a foreign-account conversation before the delete service call. Async webapp conversation title generation now also uses agent-scoped conversation lookup plus caller ownership before reading messages or calling the title model. Workflow parent-message and dialogue-count metadata helpers now repeat the same scoped conversation check before reading messages or dialogue counts.
- Published workflow webapp run/continue paths now validate a supplied `conversation_id` by agent and runtime caller before setting `sys.conversation_id`, parent message state, dialogue count, or updating the conversation `web_app_id`. The legacy `version_uuid` handler/service path shares the same guard for compatibility.
- Advanced-chat draft and published run paths now validate a supplied `conversation_id` by route agent and caller before setting `sys.conversation_id`, parent message state, or dialogue count. The draft HTTP run handler now requires route-agent `agent.manage` before JSON binding or request validation, keeping permission denial ahead of advanced-chat run-shape feedback. The service-level wrappers also reject foreign conversations before entering `RunDraftWorkflow` or `RunPublishedWorkflow`, which covers external API blocking calls.
- Generic draft and published conversation workflow run paths now validate raw `sys.conversation_id` inputs by route agent and caller before continuing an existing conversation in blocking/service execution. Streaming handlers validate raw `sys.conversation_id` or legacy top-level `conversation_id` only after the workflow type resolves to a conversation workflow, then promote the validated legacy value to `sys.conversation_id` at the handler boundary. Lower stream system-input preparation consumes only `sys.conversation_id`, preserving task workflow business-input compatibility.
- Streaming and blocking/simple workflow variable-pool construction now receive an explicit conversation access scope before loading persisted conversation variables. When a workflow declares conversation variables and `sys.conversation_id` is present, the loader validates the conversation against the current agent and caller before reading stored variable values; unauthorized conversations fail before variable reads, while missing conversation-variable configs and post-authorization load failures keep the previous compatible no-variable/default-value fallback.
- LLM workflow node legacy conversation-history loading now checks that `sys.conversation_id` belongs to the current route/runtime agent before querying `agent_messages`. This gives older node-level history settings a local agent-scope floor in addition to the run handler's caller-scope validation.
- AGENT runtime-runs builder log routes now require the route agent workspace's `agent.view` before query binding or chat-runtime queries. Detail and step routes then resolve `message_id` through caller-scoped runtime log lookups. Detail already denied cross-agent messages; the steps route now has the same focused regression so a guessed message cannot expose invocation/step payloads. Legacy AGENT `workflow-runs` list/detail/node-executions routes remain closed with 404 and no longer serve runtime chat messages, even when `run_id` is a valid runtime `message_id`.
- AGENT runtime conversation history routes keep their product/runtime path separate from builder `agent.view`, but list/detail/chat-message responses are scoped by organization, account, and agent caller before returning data. Console-agent runtime routes use `agentRuntimeContext`; published-webapp runtime routes use `webAppAgentRuntimeContext`; both carry caller-scoped conversation checks before conversation detail, update, delete, message list, stop, event stream, regeneration, and workflow continuation handling. Focused handler regressions now lock console-agent and published-webapp conversation update before request body binding, delete/message-list/stop before lower-level runtime service calls, event streaming before `message_id` query validation or SSE setup, plus message regeneration and workflow continuation before lower-level replacement or continuation work begins. `chatruntime.Service.StreamConversationEventsForCaller` and `BeginWorkflowApprovalContinuation` now reject mismatched callers before message lookup, while `PrepareConfiguredRootRegeneration` rejects mismatched callers before root replacement begins. Approval-token continuation resume now also requires the submitted approval form's `workflow_run_id` to match the caller-scoped continuation's run before `ActivePauseApprovalFormsSubmitted` or resume runner calls can execute. The legacy `StreamConversationEvents` entry remains AIChat-compatible.
- The follow-up inventory pass for older workflow history/log and non-conversation runtime subresource routes found no additional active route outside the guarded handlers above. `WorkflowStatisticHandler` remains an unregistered legacy handler, but its code path now resolves the route agent workspace and requires `agent.view` before `ShouldBindQuery` or statistic service calls. Future statistics route restoration must wire `WithWorkflowStatisticAuthorization` explicitly.
- Workflow-test HTTP routes now have handler-level regressions for `agent.view` read denial, `agent.manage` mutation denial, and empty agent workspace failure before case/batch/task subresource logic. Batch item reads plus active/latest/specific generation-task and scenario-recognition-task reads deny missing `agent.view` before stale-task recovery or task/batch-item lookup. Mutation, execution, and task-cancel paths check `agent.manage` before validating guessed case or batch UUIDs or canceling tasks, keeping permission denial ahead of subresource-shape feedback. Settings, scenario, case, generation-task, and batch mutation paths also deny missing `agent.manage` before JSON body binding, keeping malformed-body feedback behind workspace permission. Repository queries for case, batch, batch-item, generation-task, and scenario-recognition-task IDs stay scoped by `agent_id`; HTTP-triggered stale generation/scenario-recognition recovery now also updates only the route agent's task rows, while the local worker keeps its explicit global recovery path.
- Prompt optimize/playground routes now treat prompt builder tools as workspace asset helpers instead of organization-level product tools. `/prompts/optimize` and `/prompts/optimize/stream` require a current workspace with `agent.manage` before JSON binding; `/prompts/playground/stream` requires a current workspace with `agent.view|agent.manage` before JSON binding. `PromptService` repeats the current workspace visibility check before LLM/default-model setup and rejects non-official `prompt_id` values outside the current workspace, so direct service callers cannot use prompt IDs to cross workspace boundaries.
- Content-parse developer playground routes now align with the frontend workspace-scoped `/console/developer/content-parse` route. User-facing `/content-parse/playground/*` APIs require a current workspace with `workspace.view` before provider status, parse, save, run history, compare, source-preview, or PDF-render handler work. Internal `/console/api/internal/content-parse/playground/*` routes keep their separate HMAC boundary and bypass the user workspace guard.
- Runnable app listing now treats normal workspaces in the current organization as the default published app candidate set for any organization member, including members with no joined workspace. This removes builder workspace membership as a prerequisite for `/console/work/app` while keeping repository filtering on active webapp status and published versions. The list now also evaluates agent `builtin_app` runtime grants: active legacy webapps remain visible by default, while explicit organization/account/department grants narrow app-center visibility. The app center page, app sidebar layout, and app detail page now call the runnable app query without a current-workspace gate, so the organization-level route guard is not followed by an empty workspace-bound UI state. App detail direct URLs now wait for this runnable authorization before fetching public webapp config, keeping `builtin_app` app-center exposure separate from the public-compatible `webapp` surface.
- Built-in workflow catalog routes now require organization authentication and no longer require workspace membership, which gives `/console/work/image` the same organization-level product-use footing as chat and app-center entry points. The catalog uses the runtime grant evaluator for `builtin_workflow` resources: no persisted rows preserve the current organization-member default, while persisted `builtin_app` grants can narrow entries by organization, account, or department.
- Existing business handlers still need to be migrated onto `AuthorizationService` or scoped asset authorizers in later tasks. The current high-risk workflow/conversation history, prompt builder, content-parse developer helper, agent API key management, agent workflow binding candidate listing, built-in workflow direct-detail grant enforcement paths, and explicit runtime-grant incomplete/error/stale display are covered by focused regressions; remaining follow-up work is broader handler unification, bulk runtime grant UX, and the webapp/API audience decision recorded in the endpoint inventory.

Previous narrow validation, 2026-06-21 07:52 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestCreateVariablePoolWithVarsForRun" -count=1
go test ./internal/modules/app/workflow -run "Test(CreateVariablePoolWithVars|ExecuteSimpleWorkflow|BuildWorkflowStreamVariablePool|ValidateWebAppConversationAccess|RunDraftWorkflow_RejectsForeignSystemConversationBeforeRunning|RunPublishedWorkflow_RejectsForeignSystemConversationBeforeRunning|RunAdvancedChatDraftWorkflow_RejectsForeignConversationBeforeRunning|RunAdvancedChatWorkflow_RejectsForeignConversationBeforeRunning)" -count=1
go test ./internal/modules/app/workflow -run "Test(AutomationWorkflow|CreateVariablePoolWithVarsForRun|CreateVariablePoolWithVars_PreservesOrganizationID|ValidateWebAppConversationAccess|RunDraftWorkflow_RejectsForeignSystemConversationBeforeRunning|RunPublishedWorkflow_RejectsForeignSystemConversationBeforeRunning|RunWorkflowByWebAppID|RunPublishedWorkflow|WorkflowServicePrecheckWorkflowRun)" -count=1
go test ./internal/modules/app/workflow -run "Test(BuildWorkflowStreamVariablePool|AgentHistoryDispatch|AgentRuntime|GetWorkflowRun|GetWorkflowRunEvents|GetWorkflowRunNodeLogs)" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_(Conversation|BuiltInWorkflowRuntimeSurfaces)" -count=1
go test ./internal/modules/app/workflow -count=1
go test ./routes/v1 -run "^$" -count=1
git diff --check -- api/internal/modules/app/workflow/workflow_executor.go api/internal/modules/app/workflow/workflow_executor_test.go docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 20
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 20
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- Blocking/simple `WorkflowExecutor` variable-pool construction now validates `sys.conversation_id` against the current agent/caller before loading persisted conversation variables. Unauthorized conversations fail before variable loading; authorized conversations load persisted values; post-authorization variable-load errors keep the legacy default-value fallback.
- Adjacent automation workflow, conversation-access, stream variable-pool, history/runtime, run-event, node-log, published workflow, webapp workflow, and built-in workflow runtime-surface route subsets pass; v1 route registration still compiles.
- The full `internal/modules/app/workflow` package test passes. `git diff --check` reported no whitespace errors. The Docker zgi-c API image rebuilt successfully, API/web/Postgres containers were healthy, `/ping` returned `{"message":"pong"}`, web `/api/health` returned `{"status":"ok"}`, the latest migration row remained `20260620090000_create_published_runtime_authorization`, and `published_runtime_surfaces` still contained 13 rows for each compatibility surface: `api`, `builtin_app`, `internal`, and `webapp`.

Latest narrow validation, 2026-06-21 07:45 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestBuildWorkflowStreamVariablePool" -count=1
go test ./internal/modules/app/workflow -run "Test(BuildWorkflowStreamVariablePool|BuildWorkflowStreamGraph|ValidateWebAppConversationAccess|RunAdvancedChatDraftWorkflow_RejectsForeignConversationBeforeRunning|RunAdvancedChatWorkflow_RejectsForeignConversationBeforeRunning|RunDraftWorkflow_RejectsForeignSystemConversationBeforeRunning|RunPublishedWorkflow_RejectsForeignSystemConversationBeforeRunning|AgentHistoryDispatch|AgentRuntime|GetWorkflowRun|GetWorkflowRunEvents|GetWorkflowRunNodeLogs)" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_Conversation" -count=1
go test ./routes/v1 -run "^$" -count=1
git diff --check -- api/internal/modules/app/workflow/workflow_stream_runtime.go api/internal/modules/app/workflow/workflow_stream_execute.go api/internal/modules/app/workflow/workflow_stream_runtime_reachability_test.go docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 20
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 20
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- Stream variable-pool construction now validates `sys.conversation_id` against the current agent/caller before reading persisted conversation variables. Unauthorized conversations fail before variable loading; authorized conversations load persisted values; workflows with no conversation-variable config skip the extra access check; post-authorization variable-load errors keep the legacy default-value fallback.
- Adjacent workflow conversation-access, history/runtime, run-event, node-log, and published webapp conversation route subsets pass; v1 route registration still compiles.
- `git diff --check` reported no whitespace errors. The Docker zgi-c API image rebuilt successfully, API/web/Postgres containers were healthy, `/ping` returned `{"message":"pong"}`, web `/api/health` returned `{"status":"ok"}`, the latest migration row remained `20260620090000_create_published_runtime_authorization`, and `published_runtime_surfaces` still contained 13 rows for each compatibility surface: `api`, `builtin_app`, `internal`, and `webapp`.

Latest narrow validation, 2026-06-21 07:35 +08:00:

```powershell
go test ./internal/modules/app/agents -run "TestAgentsHandlerWorkflowBindingCandidatesRequireManageBeforeServiceCall" -count=1
go test ./internal/modules/app/agents -run "TestAgentsHandler(WorkflowBindingCandidates|MutationsRequireManageBeforeBindingRequest|ChatAgentRequiresManageBeforeBindingRequest|_UpdateWebAppStatus|_GetAgentRuntimeSurfaces|_UpdateAgentRuntimeSurfaces)|TestAgentsService_UpdateWebAppStatus|TestAgentsService_GetAgentRuntimeSurfaces|TestAgentsService_UpdateAgentRuntimeSurfaces" -count=1
go test ./routes/v1 -run "^$" -count=1
git diff --check -- api/internal/modules/app/agents/agents_handler.go api/internal/modules/app/agents/agents_webapp_handler_test.go docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 20
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 20
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- `/console/api/agents/:agent_id/workflow-bindings/candidates` now uses the route agent management preflight before calling the service. A caller without `agent.manage` is denied before the service can load the draft agent/config or list same-workspace workflow candidates.
- The adjacent agents handler/runtime-surface and webapp-status subsets pass, and v1 route registration still compiles.
- `git diff --check` reported no whitespace errors. The Docker zgi-c API image rebuilt successfully, API/web/Postgres containers were healthy, `/ping` returned `{"message":"pong"}`, web `/api/health` returned `{"status":"ok"}`, the latest migration row was `20260620090000_create_published_runtime_authorization`, and `published_runtime_surfaces` contained all four compatibility surfaces with 13 rows each: `api`, `builtin_app`, `internal`, and `webapp`.

Latest narrow validation, 2026-06-21 07:29 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestBuiltInWorkflowScenarioDetailHonorsBuiltinAppAccountGrant" -count=1
go test ./internal/modules/app/workflow -run "Test(BuiltInWorkflow|AgentHistoryDispatch|RunPublishedWorkflow|RunWorkflowByWebAppID|WebAppPrecheck|PrecheckWorkflow)" -count=1
git diff --check -- api/internal/modules/app/workflow/built_in_service_runtime_auth_test.go docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 20
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- Built-in workflow `builtin_app` grants are now locked as a list-and-detail contract: an account grant for another account hides the workflow from `GetAllBuiltInWorkflows` and also denies direct `GetBuiltInWorkflowByScenario`; a department grant containing the caller allows both the catalog entry and direct scenario detail lookup.
- The adjacent workflow history dispatch and webapp/published workflow subsets still pass, covering the route families near built-in runtime publication and workflow history access.
- `git diff --check` passes for this slice.
- This slice adds regression coverage only; no production code or frontend code changed, so the API image was not rebuilt after the previous 07:24 rebuild.
- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` remain healthy. API health returns `{"message":"pong"}`.
- Published runtime authorization storage remains stable: latest migration rows include `20260620090000_create_published_runtime_authorization`, and seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

Latest narrow validation, 2026-06-21 07:24 +08:00:

```powershell
go test ./internal/modules/api_key -run "TestAPIKeyManagement" -count=1
go test ./internal/modules/api_key -count=1
go test ./routes/v1 -run "^$" -count=1
go test ./middleware -run "TestValidateAPIKey|TestAPIKeyAuthMiddlewareDoesNotLogSensitiveAPIKey|TestUpdateLastUsed" -count=1
git diff --check -- api/internal/modules/api_key/api_key_access.go api/internal/modules/api_key/api_key_handler.go api/internal/modules/api_key/api_key_handler_permission_test.go api/routes/v1/api_key_routes.go api/routes/v1/routes.go docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 20
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 20
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
cd ../web
pnpm test:route-access
pnpm type-check
```

Observed result:

- Agent API key management now resolves the route agent's workspace before management repository/service work. Focused regressions cover `agent.view` for list/get/usage/stats, `agent.manage` for create/update/delete/revoke, denial before malformed body or API-key-ID validation, and allowed list calls using the resolved workspace ID instead of the current organization ID.
- `api_key`, `routes/v1`, and API key middleware targeted tests pass; external API key runtime surface semantics remain covered by middleware tests and were not changed by the management-route hardening.
- `git diff --check` passes for this slice with only CRLF working-copy warnings.
- `pnpm test:route-access` and `pnpm type-check` pass.
- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy after rebuilding the API image from the current worktree.
- API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.
- Published runtime authorization storage remains stable: latest migration rows include `20260620090000_create_published_runtime_authorization`, and seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

Latest narrow validation, 2026-06-21 07:08 +08:00:

```powershell
go test ./internal/modules/contentparse/handler -run "TestPlaygroundRoutesRequire|TestInternalPlaygroundRoutesBypass" -count=1
go test ./internal/modules/contentparse/handler -count=1
go test ./internal/modules/contentparse/... -count=1
go test ./routes/v1 -run "^$" -count=1
pnpm test:route-access
git diff --check -- api/internal/modules/contentparse/handler/playground_handler.go api/internal/modules/contentparse/handler/playground_types.go api/internal/modules/contentparse/handler/playground_permission_test.go api/internal/modules/contentparse/handler/playground_parse_session_test.go api/internal/modules/contentparse/module.go api/routes/v1/contentparse_routes.go api/routes/v1/routes.go web/scripts/test-route-access.mjs
```

Observed result:

- Content-parse playground permission regressions pass and prove user-facing playground routes require current workspace `workspace.view` before request-shape handling, while internal HMAC playground routes bypass that user workspace guard.
- `go test ./internal/modules/contentparse/... -count=1` and `go test ./routes/v1 -run "^$" -count=1` pass after injecting `OrganizationService` into the content-parse module/route deps.
- `pnpm test:route-access` passes and now locks `/console/developer/content-parse` as workspace-scoped. The existing playground LRU cache test had same-tick timestamp flakiness; its test setup now sleeps between stores/accesses to make the intended LRU order deterministic.

Latest narrow validation, 2026-06-21 06:59 +08:00:

```powershell
go test ./internal/modules/prompts/handler -run "TestPromptBuilderToolsRequire" -count=1
go test ./internal/modules/prompts/service -run "TestPrompt(Optimizer|Playground).*Workspace|TestPromptOptimizerRejectsPromptFromAnotherWorkspace" -count=1
go test ./internal/modules/prompts/... -count=1
git diff --check -- api/internal/modules/prompts/handler/prompt_handler.go api/internal/modules/prompts/module.go api/internal/modules/prompts/service/prompt_optimizer.go api/internal/modules/prompts/service/prompt_playground.go api/internal/modules/prompts/service/prompt_service.go api/internal/modules/prompts/handler/prompt_handler_permission_test.go api/internal/modules/prompts/service/prompt_workspace_access_test.go
```

Observed result:

- Prompt handler regressions pass and prove optimize, optimize-stream, and playground-stream deny missing current-workspace permission before malformed JSON is bound or prompt service methods are called.
- Prompt service regressions pass and prove empty workspace, missing workspace visibility, and cross-workspace non-official `prompt_id` inputs fail before LLM/default-model setup. Official prompts remain compatible under the current workspace permission boundary.
- `go test ./internal/modules/prompts/... -count=1` passes for the whole prompts module. `git diff --check` passes for prompt files; Git reports only local CRLF normalization warnings.

Latest Docker validation, 2026-06-21 07:10 +08:00:

```powershell
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 20
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 20
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy after rebuilding the API image from the current worktree, including the prompt builder and content-parse developer playground permission guards.
- API `/ping` returns 200 with `{"message":"pong"}` and web `/api/health` returns 200 with `{"status":"ok"}`.
- The latest migration list includes `20260620090000_create_published_runtime_authorization`; seeded published runtime surfaces currently count 13 each for `api`, `builtin_app`, `internal`, and `webapp`.

Latest narrow validation, 2026-06-21 06:44 +08:00:

```powershell
go test ./internal/util -run "TestSignedParserImage" -count=1
go test ./internal/modules/file_process/handler -run "TestGetMinerUImage|TestAllowMinerULocalImagePath" -count=1
go test ./internal/modules/file_process/service/extractor -run "Test(ImageDataURLFromStorageKey|ImageFileDataURLRejectsPreviewURL|Persist|Markdown|MinerU|Signed)" -count=1
go test ./internal/modules/file_process/service/extractor/hyperparse -run "Test.*MinerU|TestMapResultToExtractOutput" -count=1
go test ./internal/modules/dataset/service -run "TestNormalizeKnowledgeImageURLs|TestNormalizeHitTestingResponseKnowledgeImageURLs" -count=1
git diff --check -- api/internal/util/file_helper.go api/internal/util/file_helper_test.go api/internal/modules/file_process/handler/image_preview_handler.go api/internal/modules/file_process/handler/image_preview_handler_test.go api/internal/modules/file_process/service/extractor/markdown_image_assets.go api/internal/modules/file_process/service/extractor/hyperparse/mineru_assets.go api/internal/modules/file_process/service/extractor/extract_processor_images.go api/internal/modules/dataset/service/knowledge_image_url_normalizer.go api/internal/modules/dataset/service/knowledge_image_url_normalizer_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 20
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 20
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- Parser image URL generation now signs new storage-key and local-path URLs with `timestamp`, `nonce`, and `sign` bound to the exact `key` or `path`; generated signatures verify, tampered values fail, and unsigned legacy fallback remains available only when signing config is missing.
- Markdown asset persistence, HyperParse MinerU assets, extract-processor local image URLs, and knowledge-image normalization now consume the same signed parser image helper, so generated parser markdown uses one contract.
- `/console/api/files/mineru-images` still supports legacy unsigned rendered parser images under the key/path allowlist, but tampered or partial signed requests are denied before storage `Load` or local `os.ReadFile`.
- `git diff --check` reports no whitespace errors; Git only reports existing CRLF conversion warnings on touched files.
- The zgi-c API image was rebuilt from the current worktree; `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy. API `/ping` returns `{"message":"pong"}`, web `/api/health` returns `{"status":"ok", ...}`, latest migration remains `20260620090000_create_published_runtime_authorization`, and runtime surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.
- Broader package runs are still limited in this local shell by existing environment issues: `file_process/service/extractor` has a Windows `.xls` TempDir cleanup lock in `TestExcelExtractorHandleXlsReadsLegacyWorkbook`, and `dataset/service` vector tests require CGO-enabled sqlite.

Latest narrow validation, 2026-06-21 06:33 +08:00:

```powershell
go test ./internal/modules/file_process/handler -run "Test(GetMinerUImage|AllowMinerULocalImagePath)" -count=1
go test ./internal/modules/file_process/handler -run "Test(AuthorizeFile|PatchFolder|CanListFavoriteFile|GetMinerUImage|AllowMinerULocalImagePath)" -count=1
go test ./internal/modules/file_process/handler -count=1
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 20
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 20
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- `/console/api/files/mineru-images` no longer accepts arbitrary storage keys or local filesystem paths. Invalid keys are denied before storage `Load`, and non-MinerU local paths are denied before `os.ReadFile`.
- Existing file access helper regressions still pass, covering file preview/download/manage/folder visibility behavior adjacent to this route.
- The zgi-c API image was rebuilt from the current worktree; `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy. API `/ping` returns `{"message":"pong"}`, web `/api/health` returns `{"status":"ok", ...}`, latest migration remains `20260620090000_create_published_runtime_authorization`, and runtime surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

## Acceptance Evidence

Backend evidence:

- Organization member with no workspace can keep `current_workspace_id = null`.
- Organization member with no workspace can use organization-level product endpoints.
- Organization member with no workspace cannot access workspace assets by guessing IDs.
- Account context/profile helpers preserve organization mode without backfilling a workspace: `EnsureCurrentOrganizationID` returns the organization ID while leaving `current_workspace_id=null`, `GetAccountProfile` exposes the same organization/no-workspace context, and `GET /account/context` serializes `mode=organization`.
- Organization admin/owner users can search current organization members for runtime account grants without needing a managed workspace.
- Current-organization member detail has a `/organizations/current/members/:member_id` route backed by organization scope, so runtime grant UI hydration does not depend on a current workspace join.
- Workspace asset APIs deny cross-organization and cross-workspace IDs.
- Workspace permission filtering returns only workspaces with the requested permission.
- Workspace update/statistics routes resolve the route workspace's organization before checking `workspace.manage`/`workspace.view`; statistics fail before service calls without permission, and organization admins can update without a workspace membership row.
- Workspace member list/extension/cancel-invite routes resolve workspace organization before permission checks, and cancel-invite denies missing `workspace.manage` before looking up the target member account.
- Organization workspace assets, member removal, member role update, and batch-add routes reject route workspaces from another organization before permission checks, account lookup, workspace lookup, or asset counting.
- Workspace permission service checks fail closed for empty or missing organization scope, instead of allowing legacy callers through with an absent organization.
- Dashboard recent-work denies organization-mode/no-current-workspace callers before permission or asset service calls, denies callers without `workspace.view`, and passes only the current workspace to the recent-work service after the permission check succeeds.
- Agent detail access denies callers without `agent.view` for the agent workspace.
- Organization admin can manage organization workspaces without requiring a `workspace_members` row.
- Webapp/API surface compatibility tests preserve existing active behavior and verify persisted surface rows override legacy fields.
- Published runtime surface migration and store tests preserve legacy webapp/API/internal semantics and model builtin user/department grants for the next UI/API slice.
- Agent runtime-surface read API denies callers without `agent.view` before returning published authorization data; write API requires `agent.manage` and refuses to disable `internal`.
- Agent and built-in runtime-surface write APIs reject cross-organization account/department grant targets before persisting rows.
- Agent memory slot/value builder APIs deny callers without `agent.manage` or editor capability before reading or mutating memory configuration and organizer values.
- Workflow export denies callers without `agent.view` on the route agent workspace before generating YAML. Workflow import rejects missing account or organization context before reading form data, then checks `agent.manage` on the target workspace, including a form-supplied `workspace_id` that differs from the current workspace context, before editor checks, uploaded-file parsing, or agent creation.
- External API file-variable and chat-file inputs deny `upload_file_id` values created by another API key before content extraction or workflow execution, while allowing the current key's own temporary upload files.
- External API workflow task stop passes the API key's workspace, agent, and key ID into the workflow run stop service; inaccessible or mismatched `task_id` values return not-found/not-accessible before runtime cancellation.
- External API workflow run detail and task stop deny same-agent run/task IDs created by another API key before returning detail or invoking cancellation; service regressions lock the underlying workspace/agent/API-key owner check.
- Organization member with no joined workspace can list runnable apps published in normal organization workspaces.
- Built-in workflow catalog denies unauthenticated callers, accepts organization members without a workspace, preserves default visibility when no runtime rows exist, hides entries granted only to another account, and allows entries granted to the caller's department. Built-in workflow runtime-surface management is organization admin/owner only; admin read returns builtin/internal surfaces, admin patch persists account/department/organization grants, and non-admin patch is denied before storage writes.
- Workflow task stop, draft node single-run, workflow run list/detail, node-executions, node-log listing, run-event SSE streams, and manual diagnosis paths deny mismatched agent/run/node scopes and enforce `agent.view`/`agent.manage` before binding query/body inputs, looking up guessed run IDs, cancelling, returning, streaming, or diagnosing runtime details. System workflow run detail/node-executions/events deny runs created by another account.
- Agent conversation history, conversation detail, chat messages, and legacy runtime-log listing deny non-runtime builder access without `agent.view`; builder runtime-log dispatch also denies malformed request bodies after the permission check, not before it. AGENT runtime history continues to allow caller-scoped product/runtime history without builder workspace permission, while filtering or denying conversations and runtime-log messages from another agent or account.
- Published workflow webapp conversation detail/delete deny same-account conversation IDs that belong to another agent/webapp before reading messages or soft-deleting rows.
- Published workflow webapp run/continue paths deny supplied conversation IDs that belong to another agent/webapp or another caller before setting runtime system conversation variables, updating conversation webapp metadata, or executing the workflow.
- Advanced-chat draft/published run paths deny supplied conversation IDs that belong to another agent or caller before setting runtime system conversation variables or executing the workflow.
- Generic draft/published conversation workflow run paths deny raw `sys.conversation_id` values that belong to another agent or caller before reusing the conversation in service execution.
- LLM workflow node legacy history loading denies a `sys.conversation_id` from another agent before querying message history.
- AGENT runtime-runs list/detail/steps deny callers without workspace `agent.view` before query binding or chat-runtime queries; detail and steps also deny cross-agent `message_id` values before building runtime detail or step responses. The old AGENT `workflow-runs` list/detail/node-executions endpoints return 404 instead of falling back to runtime message detail or node-step payloads.
- AGENT runtime conversation update denies cross-caller conversation IDs before update-body binding, so invalid JSON cannot expose request-shape feedback before caller-scope denial. AGENT runtime delete, message-list, and stop deny cross-caller conversation IDs before lower-level runtime service calls. AGENT runtime event streaming denies cross-caller conversation IDs before `message_id` query validation or SSE setup, and webapp event streaming, message regeneration, and workflow continuation deny cross-caller conversation/message IDs before entering `RunPreparedStream` or continuation stream handling. Event streaming now uses `StreamConversationEventsForCaller`, and the `chatruntime` service rejects mismatched callers before event message lookup/replay, continuation message lookup, or root-message replacement.
- Workflow-test handler permissions deny guessed batch/case/task paths before subresource service calls when the caller lacks `agent.view` or `agent.manage`; empty agent workspace scope fails before permission service calls.
- Manual workflow node diagnosis denies callers without `agent.manage` before request body binding, and denies mismatched `run_id`/`node_log_id` scope before model request validation or diagnosis execution.
- Workflow run event SSE denies callers without workspace `agent.view` before `after` query validation or event-stream headers are opened.
- zgi-c API-level backtest uses an isolated local account with active organization membership and zero `workspace_members` rows. In organization mode, `GET /account/context` returns `mode=organization`, `GET /account/capabilities` allows organization scope and denies workspace scope, `GET /agents/runnable-webapps` and `GET /built-in-workflows` return product data, and workspace asset list endpoints return no assets. After temporarily adding a workspace membership and switching through `PUT /account/context` with `mode=workspace`, capabilities return `workspace_scope_allowed=true` and workspace asset lists return data again; the fixture is reset to zero workspace memberships after the check.

Frontend evidence:

- Console shell renders product routes without current workspace, consumes `/account/capabilities` before rendering organization or workspace route children, and leaves workspace-required versus access-denied decisions to the shared backend capability contract.
- Workspace asset routes show a workspace-required state without leaking data.
- Console route access metadata now has a single organization-scoped route source for the shell and work layout; route-access regression locks chat, image, app, app detail, settings, console shell capability consumption, and workspace asset boundaries against future metadata drift.
- Workspace permission helpers fail closed without a ready workspace context; route-access regression guards both the hook contract and the lower-level workspace store helpers against synthetic organization-mode permissions.
- `useJoinedWorkspaces` no longer writes the first workspace when profile context is intentionally empty.
- Workspace switcher has a first-class organization-mode item and workspace selection sends `mode=workspace`.
- Navigation uses the same access metadata as page guards; desktop and mobile sidebars hide workspace-scoped asset entries in organization mode while keeping organization product routes visible.
- Console home consumes account capabilities before showing workspace-scoped recent work or resource cards, so organization-only members land on organization product actions instead of asset links that immediately block.
- Runnable app list is based on published/runtime access, not builder workspace membership; app center page/layout/detail queries are no longer disabled when `currentWorkspace` is empty.
- Organization permissions includes a minimal built-in workflow runtime surface editor backed by the admin `runtime-surfaces` API.
- Agent detail navigation includes a Publication Access entry for AGENT-mode apps and a Publication Access tab alongside API keys/docs for workflow/chatflow apps; both are backed by `/console/api/agents/:agent_id/runtime-surfaces`.
- Agent and built-in runtime grant rows reuse current-organization member search for account grants and the organization department tree for department grants. Saved account grant IDs are hydrated into member labels through `/organizations/current/members/:member_id`, avoiding raw account/department ID entry in the main management paths.

Latest frontend validation, 2026-06-21:

```powershell
pnpm type-check
pnpm test:route-access
pnpm exec eslint src/customer/default.tsx scripts/test-route-access.mjs
git diff --check -- web/src/customer/default.tsx web/src/app/console/page.tsx web/src/components/console/console-sidebar.tsx web/src/routes/access.ts web/src/store/workspace-store.ts web/scripts/test-route-access.mjs docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
```

Observed result:

- `pnpm type-check`, `pnpm test:route-access`, and targeted eslint for `web/src/customer/default.tsx` plus `web/scripts/test-route-access.mjs` pass.
- Full `pnpm lint` is still blocked by existing repository-wide lint debt outside this slice, including `next-env.d.ts`, auth invite/test pages, content-parse, knowledge-graph, workflow node files, generated sensitive-word output, and other pre-existing `any`/unused/type-import/quote issues.

Latest backend validation, 2026-06-21:

```powershell
go test ./internal/modules/user/auth/service -run "Test(EnsureCurrentOrganizationIDPreservesOrganizationModeWithoutWorkspace|GetAccountProfileReturnsOrganizationContextWithoutWorkspace|GetAccountContext|GetAccountCapabilities|UpdateAccountContext|EnsureAccountContextForWorkspace)" -count=1
go test ./internal/modules/user/auth/handler -run "TestAccountHandler(GetAccountContext|UpdateAccountContext|GetAccountCapabilities)" -count=1
go test ./internal/modules/system/handler -run "TestDashboardRecentWork" -count=1
go test ./internal/modules/system/service -run "Test" -count=1
go test ./routes/v1 -run "^$" -count=1
go test ./internal/modules/workspace/service -run "Test(OrganizationServiceCheckWorkspace(Permission|OrganizationAnyPermission)FailsClosedForMissingOrganizationScope|WorkspacePermissionFilter)" -count=1
go test ./internal/modules/workspace/handler -run "Test(OrganizationWorkspaceRoutesRejectCrossOrganizationWorkspaceBeforePermission|MembersHandler(CurrentMembersUsesCurrentWorkspaceOrganizationForPermission|WorkspaceMembersExtensionUsesRouteWorkspaceOrganizationForPermission|CancelWorkspaceInviteRequiresManageBeforeMemberLookup)|WorkspaceStatisticsUsesRouteWorkspaceOrganizationForPermission|UpdateWorkspaceAllowsOrganizationAdminWithoutWorkspaceMembership|GetCurrentOrganizationMembersUsesKeyword|GetCurrentOrganizationMembersAllowsOrganizationAdminWithoutManagedWorkspace|GetCurrentOrganizationMemberDetailUsesOrganizationScope|OrganizationRoutesRegisterCurrentMember|OrganizationRoutesRegisterCurrentMembersList|OrganizationRoutesRegisterCurrentMemberDetail)" -count=1
go test ./internal/modules/app/runtimeauth -count=1
go test ./internal/migrations -run "Test(RegisteredMigrationsAreValid|MigrationFilenameMatchesRegisteredID|PublishedRuntimeAuthorization|CheckStaticRules)" -count=1
go test ./middleware -run "TestValidateAPIKey|TestAPIKeyAuthMiddlewareDoesNotLogSensitiveAPIKey|TestUpdateLastUsed" -count=1
go test ./internal/modules/app/agents -run "Test(AgentRuntimeAuthorizationFromUpdateRequest_RejectsInvalidSurfaceGrantSubjects|AgentsService_UpdateAgentRuntimeSurfaces_(RejectsInternalDisable|RequiresBuiltinGrantWhenEnabled|RejectsAccountGrantOutsideOrganization|RejectsDepartmentGrantOutsideOrganization))" -count=1
go test ./internal/modules/app/agents -run "TestAgentsHandlerMutationsRequireManageBeforeBindingRequest" -count=1
go test ./internal/modules/app/agents -run "TestAgentsHandler(ChatAgentRequiresManageBeforeBindingRequest|MutationsRequireManageBeforeBindingRequest|CreateRequiresManageBeforeBindingBusinessFields|_UpdateAgentRuntimeSurfaces_PassesContextAndRequest|_GetAgentRuntimeSurfaces_PassesContext)|TestPublicAgentWebAppConfig|TestRequireAuthenticated" -count=1
go test ./internal/modules/app/agents -run "TestAgentsHandlerCreateRequiresManageBeforeBindingBusinessFields" -count=1
go test ./internal/modules/app/agents -run "TestAgentsService_GetRunnableWebApps_(OrganizationAdminUsesAllNormalOrganizationWorkspaces|OrganizationMemberWithoutWorkspaceUsesNormalOrganizationWorkspaces|AllowsOrganizationWorkspaceWithoutAgentView|OrganizationMemberCanRequestWorkspaceWithoutJoiningIt|ReturnsEmptyWhenWorkspaceOutsideOrganization|ReturnsErrorWhenCurrentOrganizationMissing)" -count=1
go test ./internal/modules/app/agents -run "Test(AgentsService_GetAgent_RejectsMissingWorkspaceViewPermission|AgentsService_GetAgentRuntimeSurfaces_UsesWorkspaceViewAndLegacyFallback|AgentsService_GetAgentRuntimeSurfaces_RejectsMissingWorkspaceViewPermission|AgentsService_UpdateAgentRuntimeSurfaces_RejectsInternalDisable|AgentsService_UpdateAgentRuntimeSurfaces_RequiresBuiltinGrantWhenEnabled|AgentsService_UpdateAgentRuntimeSurfaces_RejectsAccountGrantOutsideOrganization|AgentsService_UpdateAgentRuntimeSurfaces_RejectsDepartmentGrantOutsideOrganization|AgentRuntimeAuthorizationFromUpdateRequest_RejectsInvalidSurfaceGrantSubjects|AgentsService_GetPublishedAgentWebAppConfig_RejectsUnpublishedActiveWebApp|AgentsService_GetPublishedAgentWebAppConfig_RejectsPersistedDisabledWebApp|PublicAgentWebAppConfig_DoesNotExposeRuntimeSecrets|RequireAuthenticatedWebAppAgent)" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_BuiltInWorkflowRuntimeSurfaces(UpdateAccountGrantForAdmin|RejectsAccountGrantOutsideOrganization|RejectsDepartmentGrantOutsideOrganization|ReturnFallbackForAdmin|RejectsNonAdminUpdate)" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_BuiltInWorkflow(RuntimeSurfaces|s)" -count=1
go test ./internal/modules/app/workflowtest -run "Test(Handler|RepositoryDeleteCasesScopesByAgentAndIDs|UpdateCase|.*GenerationTask|.*Batch)" -count=1
go test ./internal/modules/app/workflowtest -run "TestHandler(MutationRoutesRequireAgentManageBeforeBindingRequest|MutationRoutesRequireAgentManageBeforeSubresourceIDValidation|ListBatchItemsRequiresAgentViewPermission|UpdateCaseRequiresAgentManagePermission|WorkflowTestAccessRejectsAgentWithoutWorkspace)" -count=1
go test ./internal/modules/datasource/handler -run "Test(ImportTableRecordsRequiresDatabaseEditBeforeBindingRequest|UpsertTablePromptRequiresDatabaseManageBeforeBindingRequest|DeleteTableRecordsRequiresDatabaseEditBeforeIDValidation|UpdateTableRecordsRequiresDatabaseEditBeforeBindingRequest|AddTableRecordsRequiresDatabaseEditBeforeBindingRequest|UpdateTableColumnsRequiresDatabaseManageBeforeBindingRequest|UpdateTableRequiresDatabaseManageBeforeBindingRequest|CreateTableRequiresDatabaseManageBeforeBindingRequest|UpdateDataSourceRequiresDatabaseManageBeforeBindingRequest|ExcelImportMutationsRequireDatabaseManageBeforeBindingRequest)" -count=1
go test ./internal/modules/datasource/service -run "Test" -count=1
go test ./internal/modules/file_process/handler -run "Test(PatchFolderRequiresManageBeforeBindingRequest|AuthorizeFile|CanListFavoriteFile)" -count=1
go test ./internal/modules/file_process/handler -run "Test" -count=1
go test ./internal/modules/dataset/handler -run "Test(UpdateDocumentStatusRequiresManageBeforeDocumentIDValidation|RetryDocumentRequiresManageBeforeBindingRequest|UpdateDocumentRequiresManageBeforeBindingRequest|AuthorizeDataset|FailDatasetFolder)" -count=1
go test ./internal/modules/dataset/handler -run "Test" -count=1
go test ./internal/modules/app/agents -run "Test(WebAppAgentRuntime(UpdateConversationRequiresCallerScopedConversationBeforeBindingRequest|EventsRequireCallerScopedConversation|EventsRequireCallerScopedConversationBeforeMessageIDValidation|EventsPassCallerToStreamService|ContinuationRequiresCallerScopedMessage|RegenerationRequiresCallerScopedMessage)|AgentRuntime(UpdateConversationRequiresCallerScopedConversationBeforeBindingRequest|EventsRequireCallerScopedConversationBeforeMessageIDValidation|EventsPassCallerToStreamService|ContinuationRequiresCallerScopedMessage|RegenerationRequiresCallerScopedMessage))" -count=1
go test ./internal/modules/app/workflow -run "TestAgentHistoryDispatch(RuntimeLogsAreCallerScoped|RequiresAgentViewForBuilderHistory|RuntimeAgentBypassesBuilderHistoryPermission|RuntimeConversationsAreCallerScoped|RuntimeConversationDetailRejectsOtherAgentConversation|RuntimeChatMessagesRejectsOtherAccountConversation)|TestAgentRuntime(LogRoutesRequireAgentViewPermission|RunsReturnsOnlyNewWebAppMessages|RunsIncludesWebAppMessagesFromEndUserAccounts|RunsCanFilterByConversation|RunsCanFilterConsoleAndKeyword|RunDetailRejectsOtherAgentMessage|RunStepsRejectsOtherAgentMessage|RunDetailAllowsWebAppMessageFromEndUserAccount|StepsFromSkillInvocationsAndFinalAnswer)" -count=1
go test ./internal/modules/app/workflow -run "Test(GetWorkflowRunEvents|ManualDiagnoseNode|ValidateWorkflowRunNodeScope|RunDraftWorkflowNodeRequiresAgentManageBeforeBindingRequest|RunAdvancedChatDraftWorkflowRequiresAgentManageBeforeBindingRequest|StopWorkflowTask)" -count=1
go test ./internal/modules/app/workflow -run "Test(DraftWorkflowManagementHandlersRequireWorkspacePermissionBeforeWork|GenerateDraftWorkflowSuggestedQuestionsRequiresAgentManageBeforeBindingRequest|RunDraftWorkflowNodeRequiresAgentManageBeforeBindingRequest|RunAdvancedChatDraftWorkflowRequiresAgentManageBeforeBindingRequest|ManualDiagnoseNode|GetWorkflowRunEvents|ImportWorkflow|ExportWorkflow)" -count=1
go test ./internal/modules/app/workflow -count=1
```

Closure audit, 2026-06-21:

| Requirement slice | Current evidence | Status |
| --- | --- | --- |
| Account context mode API and no-workspace state | Service/handler tests, zgi-c API login/context/capabilities backtest, and headless Chrome product/asset route checks | Covered for the MVP boundary |
| Frontend route guard and workspace switcher | `pnpm test:route-access`, `pnpm type-check`, headless Chrome switcher popover, desktop product/asset routes, mobile app/asset smoke | Covered for functional behavior |
| Organization product use without workspace | zgi-c API and browser checks for chat, image, app, settings, runnable apps, and built-in workflows | Covered for current product surfaces |
| Workspace asset isolation | Authorization service adoption plus targeted datasource, file, dataset, agent, workflow-test, workflow history/runtime tests | Covered for the high-risk bare-ID paths listed in the MVP matrix |
| Published runtime authorization storage and compatibility | Migration tests, `runtimeauth` tests, Docker migration/status checks, agents runtime-surface tests, built-in workflow runtime-surface route tests, API key middleware tests | Covered for webapp/API/builtin/internal MVP semantics |
| User/department grant foundation | Runtime surface storage, account/department validation tests, current-organization member search/detail routes, frontend grant selector paths | Foundation covered; incomplete rows, lookup failures, and stale saved grants are explicit in the UI; richer bulk grant UX remains follow-up work |

Latest zgi-c no-workspace API validation, 2026-06-21:

- Local isolated account: active organization member, `current_workspace_id=NULL`, zero `workspace_members`.
- Standard `POST /console/api/login` succeeds for the local fixture account and keeps `current_workspace_id=NULL` with zero `workspace_members`, so login no longer repairs the user back into a workspace.
- Organization mode:
  - `GET /console/api/account/context`: 200, `mode=organization`, `current_workspace_id=null`.
  - `GET /console/api/account/capabilities`: 200, `organization_scope_allowed=true`, `workspace_scope_allowed=false`, `workspace_required=true`, workspace permissions empty.
  - `GET /console/api/agents/runnable-webapps`: 200, returned 6 runnable apps.
  - `GET /console/api/built-in-workflows`: 200, returned 3 built-in workflows.
  - `GET /console/api/agents`, `/datasets`, `/files`: 200 with `total=0` for this no-workspace account.
- Temporary workspace mode:
  - After adding one workspace membership and calling `PUT /console/api/account/context` with `mode=workspace`, `GET /console/api/account/capabilities` returned `workspace_scope_allowed=true`, `workspace_required=false`, `workspace.can_view=true`, and 22 workspace permissions.
  - Workspace asset lists then returned data again (`agents=9`, `files=21` in the local fixture).
  - The temporary membership was removed and account context reset to organization mode after validation.

Latest zgi-c no-workspace browser validation, 2026-06-21:

- Used local headless Chrome against `http://localhost:2880` with the same isolated no-workspace account. The in-app Browser plugin could not attach in this desktop session because its runtime metadata was missing, so Chrome DevTools Protocol was used as the real-browser fallback.
- Organization mode capability snapshot during browser validation: `mode=organization`, `organization_scope_allowed=true`, `workspace_scope_allowed=false`, `workspace_required=true`, and `current_workspace_id=null`.
- Product routes rendered real page content without login/init redirects:
  - `/console/work/chat`: rendered the console chat workbench and model input affordances.
  - `/console/work/image`: rendered the AI image generation workbench and built-in workflow data request.
  - `/console/work/app`: rendered app center content with 6 runnable apps.
  - `/console/settings`: rendered system/account settings content.
- Workspace switcher UI in organization mode:
  - The trigger rendered `组织模式` with the `切换工作空间` aria label.
  - Opening the menu set `aria-expanded=true` and showed `切换工作空间`, `组织模式`, and the member no-workspace message (`您还没有被分配到任何工作空间。`).
- Workspace asset routes remained blocked by the no-workspace state, without redirects or data leakage:
  - `/console/agents`, `/console/dataset`, and `/console/files` all rendered the no-workspace state (`暂无可用工作空间`) for the fixture account.
- Mobile viewport smoke (`390x844`) kept the same boundaries:
  - `/console/work/app` rendered the app center with 6 apps and no login/init redirect.
  - `/console/agents` rendered the no-workspace state and no asset list.
- Temporary workspace-mode browser check:
  - Added one local workspace membership, switched with `PUT /console/api/account/context mode=workspace`, and verified `workspace_scope_allowed=true`, `workspace_required=false`, `GET /agents total=9`, and `GET /files total=21`.
  - `/console/agents` then rendered the `测试空间` asset list with agent rows instead of the no-workspace block.
  - Cleanup removed the temporary membership and reset `account_contexts.current_workspace_id=NULL`; final DB check returned zero workspace memberships.

Latest narrow validation, 2026-06-20:

```powershell
go test ./internal/modules/user/auth/handler -run TestAccountHandler.*AccountContext -count=1
go test ./internal/modules/user/auth/service -run "Test(UpdateAccountContext|GetAccountContext|EnsureAccountContext)" -count=1
go test ./internal/modules/user/auth/handler -run "TestAccountHandler.*(AccountContext|Capabilities)" -count=1
go test ./internal/modules/user/auth/service -run "Test(GetAccountCapabilities|UpdateAccountContext|GetAccountContext|EnsureAccountContext)" -count=1
go test ./internal/modules/user/auth/service -run "TestGetAccountCapabilities(OrganizationModeAllowsProductSurfacesOnly|RuntimeAudienceIncludesActiveDepartments|WorkspaceModeUsesWorkspaceViewPermission|WorkspaceModeDeniesWithoutWorkspaceViewPermission)" -count=1
go test ./internal/modules/app/runtimeauth -count=1
go test ./internal/modules/app/agents -run "TestAgents(Service_GetAgentRuntimeSurfaces|Service_UpdateAgentRuntimeSurfaces|Handler_GetAgentRuntimeSurfaces|Handler_UpdateAgentRuntimeSurfaces|Service_GetAgent_|Handler_UpdateWebAppStatus|Service_UpdateWebAppStatus|Service_GetPublishedAgentWebAppConfig|Service_GetAgentConfig)|TestPublicAgentWebAppConfig|TestRequireAuthenticated" -count=1
go test ./internal/modules/app/agents -run "TestAgentsService_(AgentMemoryEndpointsRequireManagePermission|AgentMemoryEndpointsRequireEditor|GetAgentConfig|GetAgent_|GetAgentRuntimeSurfaces)" -count=1
go test ./internal/modules/app/agents -run "TestWebAppAgentRuntime(EventsRequireCallerScopedConversation|ContinuationRequiresCallerScopedMessage|RegenerationRequiresCallerScopedMessage)" -count=1
go test ./internal/capabilities/chatruntime/service -run "Test(StreamConversationEventsForCallerRejectsOtherCallerBeforeMessageLookup|BeginWorkflowApprovalContinuationRejectsOtherCallerBeforeMessageLookup|PrepareConfiguredRootRegenerationRejectsOtherCallerBeforeReplacement)" -count=1
go test ./internal/modules/app/agents -run "TestWebAppAgentRuntime(EventsRequireCallerScopedConversation|EventsPassCallerToStreamService|ContinuationRequiresCallerScopedMessage|RegenerationRequiresCallerScopedMessage)" -count=1
go test ./internal/modules/app/workflow -run "Test(RunDraftWorkflow_RejectsForeignSystemConversation|RunPublishedWorkflow_RejectsForeignSystemConversation|RunAdvancedChat.*RejectsForeignConversation|ValidateWebAppConversationAccess)" -count=1
go test ./internal/modules/app/workflow/nodes/llm -run "Test(LoadConversationHistoryPromptMessagesRejectsOtherAgentBeforeMessageQuery|FetchMemory_LegacyFallbackDisabledDoesNotLoadHistory|FetchMemory_LegacyFallbackZeroWindowDoesNotLoadHistory|TokenBufferMemory_DoesNotImplicitlyLoadConversationHistory|PromptMessagesFromAgentMessagesExpandsRounds)" -count=1
go test ./internal/modules/app/workflow -run "TestAgentHistoryDispatchRuntime(AgentBypassesBuilderHistoryPermission|ConversationsAreCallerScoped|ConversationDetailRejectsOtherAgentConversation|ChatMessagesRejectsOtherAccountConversation)" -count=1
go test ./internal/modules/app/workflow -run "TestAgentRuntime(WorkflowRunsNoLongerServeAgentMessages|WorkflowRunDetailNoLongerServesAgentMessages|WorkflowRunNodeExecutionsNoLongerServeAgentMessages|RunStepsRejectsOtherAgentMessage|RunDetailRejectsOtherAgentMessage)" -count=1
go test ./internal/modules/app/workflow -run "TestAgentRuntimeRun(StepsRejectsOtherAgentMessage|DetailRejectsOtherAgentMessage|DetailAllowsWebAppMessageFromEndUserAccount|StepsFromSkillInvocationsAndFinalAnswer)" -count=1
go test ./internal/modules/app/workflow -run "TestGetWorkflowRunEvents" -count=1
go test ./internal/modules/app/workflow -run "TestStopWorkflowTask" -count=1
go test ./internal/modules/app/workflow -run "Test(ExportWorkflowRequiresAgentViewPermission|ImportWorkflow(UsesFormWorkspaceForAgentManagePermission|RejectsUnauthorizedBeforeReadingForm|RejectsMissingOrganizationBeforeReadingForm))" -count=1
go test ./internal/modules/app/workflow -run "Test(StopWorkflowTask|RunDraftWorkflowNodeRequiresAgentManageBeforeBindingRequest)" -count=1
go test ./internal/modules/app/workflow -run "Test(StopWorkflowTask|GetWorkflowRunEvents|GetWorkflowRunDetail|GetWorkflowRunNodeExecutions|ValidateWorkflowRunAccess|ValidateWorkflowRunNodeScope|GetWorkflowRunNodeLogs|UpdateWorkflowNodeRuntimeLog|RunDraftWorkflowNode)" -count=1
go test ./internal/modules/app/workflow -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_(ConversationDetail|ConversationDelete|WebAppConfig|Run|Precheck|BuiltInWorkflows)" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_BuiltInWorkflow(RuntimeSurfaces|s)" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_" -count=1
go test ./internal/modules/app/workflowtest -run "Test(Handler|RepositoryDeleteCasesScopesByAgentAndIDs|UpdateCase|.*GenerationTask|.*Batch)" -count=1
go test ./middleware -run "TestValidateAPIKey|TestAPIKeyAuthMiddlewareDoesNotLogSensitiveAPIKey|TestUpdateLastUsed" -count=1
go test ./internal/migrations -run "Test(RegisteredMigrationsAreValid|MigrationFilenameMatchesRegisteredID|PublishedRuntimeAuthorization|CheckStaticRules)" -count=1
pnpm type-check
pnpm test:route-access
cd docker
docker compose -f docker-compose.yaml -f compose.zgi-c.local.yaml --env-file .env up -d --build api web
docker exec zgi-c-api-1 ./server migrate:status
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('published_runtime_surfaces','published_runtime_surface_grants') ORDER BY table_name;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Latest narrow validation, 2026-06-21:

```powershell
go test ./routes/external -run "TestExternal(GetWorkflowRunDetail|StopWorkflowTask|Validate)" -count=1
go test ./routes/external -count=1
go test ./tests/routes -run "TestExternalWorkflowStopTask" -count=1
go test ./middleware -run "TestValidateAPIKey|TestAPIKeyAuthMiddlewareDoesNotLogSensitiveAPIKey|TestUpdateLastUsed" -count=1
go test ./internal/modules/app/workflow -run "Test(ValidateExternalWorkflowRunAccess|StopWorkflowTask|ValidateWorkflowRunAccess)" -count=1
go test ./internal/modules/app/workflow -run "Test(StopWorkflowTask|DraftWorkflowManagementHandlersRequireWorkspacePermissionBeforeWork|PublishedRuntimeHandlersUseAPIKeyScopeInsteadOfWorkspaceMembership|PublishedRuntimeHandlersRejectAPIKeyAgentMismatchBeforeWorkspacePermission)" -count=1
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- External API file input validation now treats the API key as the runtime file owner. Current-key temporary uploads are accepted, foreign API key uploads are denied, and tenant mismatches are still rejected for non-temporary files.
- Workflow, chat workflow, chat `files`, and specific-workflow external API paths all use the owner-aware validator before file extraction or execution.
- External API workflow task stop now passes API key workspace/agent/key scope into the workflow run stop service and returns not-found/not-accessible for service-level run scope denial.
- External API workflow run detail and task stop now deny same-agent runs created by another API key before detail lookup or cancellation. The shared service preflight requires matching workspace, agent, and API key owner.
- The focused external route package tests and adjacent API-key/workflow published-runtime regressions pass.
- The local zgi-c API container was rebuilt from the current worktree; `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy.
- API health remains `{"message":"pong"}` on `http://127.0.0.1:2870/ping`.
- Published runtime authorization storage remains present in zgi-c Postgres: latest migration rows include `20260620090000_create_published_runtime_authorization`, and seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

Latest narrow validation, 2026-06-21:

```powershell
go test ./internal/modules/app/workflow -run "Test(DraftWorkflowManagementHandlersRequireWorkspacePermissionBeforeWork|PublishedRuntimeHandlersUseAPIKeyScopeInsteadOfWorkspaceMembership|PublishedRuntimeHandlersRejectAPIKeyAgentMismatchBeforeWorkspacePermission)" -count=1
go test ./middleware -run "TestValidateAPIKey|TestAPIKeyAuthMiddlewareDoesNotLogSensitiveAPIKey|TestUpdateLastUsed" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_" -count=1
go test ./internal/modules/app/runtimeauth -count=1
pnpm test:route-access
pnpm type-check
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- External API key published workflow and chat runtime handlers now use API-key runtime scope after middleware validates the `api` surface, so valid API keys are not required to be workspace members with `agent.view`.
- Console published workflow and advanced-chat run handlers still require route-agent workspace `agent.view`; the existing permission-order regression remains green.
- Mismatched API key agent context is denied before workspace permission checks.
- Middleware API-surface tests, workflow route regressions, runtimeauth tests, frontend route access, and frontend type-check pass.
- The local zgi-c API container was rebuilt from the current worktree; `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy.
- API health remains `{"message":"pong"}` on `http://127.0.0.1:2870/ping`.
- Published runtime authorization storage remains present in zgi-c Postgres: latest migration rows include `20260620090000_create_published_runtime_authorization`, and seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

Latest narrow validation, 2026-06-21 05:43 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "Test(RejectInactiveWebApp|ResolveWebAppRunScope|WebAppPrecheck|ValidateWebAppConversationAccess|RunWorkflowByWebAppID|RunWorkflowByVersionUUID)" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_(WebAppConfig|Run|Precheck|ConversationList)" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_" -count=1
go test ./internal/modules/app/runtimeauth -count=1
pnpm test:route-access
pnpm type-check
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- Legacy workflow webapp routes and the direct workflow webapp service path now resolve the published `webapp` runtime surface through `published_runtime_surfaces` first, falling back to `agents.web_app_status` only when no persisted rows exist.
- Route regressions cover persisted `webapp=false` denying public config, run, precheck, and conversation-list paths even when the legacy `web_app_status` field is active.
- The full `TestWorkflowRoutes_` subset passes, covering the adjacent conversation detail/delete, built-in workflow catalog/runtime-surface, and compatibility routes.
- Runtimeauth tests, frontend route access, and frontend type-check pass.
- The local zgi-c API container was rebuilt from the current worktree; `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy.
- API health remains `{"message":"pong"}` on `http://127.0.0.1:2870/ping`.
- Published runtime authorization storage remains present in zgi-c Postgres: latest migration rows include `20260620090000_create_published_runtime_authorization`, and seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

Latest narrow validation, 2026-06-21:

```powershell
pnpm test:route-access
pnpm type-check
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build web
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- App-center detail direct URLs now gate public webapp config loading behind the runnable-app authorization check. This preserves public webapp config compatibility while preventing an app hidden by `builtin_app` grants from being prefetched through the organization product UI.
- `useWebAppConfig` now accepts an `enabled` option so callers can apply their own authorization prerequisite without changing public webapp service behavior.
- Frontend route access and type-check pass.
- The local zgi-c web container was rebuilt from the current worktree. Compose also recreated the API container through the dependency graph; `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy.
- API health remains `{"message":"pong"}` on `http://127.0.0.1:2870/ping`.
- Published runtime authorization storage remains present in zgi-c Postgres: latest migration rows include `20260620090000_create_published_runtime_authorization`, and seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

Latest narrow validation, 2026-06-21 05:31 +08:00:

```powershell
go test ./internal/modules/app/runtimeauth -count=1
go test ./internal/modules/app/agents -run "TestAgentsService_GetRunnableWebApps|TestAgentsService_GetAgentRuntimeSurfaces|TestAgentsService_UpdateAgentRuntimeSurfaces" -count=1
pnpm test:route-access
pnpm type-check
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, enabled, compatibility_source, COUNT(*) FROM published_runtime_surfaces WHERE resource_type = 'agent' GROUP BY surface, enabled, compatibility_source ORDER BY surface, enabled, compatibility_source;"
```

Observed result:

- Agent app-center listing now evaluates `builtin_app` runtime authorization after selecting active published webapps from normal organization workspaces. No-workspace organization members keep legacy app-center visibility because active legacy webapps with seeded `builtin_app=false`, `compatibility_source=legacy_agent_fields`, and no grants are interpreted as not explicitly disabled.
- Explicit agent `builtin_app` account grants hide apps from other accounts, and explicit department grants allow callers in the granted department.
- `runtimeauth` now treats active legacy webapps as app-center compatible by default while preserving explicit `grant` source rows as authoritative.
- Frontend route access and type-check pass.
- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy after rebuilding the API image from the current worktree.
- API health: external `http://127.0.0.1:2870/ping` returns 200 with `{"message":"pong"}`.
- Published runtime authorization storage remains present in zgi-c Postgres: latest migration rows include `20260620090000_create_published_runtime_authorization`, seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`, and current agent rows remain legacy-compatible (`builtin_app=false`, `compatibility_source=legacy_agent_fields`) without destructive backfill.

Latest narrow validation, 2026-06-21 05:28 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestDraftWorkflowManagementHandlersRequireWorkspacePermissionBeforeWork" -count=1
go test ./internal/modules/app/workflow -run "Test(DraftWorkflowManagementHandlers|RunAdvancedChatWorkflow|RunPublishedWorkflow|PrecheckWorkflow|WebAppPrecheck|RunWorkflowByWebAppID|RunWorkflowByVersionUUID)" -count=1
go test ./internal/modules/app/workflow -run "Test(DraftWorkflowManagementHandlers|GetWorkflowRunNodeLogs|GetWorkflowRun|AgentHistoryDispatch|AgentRuntime)" -count=1
pnpm test:route-access
pnpm type-check
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- Console agent-id published workflow run/precheck routes now require the route agent's workspace `agent.view` before request-body binding or published workflow lookup. This closes the legacy bare-agent-ID invocation path without changing public webapp, API key, or internal invocation routes.
- Focused workflow permission-order tests and adjacent workflow/webapp conversation tests pass.
- Frontend route access and type-check pass.
- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy after rebuilding the API image from the current worktree.
- API health: external `http://127.0.0.1:2870/ping` returns 200 with `{"message":"pong"}`.
- Published runtime authorization storage remains present in zgi-c Postgres: latest migration rows include `20260620090000_create_published_runtime_authorization`, and seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

Latest narrow validation, 2026-06-21 05:14 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestGetWorkflowRunNodeLogs|TestValidateWorkflowRunAccess" -count=1
go test ./internal/modules/app/workflow -run "TestGetWorkflowRunNodeLogs|TestGetWorkflowRun|TestAgentHistoryDispatch|TestAgentRuntime" -count=1
pnpm test:route-access
pnpm type-check
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:2870/ping
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- Workflow node-log listing now checks route-agent workspace `agent.view` before looking up `run_id`; focused regressions cover no-permission no-run-lookup and still reject runs from another route agent after permission passes.
- Related workflow run, agent-history dispatch, and AGENT runtime-run subsets pass.
- Frontend route access and type-check pass.
- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy after rebuilding the API image from the current worktree.
- API health: external `http://127.0.0.1:2870/ping` returns 200 with `{"message":"pong"}`.
- Published runtime authorization storage remains present in zgi-c Postgres: latest migration rows include `20260620090000_create_published_runtime_authorization`, and seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

Latest narrow validation, 2026-06-21 05:08 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestAgentRuntime(RunsRequiresAgentViewBeforeBindingQuery|LogRoutesRequireAgentViewPermission|RunStepsRejectsOtherAgentMessage|RunDetailRejectsOtherAgentMessage|RunsReturnsOnlyNewWebAppMessages|RunsIncludesWebAppMessagesFromEndUserAccounts|RunsCanFilterByConversation)" -count=1
go test ./internal/modules/app/workflow -run "TestAgentRuntime" -count=1
go test ./internal/modules/app/workflow -run "TestGetWorkflowRun|TestAgentHistoryDispatch" -count=1
pnpm test:route-access
pnpm type-check
docker ps --filter "name=zgi-c-(api|web|postgres)" --format "table {{.Names}}\t{{.Status}}"
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:2870/ping
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- AGENT runtime-runs list now has a focused regression for denying missing `agent.view` before query binding, and the wider `TestAgentRuntime` workflow subset passes.
- Workflow-run query and agent-history dispatch subsets pass.
- Frontend route access and type-check pass.
- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy in the current local Docker environment. This slice changed tests and docs only, so the API image was not rebuilt after the previous 05:03 rebuild.
- API health: external `http://127.0.0.1:2870/ping` returns 200 with `{"message":"pong"}`.
- Published runtime authorization storage remains present in zgi-c Postgres: latest migration rows include `20260620090000_create_published_runtime_authorization`, and seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

Latest narrow validation, 2026-06-21 05:03 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestGetWorkflowRun(sRequiresAgentViewBeforeBindingQuery|DetailRequiresAgentViewPermission|DetailRejectsSystemRunFromAnotherAccount|NodeExecutionsRejectsRunFromAnotherAgent)" -count=1
go test ./internal/modules/app/workflow -run "TestGetWorkflowRun" -count=1
go test ./internal/modules/app/workflow -run "TestAgentHistoryDispatch" -count=1
pnpm test:route-access
pnpm type-check
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
docker ps --filter "name=zgi-c-(api|web|postgres)" --format "table {{.Names}}\t{{.Status}}"
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:2870/ping
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- Workflow-run list now uses the same workspace permission checker abstraction as detail/node-executions and has a focused regression for denying missing `agent.view` before query binding or `GetWorkflowRuns` service work.
- The related workflow-run query and agent-history dispatch subsets pass.
- Frontend route access and type-check pass.
- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy in the current local Docker environment after rebuilding the API image from the current worktree with `docker/.env` and `compose.zgi-c.local.yaml`.
- API health: external `http://127.0.0.1:2870/ping` returns 200 with `{"message":"pong"}`.
- Published runtime authorization storage remains present in zgi-c Postgres: latest migration rows include `20260620090000_create_published_runtime_authorization`, and seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

Latest narrow validation, 2026-06-21 04:59 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestAgentHistoryDispatch(RuntimeLogsRequiresAgentViewBeforeBindingRequest|RequiresAgentViewForBuilderHistory|RuntimeLogsAreCallerScoped|RuntimeChatMessagesRejectsOtherAccountConversation|RuntimeConversationDetailRejectsOtherAgentConversation|RuntimeConversationsAreCallerScoped)" -count=1
go test ./internal/modules/app/workflow -run "TestAgentHistoryDispatch" -count=1
pnpm test:route-access
pnpm type-check
docker ps --filter "name=zgi-c-(api|web|postgres)" --format "table {{.Names}}\t{{.Status}}"
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:2870/ping
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- Builder agent-history dispatch now has a focused regression for `POST /agents/:agent_id/runtime-logs` denying missing `agent.view` before JSON body binding, and the full `TestAgentHistoryDispatch` subset passes.
- Frontend route access and type-check pass.
- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy in the current local Docker environment. This slice changed tests and docs only, so the API image was not rebuilt.
- API health: external `http://127.0.0.1:2870/ping` returns 200 with `{"message":"pong"}`.
- Published runtime authorization storage remains present in zgi-c Postgres: latest migration rows include `20260620090000_create_published_runtime_authorization`, and seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`.

Latest narrow validation, 2026-06-21 04:56 +08:00:

```powershell
go test ./internal/modules/app/agents -run "Test(WebAppAgentRuntime(DeleteConversation|ListMessages|StopConversation)|AgentRuntime(DeleteConversation|ListMessages|StopConversation))" -count=1
go test ./internal/modules/app/agents -run "Test(WebAppAgentRuntime|AgentRuntime)" -count=1
pnpm test:route-access
pnpm type-check
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:2870/ping
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND (table_name ILIKE '%migration%' OR table_name IN ('published_runtime_surfaces','published_runtime_surface_grants')) ORDER BY table_name;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 5;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT COUNT(*) AS grant_count FROM published_runtime_surface_grants;"
```

Observed result:

- The new console-agent and published-webapp AGENT runtime delete/message-list/stop caller-scope regressions pass, and the wider `Test(WebAppAgentRuntime|AgentRuntime)` subset passes.
- Frontend route access and type-check pass.
- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy in the current local Docker environment. This slice changed tests and docs only, so the API image was not rebuilt.
- API health: external `http://127.0.0.1:2870/ping` returns 200 with `{"message":"pong"}`.
- Published runtime authorization storage is present in zgi-c Postgres: `migrations` includes `20260620090000_create_published_runtime_authorization`, `published_runtime_surfaces` and `published_runtime_surface_grants` exist, seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`, and grant count is 39.
- Compose status via repository-root `docker compose --env-file .env ...` could not run because there is no root `.env`; the running zgi-c container health was verified directly with `docker ps` and the API/database checks above.

Latest Docker validation, 2026-06-21 04:45 +08:00:

```powershell
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
```

Observed result:

- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy after rebuilding the API image from the current worktree, including the dashboard recent-work, draft workflow management unified permission preflight, console AGENT draft chat permission-order, draft node-run permission-order, advanced-chat draft run permission-order, workflow-test subresource permission-order, datasource update permission-order, datasource table create/update/columns-update permission-order, datasource table prompt upsert permission-order, datasource record add/update/delete/import permission-order, datasource Excel import permission-order, file folder patch permission-order, dataset document patch permission-order, dataset retry permission-order, dataset document status permission-order, agent management mutation permission-order, legacy agent update permission-order, create-agent permission-order, AGENT runtime conversation update caller-scope-before-body-binding, AGENT runtime event stream caller-scope-before-message-id-validation, and AGENT runtime-runs workspace `agent.view` changes.
- The rebuilt API image also includes the workflow import account/organization preflight change, so missing account or organization context is rejected before form body parsing.
- The web container remains healthy from the earlier rebuilt frontend bundle, including the workspace-store fail-closed permission helper change.
- The published runtime authorization migration/storage evidence was verified earlier in this zgi-c DB: `20260620090000_create_published_runtime_authorization` is applied, `published_runtime_surfaces` and `published_runtime_surface_grants` exist, and seeded surface counts are `api=13`, `builtin_app=13`, `internal=13`, and `webapp=13`.
- API health: external `http://127.0.0.1:2870/ping` returns 200 with `{"message":"pong"}`.

Latest Docker validation for console-shell capability guard, 2026-06-21:

```powershell
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build web
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT COUNT(*) AS grant_count FROM published_runtime_surface_grants;"
```

Observed result:

- The web image rebuild completes with a production Next build, including `web/src/customer/default.tsx` console-shell capability guard changes. Compose also refreshes the API image from cache as a dependency of the web stack.
- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy after the rebuild.
- API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.
- Published runtime authorization storage remains stable: latest migration is still `20260620090000_create_published_runtime_authorization`, seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`, and grant count remains 39.

Note: `go test ./internal/modules/app/agents -count=1` is currently blocked in this local shell by an existing sqlite-dependent test when CGO is disabled (`go-sqlite3 requires cgo to work`). The targeted agents runtime/webapp subset above passes and covers the changed files.

Previous narrow validation, 2026-06-21 08:02 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestGetRuntimeLogs|TestGetWorkflowRunNodeLogs" -count=1
go test ./internal/modules/app/workflow -run "Test(AgentHistoryDispatch|GetWorkflowRun|GetWorkflowRuns|GetRuntimeLogs|GetWorkflowRunNodeLogs|ValidateWorkflowRunAccess)" -count=1
go test ./internal/modules/app/workflow -count=1
go test ./routes/v1 -run "^$" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_(Conversation|BuiltInWorkflowRuntimeSurfaces)" -count=1
git diff --check -- api/internal/modules/app/workflow/runtime_log_handler.go api/internal/modules/app/workflow/runtime_log_handler_test.go api/internal/modules/app/workflow/workflow_repository.go api/internal/modules/app/workflow/workflow_service_runtime_log_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT COUNT(*) AS grant_count FROM published_runtime_surface_grants;"
```

Observed result:

- Runtime-log handler regressions pass: missing `agent.view` is denied before log-row queries, node-log run scope remains enforced, and system workflow runtime-log lists pass the current account as `created_by`.
- `go test ./internal/modules/app/workflow -count=1`, route compile, and focused workflow route tests pass.
- `git diff --check` reports no whitespace errors for the touched backend/docs files; only the existing Windows CRLF warnings appear for Go files.
- zgi-c API image rebuilds successfully, and `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy.
- API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.
- Published runtime authorization storage remains stable: latest migration is still `20260620090000_create_published_runtime_authorization`, seeded surface counts remain `api=13`, `builtin_app=13`, `internal=13`, `webapp=13`, and grant count remains 39.

Latest narrow validation, 2026-06-21 08:07 +08:00:

```powershell
pnpm test:route-access
pnpm type-check
git diff --check -- web/scripts/test-route-access.mjs docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 15
```

Observed result:

- `pnpm test:route-access` passes and now also locks the workspace switcher organization-mode entry, redirect from workspace-required routes to `/console/work/app`, `mode=organization` account-context call, capability refresh, and cross-tab context broadcast.
- `pnpm type-check` passes, including customer adapter preparation, sensitive-word generation, i18n route coverage, and `tsc --noEmit`.
- `git diff --check` reports no whitespace errors for the touched frontend test/docs files.
- zgi-c `api`, `web`, and `postgres` containers remain healthy; API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.

Latest narrow validation, 2026-06-21 08:20 +08:00:

```powershell
go test ./internal/modules/aichat/service -run "TestAIChat(MessageActionsRejectOtherAccountMessage|CreateConversationAllowsOrganizationMemberWithoutWorkspace|ConversationHistoryUsesOrganizationAndAccountScope|GetConversationRejectsOtherAccountConversation|ListMessagesRejectsOtherAccountConversation)" -count=1
go test ./internal/modules/aichat/service -count=1
git diff --check -- api/internal/modules/aichat/service/conversation.go api/internal/modules/aichat/service/conversation_scope_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- AIChat service regressions pass for no-workspace organization-member product chat creation, organization/account scoped conversation-history/message reads, and bare-message delete/stop/regenerate denial for another account's message ID.
- `go test ./internal/modules/aichat/service -count=1` passes.
- `git diff --check` reports no whitespace errors for the touched AIChat service/test/docs files; Windows only reports the expected CRLF warning for the Go file.
- zgi-c API image rebuilds successfully from the current worktree, and `api`, `web`, and `postgres` containers remain healthy.
- API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 08:25 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestMigrateUser" -count=1
go test ./internal/modules/app/workflow -run "Test(MigrateUser|UserMigrationService_MigrateChatRuntimeConversations|AgentHistoryDispatch|WebAppConversation|GetRuntimeLogs|GetWorkflowRunNodeLogs)" -count=1
git diff --check -- api/internal/modules/app/workflow/workflow_migration_handler_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 15
```

Observed result:

- Legacy webapp user-migration handler regressions pass: missing migration context is denied before service work, valid migration uses middleware-derived virtual/authenticated account IDs, and same-account migration preserves its existing error mapping.
- Adjacent workflow history/runtime and webapp conversation subsets still pass with the migration handler coverage included.
- `git diff --check` reports no whitespace errors for the touched migration-handler test/docs files.
- zgi-c `api`, `web`, and `postgres` containers remain healthy. This slice changed tests/docs only, so the API image from the 08:20 rebuild remains current for production code.
- API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.

Latest narrow validation, 2026-06-21 08:32 +08:00:

```powershell
pnpm test:route-access
pnpm type-check
git diff --check -- web/src/app/console/workspace/layout.tsx web/scripts/test-route-access.mjs
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build web
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `pnpm test:route-access` passes and now treats `/console/workspace`, `/console/workspace/members`, and `/console/workspace/settings` as workspace-scoped routes.
- The workspace management layout now consumes `/account/capabilities`, keeps missing-workspace state distinct from denied workspace capability, and still requires concrete `workspace.view` before rendering members/settings children.
- `pnpm type-check` passes, including customer adapter preparation, sensitive-word generation, i18n route coverage, and `tsc --noEmit`.
- `git diff --check` reports no whitespace errors for the touched frontend route guard/test files.
- zgi-c web image rebuilds successfully; Compose also recreates the API image/container from the current worktree.
- `api`, `web`, and `postgres` containers are healthy. API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 08:39 +08:00:

```powershell
go test ./internal/capabilities/chatruntime/service -count=1
go test ./internal/modules/app/workflow -run "TestAgentHistoryDispatch" -count=1
go test ./internal/modules/app/agents -run "Test(WebAppAgentRuntime|AgentRuntime).*" -count=1
git diff --check -- api/internal/capabilities/chatruntime/service/service.go api/internal/capabilities/chatruntime/service/conversation.go api/internal/capabilities/chatruntime/service/stream_caller_scope_test.go api/internal/modules/app/workflow/agent_runtime_history_handler.go api/internal/modules/app/workflow/agent_runtime_history_handler_test.go api/internal/modules/app/agents/runtime_context.go api/internal/modules/app/agents/runtime_context_permission_test.go
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `chatruntime.Service` now exposes `ListConversationMessagesByCaller`, so AGENT/webapp runtime message list paths can enforce caller-scoped conversation ownership inside the service contract before message listing.
- Service-level regression coverage proves a mismatched caller is rejected before `MessageRepository.ListByConversationScoped` can run.
- AGENT runtime conversation-message handlers and workflow runtime history message listing now call the new caller-scoped service method instead of hand-rolling a preflight followed by bare conversation message listing.
- `go test` passes for the chatruntime service package, `AgentHistoryDispatch` workflow regressions, and AGENT/webapp runtime handler regressions.
- `git diff --check` reports no whitespace errors for the touched backend files; Windows only reports the expected CRLF warnings for Go files.
- zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` containers are healthy. API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 08:46 +08:00:

```powershell
go test ./internal/capabilities/chatruntime/service -run "Test(UpdateConversationByCallerRejectsOtherCallerBeforeUpdate|DeleteConversationByCallerRejectsOtherCallerBeforeDelete|StopConversationByCallerRejectsOtherCallerBeforeStop|ListConversationMessagesByCallerRejectsOtherCallerBeforeMessageList)" -count=1
go test ./internal/modules/app/agents -run "Test(WebAppAgentRuntime|AgentRuntime).*Conversation" -count=1
go test ./internal/modules/app/workflow -run "TestAgentHistoryDispatch" -count=1
go test ./internal/capabilities/chatruntime/service -count=1
go test ./internal/modules/app/agents -run "Test(WebAppAgentRuntime|AgentRuntime).*" -count=1
go test ./internal/modules/app/workflow -run "TestAgentHistoryDispatch|TestAgentRuntime" -count=1
git diff --check -- api/internal/capabilities/chatruntime/service/service.go api/internal/capabilities/chatruntime/service/conversation.go api/internal/capabilities/chatruntime/service/stream_caller_scope_test.go api/internal/modules/app/agents/runtime_context.go api/internal/modules/app/agents/runtime_context_permission_test.go api/internal/modules/app/workflow/agent_runtime_history_handler_test.go
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `chatruntime.Service` now exposes caller-scoped update, delete, and stop methods for runtime conversations: `UpdateConversationByCaller`, `DeleteConversationByCaller`, and `StopConversationByCaller`.
- Service-level regressions prove mismatched callers are rejected before `ConversationRepository.UpdateScoped`, `ConversationRepository.DeleteScoped`, or bare stop conversation loading can run.
- AGENT/webapp runtime conversation update/delete/stop handlers now call the caller-scoped service methods. Update still keeps the handler-level preflight before JSON binding so cross-caller requests do not receive request-shape feedback first.
- Targeted chatruntime, AGENT/webapp runtime, and AgentHistoryDispatch regressions pass.
- `git diff --check` reports no whitespace errors for the touched backend/docs files; Windows only reports the expected CRLF warnings for Go files.
- zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` containers are healthy. API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 08:54 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestConversationQueryHandler(DetailRejectsForeignAccountBeforeMessages|DetailRejectsOtherAgentBeforeMessages|DeleteRejectsForeignAccountBeforeDelete)" -count=1
go test ./internal/modules/app/workflow -run "Test(ValidateWebAppConversationAccess|RunAdvancedChatDraftWorkflow_RejectsForeignConversationBeforeRunning|RunAdvancedChatWorkflow_RejectsForeignConversationBeforeRunning|RunDraftWorkflow_RejectsForeignSystemConversationBeforeRunning|RunPublishedWorkflow_RejectsForeignSystemConversationBeforeRunning|ConversationQueryHandler)" -count=1
go test ./internal/modules/app/workflow -count=1
git diff --check -- api/internal/modules/app/workflow/workflow_webapp_conversation_access_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `ConversationQueryHandler` now has HTTP handler-level regressions for legacy published workflow webapp conversation detail/delete under `/console/api/workflows/:web_app_id/conversations/:conversation_id`.
- Detail rejects a foreign-account conversation before `AgentMessageService.GetConversationMessages` and rejects another agent's conversation before message lookup.
- Delete rejects a foreign-account conversation before `AgentConversationService.DeleteConversation`.
- The focused workflow conversation access subset and the whole `./internal/modules/app/workflow` package pass.
- `git diff --check` reports no whitespace errors for the touched test/docs files.
- zgi-c `api`, `web`, and `postgres` containers are healthy. API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 09:01 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestGenerateWebAppConversationTitle|TestBuildWorkflowConversationTitleMessages|TestIsDefaultWorkflowConversationName" -count=1
go test ./internal/modules/app/workflow -count=1
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `WorkflowService.generateWebAppConversationTitle` now uses `GetConversationByIDAndAgent` and `conversationBelongsToAccount` before loading conversation messages or invoking the title generator.
- A targeted regression proves a foreign-account conversation is rejected before message lookup and before the title model call.
- The title-generation subset and the whole `./internal/modules/app/workflow` package pass.
- zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` containers are healthy. API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 09:09 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "Test(WorkflowConversationMetadataRejectsForeignAccountBeforeMessages|WorkflowServiceDialogueCountUsesCallerScopedConversation|ValidateWebAppConversationAccess|RunAdvancedChatDraftWorkflow_RejectsForeignConversationBeforeRunning|RunAdvancedChatWorkflow_RejectsForeignConversationBeforeRunning|RunDraftWorkflow_RejectsForeignSystemConversationBeforeRunning|RunPublishedWorkflow_RejectsForeignSystemConversationBeforeRunning|ConversationQueryHandler)" -count=1
go test ./internal/modules/app/workflow -count=1
git diff --check -- api/internal/modules/app/workflow/workflow_webapp_conversation_access.go api/internal/modules/app/workflow/workflow_conversation_support.go api/internal/modules/app/workflow/workflow_advanced_chat_run.go api/internal/modules/app/workflow/workflow_webapp_run.go api/internal/modules/app/workflow/workflow_service.go api/internal/modules/app/workflow/workflow_webapp_conversation_access_test.go
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml build --pull=false api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 15
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 15
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `validateWebAppConversationAccess` now delegates to `loadWebAppConversationForCaller`, so metadata readers can reuse the same scoped conversation record instead of repeating weaker bare-ID reads.
- `WorkflowHandler.getLatestMessageIDForCaller` and `getDialogueCountForCaller` now require agent-scoped conversation ownership before reading messages or dialogue counts. `WorkflowService.getDialogueCountForCaller` uses the same loader and no longer opens a bare conversation repository read.
- Focused regressions prove a foreign-account conversation is rejected before parent-message message lookup and that service-layer dialogue count uses caller-scoped conversation ownership.
- The focused workflow conversation access subset and the whole `./internal/modules/app/workflow` package pass. `git diff --check` reports no whitespace errors; Windows only reports expected CRLF warnings for touched Go files.
- zgi-c API rebuild could not complete in this attempt because Docker Hub returned `EOF` while resolving `docker.io/library/golang:1.26.2-alpine`. A retry and a non-BuildKit `--pull=false` build hit the same registry metadata error.
- Existing zgi-c containers remained healthy from the previous successful build: `api`, `web`, and `postgres` are healthy; API health returns `{"message":"pong"}` and web health returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 09:20 +08:00:

```powershell
go test ./internal/modules/app/agents -run "Test(WebAppAgentRuntimeStopConversationRequiresCallerScopedConversationBeforeStop|AgentRuntimeStopConversationRequiresCallerScopedConversationBeforeStop)" -count=1
go test ./internal/modules/app/workflow -run "Test(GetWorkflowRunNodeLogs|GetWorkflowRunEvents|ValidateWorkflowRunAccess|ValidateWorkflowRunNodeScope)" -count=1
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- AGENT and webapp runtime conversation stop denial tests now also assert that `StopWorkflowContinuation` is not called when `StopConversationByCaller` rejects the caller-scoped conversation. This pins the workflow-run stop hook behind the conversation ownership gate before any metadata `workflow_run_id` can be acted on.
- Legacy workflow runtime log/event regressions still pass for node log access, workflow-run event access, workflow-run ownership, and node-log scope checks.
- The zgi-c API image rebuilds successfully again from the current worktree after the Docker Hub metadata outage. `api`, `web`, and `postgres` are healthy; API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 09:27 +08:00:

```powershell
go test ./internal/modules/app/agents -run "TestAgentWorkflowContinuationApprovalForm|Test(WebAppAgentRuntime|AgentRuntime).*Continuation" -count=1
git diff --check -- api/internal/modules/app/agents/workflow_continuation.go api/internal/modules/app/agents/workflow_continuation_permission_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `ensureAgentWorkflowContinuationApprovalForm` now pins approval-token resume to the already caller-scoped workflow continuation run. A valid approval token for another `workflow_run_id` returns `chatruntime.ErrNotFound` before pause readiness checks or approval resume runner calls can execute.
- The existing console-agent and published-webapp continuation denial tests still pass, keeping cross-caller conversation/message IDs denied before continuation stream handling.
- `git diff --check` reports no whitespace errors; Windows only reports the expected CRLF warning for the touched Go file.
- The zgi-c API image rebuilds successfully from the current worktree. `api`, `web`, and `postgres` are healthy; API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 09:33 +08:00:

```powershell
go test ./internal/modules/app/workflow/approval -run "Test(SubmitByTokenForWorkflowRun|EnsureFormWorkflowRun)" -count=1
go test ./internal/modules/app/agents -run "TestAgentWorkflowContinuationApprovalForm|Test(WebAppAgentRuntime|AgentRuntime).*Continuation" -count=1
git diff --check -- api/internal/modules/app/workflow/approval/service.go api/internal/modules/app/workflow/approval/service_workflow_run_test.go api/internal/modules/app/agents/workflow_continuation.go api/internal/modules/app/agents/workflow_continuation_permission_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
cd docker
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `approval.Service.SubmitByTokenForWorkflowRun` now loads the approval token, checks the form's `workflow_run_id`, and only then delegates to the existing submit/update path. The sqlmock regression allows only the token lookup query for a mismatched run; any accidental form update would fail the test.
- AGENT continuation now uses `SubmitByTokenForWorkflowRun`, maps cross-run approval tokens to runtime not-found, and keeps the post-submit continuation/form run assertion as a defensive backstop.
- Console-agent and published-webapp continuation denial tests still pass.
- `git diff --check` reports no whitespace errors; Windows only reports the expected CRLF warnings for touched Go files.
- The zgi-c API image rebuilds successfully from the current worktree. `api`, `web`, and `postgres` are healthy; API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 10:13 +08:00:

```powershell
go test ./internal/modules/file_process/handler -run "Test(GetFileStatisticsReturnsZeroWithoutVisibleWorkspace|GetFileStatisticsScopesServiceToVisibleWorkspaces|PatchFolderRequiresManageBeforeBindingRequest|AuthorizeFileViewAccessAllowsWorkspaceDownloadPermission)" -count=1
go test ./internal/modules/file_process/repository -run TestTotalFileCountWithVisibilityAppliesWorkspaceAndFolderAccessFilters -count=1
go test ./internal/modules/file_process/handler ./internal/modules/file_process/service ./internal/modules/file_process/repository -count=1
go test ./internal/modules/file_process/... -count=1
git diff --check -- api/internal/modules/file_process/handler/file_access_test.go api/internal/modules/file_process/handler/file_resource_handler.go api/internal/modules/file_process/repository/file_folder_repository.go api/internal/modules/file_process/repository/file_folder_repository_statistics_test.go api/internal/modules/file_process/service/file_resource_service.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- File statistics now resolve visible file workspaces before service calls. No-visible-workspace callers get zero statistics without invoking the statistics service; callers with mixed visible/denied/archived workspaces only pass visible normal workspace IDs to the service.
- Repository sqlmock coverage verifies the total-count statistics query includes both `upload_files.workspace_id IN (...)` and folder visibility subqueries before counting files.
- Handler, service, and repository package tests pass for the touched path.
- The broader `go test ./internal/modules/file_process/... -count=1` reaches the touched packages but fails in `service/extractor` on Windows temp cleanup for `TestExcelExtractorHandleXlsReadsLegacyWorkbook`: `legacy.xls` remains locked during `TempDir RemoveAll`. This appears unrelated to the statistics authorization change and is recorded as residual validation noise rather than a permission regression.
- `git diff --check` reports no whitespace errors; Windows only reports CRLF warnings for touched Go files.
- The zgi-c API image rebuilds successfully from the current worktree using `docker/.env` and the compose files under `docker/`. `api`, `web`, and `postgres` are healthy; API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 10:37 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestWorkflowStatistic" -count=1
go test ./internal/modules/app/workflow -run "Test(WorkflowStatistic|AgentHistoryDispatch|AgentRuntime|GetWorkflowRun|GetRuntimeLogs|ValidateWorkflowRunAccess|ManualDiagnoseNode|StopWorkflowTask)" -count=1
gofmt -w api/internal/modules/app/workflow/workflow_statistic_handler.go api/internal/modules/app/workflow/workflow_statistic_handler_test.go
git diff --check -- api/internal/modules/app/workflow/workflow_statistic_handler.go api/internal/modules/app/workflow/workflow_statistic_handler_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- The unregistered legacy `WorkflowStatisticHandler` now has a fail-closed route-agent `agent.view` preflight and uses the resolved agent workspace instead of ambient `tenant_id` before statistic calls.
- Focused regressions cover all four statistic handler entry points denying missing `agent.view` before malformed query binding or service calls, plus the allowed path using the route agent workspace.
- The wider workflow permission subset still passes after the statistic handler hardening.
- Future statistics route restoration must pass `WithWorkflowStatisticAuthorization`; otherwise the handler returns a system error instead of serving statistics without a workspace permission checker.
- The zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`; published runtime surface counts remain `api=13`, `builtin_app=13`, `internal=13`, and `webapp=13`.

Previous narrow validation, 2026-06-21 10:31 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "Test(AgentHistoryDispatch|AgentRuntime|GetWorkflowRun|GetRuntimeLogs|ValidateWorkflowRunAccess|ManualDiagnoseNode|StopWorkflowTask)" -count=1
git diff --check -- docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
```

Observed result:

- The workflow permission subset passes, including `AgentHistoryDispatch`, AGENT runtime runs, workflow run query/node log/event/control checks, runtime-log list checks, and workflow-run access service regressions.
- `git diff --check` is clean for the touched permission docs.
- The follow-up route inventory for old workflow history/log and non-conversation runtime subresources found no additional active route outside the existing guarded handler set.
- Console builder workflow run detail/list/node views continue to use route-agent workspace permission plus `agent_id` run scope; raw `workflow_run_logs.tenant_id` equality remains a documented migration/backfill decision because historical rows may store caller workspace or app workspace.
- `WorkflowStatisticHandler` is currently unregistered; its code path now fails closed without a workspace permission checker and applies route-agent `agent.view` before query binding or statistic calls when wired.

Previous narrow validation, 2026-06-21 10:21 +08:00:

```powershell
go test ./internal/modules/app/runtimeauth -count=1
go test ./internal/modules/app/agents -run "Test(AgentRuntimeAuthorizationFromUpdateRequest_RejectsInvalidSurfaceGrantSubjects|AgentsService_UpdateAgentRuntimeSurfaces_RejectsInternalDisable|AgentsService_UpdateAgentRuntimeSurfaces_RequiresBuiltinGrantWhenEnabled|AgentsService_UpdateAgentRuntimeSurfaces_RejectsAccountGrantOutsideOrganization|AgentsService_UpdateAgentRuntimeSurfaces_RejectsDepartmentGrantOutsideOrganization|AgentsService_GetAgentRuntimeSurfaces_UsesPersistedAuthorizationWithLegacyFallback)" -count=1
go test ./middleware -run "Test.*API.*Surface|TestValidateAPIKey" -count=1
go test ./internal/modules/app/workflow -run TestBuiltInWorkflowScenarioDetailHonorsBuiltinAppAccountGrant -count=1
git diff --check -- api/internal/modules/app/runtimeauth/published_runtime_store.go api/internal/modules/app/runtimeauth/published_runtime_store_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT surface, COUNT(*) FROM published_runtime_surfaces GROUP BY surface ORDER BY surface;"
```

Observed result:

- `runtimeauth.Store.SaveResourceAuthorization` now denies unsupported surface/grant subject combinations before SQL: webapp/API non-public grants, internal non-internal grants, and builtin public grants cannot be persisted through the generic store path.
- Agent runtime-surface service tests still pass for existing helper-level validation and cross-organization account/department rejection.
- API key middleware tests still pass for persisted API surface enable/disable behavior.
- Built-in workflow runtime authorization still honors account grants through the shared runtimeauth evaluator.
- `git diff --check` is clean for the touched runtimeauth files and permission docs.
- The zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`; published runtime surface counts remain `api=13`, `builtin_app=13`, `internal=13`, and `webapp=13`.

Latest narrow validation, 2026-06-21 10:00 +08:00:

```powershell
go test ./internal/modules/dataset/handler -run "Test(GetDocumentSegmentQuestionRejectsQuestionOutsideRouteBeforeResponse|UpdateDocumentSegmentQuestionRejectsQuestionOutsideRouteBeforeMutation|DeleteDocumentSegmentQuestionRejectsQuestionOutsideRouteBeforeMutation|UpdateDocumentSegmentQuestionRequiresManageBeforeBindingRequest|GenerateQuestionsForSegmentRequiresManageBeforeCountValidation)" -count=1
git diff --check -- api/internal/modules/dataset/handler/dataset_access_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Segment-question detail now rejects a guessed `question_id` whose row belongs to another route segment before returning question content.
- Existing segment-question update/delete path-ownership regressions and permission-order regressions still pass.
- `git diff --check` is clean for the touched dataset test and permission planning docs.
- The zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 09:57 +08:00:

```powershell
go test ./internal/modules/dataset/handler -run "Test(UpdateDocumentSegmentQuestionRejectsQuestionOutsideRouteBeforeMutation|DeleteDocumentSegmentQuestionRejectsQuestionOutsideRouteBeforeMutation|UpdateDocumentSegmentQuestionRequiresManageBeforeBindingRequest|GenerateQuestionsForSegmentRequiresManageBeforeCountValidation|CreateDocumentSegmentQuestionRequiresManageBeforeBindingRequest|BatchCreateDocumentSegmentQuestionsRequiresManageBeforeBindingRequest)" -count=1
git diff --check -- api/internal/modules/dataset/handler/dataset_access_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Segment-question update/delete now reject a guessed `question_id` whose row belongs to another route segment before update/delete mutation service calls.
- Existing segment-question permission-order regressions still pass for create, batch-create, generate, and update body binding.
- `git diff --check` is clean for the touched dataset test and permission planning docs.
- The zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 09:52 +08:00:

```powershell
go test ./internal/modules/dataset/handler -run "Test(CreateDocumentSegmentQuestionRequiresManageBeforeBindingRequest|BatchCreateDocumentSegmentQuestionsRequiresManageBeforeBindingRequest|GenerateQuestionsForSegmentRequiresManageBeforeCountValidation|UpdateDocumentSegmentQuestionRequiresManageBeforeBindingRequest|AuthorizeDatasetChildChunkAccess|AuthorizeDatasetSegmentViewAccess)" -count=1
git diff --check -- api/internal/modules/dataset/handler/segment_handler.go api/internal/modules/dataset/handler/dataset_access_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Segment-question generate now denies missing `knowledge_base.manage` before validating malformed `count` query values or calling generate service work.
- Segment-question update now denies missing `knowledge_base.manage` before reading a guessed `question_id`, binding malformed JSON bodies, or calling update service work.
- Existing segment-question create/batch-create, segment scope, and child-chunk scope regressions still pass.
- `git diff --check` reports no whitespace errors for the dataset handler/test and permission planning docs; the only output is the existing CRLF normalization warning for `segment_handler.go`.
- The zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 09:48 +08:00:

```powershell
go test ./internal/modules/dataset/handler -run "Test(CreateDocumentSegmentQuestionRequiresManageBeforeBindingRequest|BatchCreateDocumentSegmentQuestionsRequiresManageBeforeBindingRequest|AuthorizeDatasetChildChunkAccess|AuthorizeDatasetSegmentViewAccess|UpdateDocumentStatusRequiresManageBeforeDocumentIDValidation)" -count=1
git diff --check -- api/internal/modules/dataset/handler/segment_handler.go api/internal/modules/dataset/handler/dataset_access_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Segment-question create and batch-create now deny missing `knowledge_base.manage` through route dataset/document/segment scope before JSON body binding or create service calls, so malformed bodies cannot reveal request-shape feedback ahead of permission failure.
- Existing dataset segment and child-chunk scope regressions still pass for segment/document ownership and child-chunk-to-segment binding.
- `git diff --check` reports no whitespace errors for the dataset handler/test and permission planning docs; the only output is the existing CRLF normalization warning for `segment_handler.go`.
- The zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 09:41 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "Test(AgentWorkflowHistoryChatMessagesRejectsOtherAgentConversationBeforeMessageQuery|AgentHistoryDispatch)" -count=1
git diff --check -- api/internal/modules/app/workflow/agent_workflow_history_handler_permission_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml up -d --build api
docker compose --env-file .env -f docker-compose.yaml -f compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `AgentWorkflowHistoryHandler.GetChatMessages` now has a denial-order regression proving the route conversation is checked with `GetConversationByIDAndAgent` before `GetMessagesByConversation`.
- Existing `AgentHistoryDispatchHandler` regressions still pass for builder `agent.view` gating and runtime caller-scoped history/log behavior.
- `git diff --check` is clean for the new workflow regression and the permission planning docs.
- The zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 10:50 +08:00:

```powershell
pnpm test:route-access
pnpm type-check
git diff --check -- web/src/components/runtime-auth/runtime-grant-subject-row.tsx web/src/components/agents/api/runtime-access-tab.tsx web/src/components/dashboard/organization/built-in-workflow-runtime-section.tsx web/src/i18n/modules/agents/en-US.ts web/src/i18n/modules/agents/zh-Hans.ts web/src/i18n/modules/dashboard/en-US.ts web/src/i18n/modules/dashboard/zh-Hans.ts web/scripts/test-route-access.mjs docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build web
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `RuntimeGrantSubjectRow` now resolves saved department grants against the current organization department tree and saved account grants through current-organization member hydration; unresolved saved account or department IDs are displayed as unavailable rows with the raw ID retained.
- `pnpm test:route-access` now includes static coverage for unresolved runtime grant display and still passes the existing organization/workspace route capability checks.
- `pnpm type-check` passes after the new shared label contract is consumed by both agent Publication Access and built-in workflow runtime management UIs.
- `git diff --check` is clean for the touched frontend files and permission planning docs.
- The zgi-c web image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 10:56 +08:00:

```powershell
pnpm test:route-access
pnpm type-check
git diff --check -- web/src/components/runtime-auth/runtime-grant-subject-row.tsx web/src/components/agents/api/runtime-access-tab.tsx web/src/components/dashboard/organization/built-in-workflow-runtime-section.tsx web/src/i18n/modules/agents/en-US.ts web/src/i18n/modules/agents/zh-Hans.ts web/src/i18n/modules/dashboard/en-US.ts web/src/i18n/modules/dashboard/zh-Hans.ts web/scripts/test-route-access.mjs
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build web
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `RuntimeGrantSubjectRow` now distinguishes incomplete account/department grant rows, account lookup failures, department tree lookup failures, and stale saved account/department IDs. Stale rows keep the raw ID; lookup failures do not masquerade as valid saved subjects.
- `pnpm test:route-access` now includes static coverage for the runtime grant incomplete/error/stale display contract and still passes the organization/workspace route capability checks.
- `pnpm type-check` passes after the expanded shared label contract is consumed by both agent Publication Access and built-in workflow runtime management UIs.
- `git diff --check` is clean for the touched frontend files.
- The zgi-c web image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 11:04 +08:00:

```powershell
go test ./internal/modules/user/auth/service -run "TestGetAccountCapabilities(OrganizationModeAllowsProductSurfacesOnly|WithoutOrganizationKeepsRuntimeSurfaceContractDisabled|RuntimeAudienceIncludesActiveDepartments)" -count=1
go test ./internal/modules/user/auth/service -run "Test(GetAccountCapabilities|GetAccountContext|UpdateAccountContext|EnsureAccountContext)" -count=1
git diff --check -- api/internal/modules/user/auth/service/account_service.go api/internal/modules/user/auth/service/account_service_context_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Account capabilities now has focused regressions for the complete runtime surface contract: `webapp` and `api` expose only `public` grant subjects, `builtin_app` exposes `organization|department|account`, and `internal` exposes only `internal`.
- The same contract is returned with `enabled=false` when the account has no organization context, so frontend route/access logic can consume stable metadata without inferring that webapp/API support account or department grants.
- `populateDefaultOrganization` now returns false when the organization service is not available instead of panicking during account context repair. This keeps account capabilities fail-closed in tests and defensive runtime paths.
- The adjacent account context and capabilities tests pass.
- `git diff --check` is clean for the touched backend files and permission planning docs; Windows only reports the existing CRLF normalization warning for the touched Go files.
- The zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 11:10 +08:00:

```powershell
go test ./internal/modules/app/agents -run "TestAgentsHandler_(WebAppFileEndpointsRejectOfflineBeforeAuthOrFileValidation|GetWebAppRuntimeConfig_MapsNotPublishedError)|TestAgentsService_GetPublishedAgentWebAppConfig_RejectsPersistedDisabledWebApp" -count=1
go test ./internal/modules/app/agents -run "Test(AgentsHandler_|AgentsService_GetPublishedAgentWebAppConfig|AgentsService_UpdateAgentRuntimeSurfaces|AgentRuntimeAuthorizationFromUpdateRequest|WebAppAgentRuntime)" -count=1
git diff --check -- api/internal/modules/app/agents/agents_webapp_handler_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- AGENT webapp upload-config and file-upload endpoints now have denial-order regressions proving `webapp` surface offline is returned before authenticated-file-access, multipart parsing, or file validation.
- Existing AGENT webapp config offline mapping and persisted disabled webapp service checks still pass.
- Adjacent AGENT webapp handler/runtime-surface tests pass.
- `git diff --check` is clean for the touched AGENT handler test and permission planning docs; Windows only reports the existing CRLF normalization warning for the touched Go file.
- zgi-c `api`, `web`, and `postgres` remain healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 11:15 +08:00:

```powershell
git diff --check -- docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- The endpoint inventory now has an explicit decision table for future webapp/API account or department grants: webapp public-only vs optional-auth config vs gated config, webapp migration route/proof options, and API bearer-key vs owner-audience vs per-request caller-claim options.
- The MVP plan points remaining webapp/API and migrate-user work at that decision table, keeping the current public-only webapp/API contract intentional rather than accidental.
- `git diff --check` is clean for the permission planning docs.
- zgi-c `api`, `web`, and `postgres` remain healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 11:20 +08:00:

```powershell
pnpm test:route-access
pnpm type-check
git diff --check -- web/scripts/test-route-access.mjs docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `pnpm test:route-access` now statically locks the agent Publication Access UI contract: webapp and API save only public grants, builtin app is the only editable organization/account/department grant surface, and internal runtime stays enabled with an internal grant.
- `pnpm type-check` passes with the current frontend worktree.
- `git diff --check` is clean for the frontend route-access script and permission planning docs.
- zgi-c `api`, `web`, and `postgres` remain healthy without a rebuild for this script/docs-only change. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 11:25 +08:00:

```powershell
go test ./internal/modules/app/workflowtest -run "TestRepositoryRecoverStaleRunning(GenerationTasks|ScenarioRecognitionTasks)|TestHandler(MutationRoutesRequireAgentManageBeforeBindingRequest|MutationRoutesRequireAgentManageBeforeSubresourceIDValidation|ListBatchItemsRequiresAgentViewPermission|UpdateCaseRequiresAgentManagePermission|WorkflowTestAccessRejectsAgentWithoutWorkspace)" -count=1
go test ./internal/modules/app/workflowtest -count=1
git diff --check -- api/internal/modules/app/workflowtest/repository.go api/internal/modules/app/workflowtest/service.go api/internal/modules/app/workflowtest/handler.go api/internal/modules/app/workflowtest/generation_task_test.go api/internal/modules/app/workflowtest/scenario_recognition_task_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Workflow-test HTTP-triggered stale generation-task and scenario-recognition-task recovery now has agent-scoped repository coverage: the new `ForAgent` paths update only rows whose `agent_id` matches the route agent.
- The existing global repository recovery tests still pass, preserving local worker compatibility for background task repair.
- The adjacent handler permission regressions still pass for view/manage denial, permission-before-subresource-ID validation, permission-before-body-binding, and empty-workspace failure.
- The full `workflowtest` package passes.
- `git diff --check` is clean for touched workflow-test code and permission planning docs; Windows only reports existing CRLF normalization warnings for touched Go files.
- The zgi-c API image rebuilds successfully from the current worktree; `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 11:31 +08:00:

```powershell
go test ./routes/v1 -run "TestWorkflowRoutes_DoNotRegisterLegacyWorkflowStatisticRoutes" -count=1
go test ./internal/modules/app/workflow -run "TestWorkflowStatistic" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_(DoNotRegisterLegacyWorkflowStatisticRoutes|BuiltInWorkflow|WebApp|Run|Conversation|Precheck)" -count=1
go test ./internal/modules/app/workflow -run "Test(WorkflowStatistic|AgentHistoryDispatch|AgentRuntime|GetWorkflowRun|GetRuntimeLogs|ValidateWorkflowRunAccess|ManualDiagnoseNode|StopWorkflowTask)" -count=1
git diff --check -- api/routes/v1/workflow_routes_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Workflow routes now have a route-table regression proving legacy workflow statistics routes are not registered and `WorkflowStatisticHandler` is not wired by accident.
- The direct `WorkflowStatisticHandler` regressions still pass: if it is called directly or restored later, it fails closed without authorization, requires route-agent `agent.view` before query binding, and uses the resolved agent workspace rather than ambient `tenant_id`.
- The broader workflow route/runtime subset still passes for built-in workflow runtime grants, webapp runtime compatibility, run/precheck/conversation paths, workflow statistics, history dispatch, runtime logs, workflow-run access, manual diagnosis, and task stop coverage.
- `git diff --check` is clean for the route test and permission planning docs; Windows only reports the existing CRLF normalization warning for the touched Go test file.
- zgi-c `api`, `web`, and `postgres` remain healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 11:36 +08:00:

```powershell
go test ./routes/v1 -run "TestWorkflowRoutes_BuiltInWorkflowDetail" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_BuiltInWorkflow" -count=1
git diff --check -- api/routes/v1/workflow_routes_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Built-in workflow direct scenario detail now has route regressions for both sides of persisted `builtin_app` grants: account grants for another account return forbidden on direct detail, while department grants containing the caller allow direct detail and return the requested scenario.
- The broader built-in workflow route subset still passes, covering catalog filtering, runtime-surface management, invalid external account/department grants, non-admin writes, and the new direct detail checks together.
- This slice adds route regression coverage and documentation only; production code and frontend code were not changed after the previous API rebuild.
- `git diff --check` is clean for the route test and permission planning docs; Windows only reports the existing CRLF normalization warning for the touched Go test file.
- zgi-c `api`, `web`, and `postgres` remain healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 11:41 +08:00:

```powershell
go test ./internal/modules/file_process/handler -run "TestMoveFilesToFolderRequires" -count=1
go test ./internal/modules/file_process/handler -count=1
git diff --check -- api/internal/modules/file_process/handler/file_access_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `POST /file-folders/move-files` now has focused handler regressions for both write gates: target folder `file.manage` denial stops before `MoveFilesToFolder`, and a guessed file lacking `file.manage` also stops before folder-join mutation.
- The full `internal/modules/file_process/handler` package still passes, preserving the adjacent file preview, metadata, image preview, statistics, folder patch, and visibility regressions.
- This slice adds file asset mutation regression coverage only; production code and frontend code were not changed after the previous API rebuild.
- `git diff --check` is clean for the file handler regression and permission planning docs; Windows only reports the existing CRLF normalization warning for the touched Go test file.
- zgi-c `api`, `web`, and `postgres` remain healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 11:47 +08:00:

```powershell
go test ./internal/modules/dataset/handler -run "Test(GetBatchHitTestingTaskStatusRejectsTaskOutsideRouteDataset|GetBatchHitTestingTaskReportRejectsForeignAccountBeforeReport|StopBatchHitTestingTaskRejectsTaskOutsideRouteDatasetBeforeStop|SaveBatchHitTestingResultsRejectsForeignAccountBeforeBindingRequest)" -count=1
go test ./internal/modules/dataset/handler -count=1
git diff --check -- api/internal/modules/dataset/handler/dataset_handler.go api/internal/modules/dataset/handler/dataset_access_test.go docs/superpowers/plans/2026-06-20-permission-foundation-mvp.md docs/superpowers/plans/2026-06-20-permission-endpoint-inventory.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Dataset async batch-hit-testing task status/report/stop/save now share a route-task authorization helper: the task must belong to the route dataset, current account, and current organization, and the route dataset access is re-checked before task data or mutation is allowed.
- Focused regressions cover status denial for a task from another dataset, report denial for another account's task, stop denial before task cancellation when the route dataset is wrong, and save denial before malformed-body binding or `SaveBatchHitTestingResults` when the task belongs to another account.
- The full `internal/modules/dataset/handler` package still passes, preserving the adjacent document, segment, child-chunk, segment-question, folder, and dataset graph/content-parse regressions.
- `git diff --check` has no whitespace errors for the dataset handler/test and permission planning docs; Windows only reports the existing CRLF normalization warning for the touched Go handler file.
- The zgi-c API image rebuilt successfully from the current worktree. `api`, `web`, and `postgres` remain healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 11:54 +08:00:

```powershell
go test ./internal/modules/datasource/handler -run "TestExcelImport(ReadRoutesRequireDatabasePermissionBeforeJobLookup|MutationsRequireDatabaseManageBeforeBindingRequest)" -count=1
go test ./internal/modules/datasource/handler -count=1
git diff --no-index --check -- NUL api\internal\modules\datasource\handler\excel_import_handler_permission_test.go
git diff --no-index --check -- NUL docs\superpowers\plans\2026-06-20-permission-endpoint-inventory.md
git diff --no-index --check -- NUL docs\superpowers\plans\2026-06-20-permission-foundation-mvp.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Excel import read routes now have focused handler regressions: `GET /data-dbs/:id/excel-import/jobs/:job_id` requires `database.view` before job lookup, while `GET /data-dbs/:id/excel-import/jobs/:job_id/errors` requires `database.manage` before error lookup.
- Existing service code already binds Excel import jobs to the current organization and route datasource before returning job/error data or confirming/recognizing import state; this slice locks the handler permission boundary without changing production logic.
- The focused Excel import tests and full `internal/modules/datasource/handler` package pass. `git diff --no-index --check` reports no whitespace errors for the newly tracked-by-planning test/doc files; Windows only reports LF/CRLF normalization warnings.
- The zgi-c API image rebuilt successfully from the current worktree. `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 12:00 +08:00:

```powershell
go test ./internal/modules/app/workflowtest -run "TestHandler(TaskReadRoutesRequireAgentViewBeforeTaskLookup|TaskCancelRoutesRequireAgentManageBeforeTaskLookup|ListBatchItemsRequiresAgentViewPermission|MutationRoutesRequireAgentManageBeforeSubresourceIDValidation|MutationRoutesRequireAgentManageBeforeBindingRequest)" -count=1
go test ./internal/modules/app/workflowtest -count=1
git diff --no-index --check -- NUL api\internal\modules\app\workflowtest\handler_permission_test.go
git diff --no-index --check -- NUL docs\superpowers\plans\2026-06-20-permission-endpoint-inventory.md
git diff --no-index --check -- NUL docs\superpowers\plans\2026-06-20-permission-foundation-mvp.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Workflow-test task read routes now have focused regressions proving missing `agent.view` denies active/latest/specific generation-task and scenario-recognition-task reads before stale recovery or task lookup.
- Workflow-test task cancel routes now have focused regressions proving missing `agent.manage` denies generation-task and scenario-recognition-task cancellation before task lookup/cancel work.
- Existing repository/service code already scopes HTTP task reads and cancellations by route `agent_id`; this slice locks the handler permission boundary without changing production logic.
- The focused workflow-test handler permission tests and full `internal/modules/app/workflowtest` package pass. `git diff --no-index --check` reports no whitespace errors for the touched workflow-test handler regression and permission planning docs; Windows only reports LF/CRLF normalization warnings.
- The zgi-c API image rebuilt successfully from the current worktree. `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 12:06 +08:00:

```powershell
pnpm test:route-access
pnpm type-check
git diff --no-index --check -- NUL web\scripts\test-route-access.mjs
git diff --no-index --check -- NUL docs\superpowers\plans\2026-06-20-permission-endpoint-inventory.md
git diff --no-index --check -- NUL docs\superpowers\plans\2026-06-20-permission-foundation-mvp.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- `pnpm test:route-access` now derives the actual `src/app/console/work` page tree and verifies each page route is explicitly classified by shared access metadata. Current organization-scoped work routes are `/console/work`, `/console/work/chat`, `/console/work/image`, `/console/work/app`, and `/console/work/app/:web_app_id`; `/console/work/task` remains workspace-scoped.
- `pnpm type-check` passes, including customer adapter preparation, sensitive-word generation, i18n route coverage, and `tsc --noEmit`.
- `git diff --no-index --check` reports no whitespace errors for the route-access script and permission planning docs; Windows only reports LF/CRLF normalization warnings.
- The zgi-c API image rebuilt successfully from the current worktree. `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest narrow validation, 2026-06-21 12:10 +08:00:

```powershell
go test ./internal/modules/user/auth/handler -run TestAccountHandlerGetAccountCapabilitiesReturnsContract -count=1
go test ./internal/modules/user/auth/handler -count=1
git diff --no-index --check -- NUL api\internal\modules\user\auth\handler\account_handler_context_test.go
git diff --no-index --check -- NUL docs\superpowers\plans\2026-06-20-permission-endpoint-inventory.md
git diff --no-index --check -- NUL docs\superpowers\plans\2026-06-20-permission-foundation-mvp.md
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- Account capabilities handler regression now locks the HTTP JSON contract consumed by frontend route guards in organization/no-workspace mode: organization product surfaces are enabled, workspace is unavailable and requires workspace, route flags allow organization scope while denying workspace scope, runtime audience includes organization/account, and `webapp`/`api`/`builtin_app`/`internal` surface metadata is present.
- The focused handler contract test and full `internal/modules/user/auth/handler` package pass.
- `git diff --no-index --check` reports no whitespace errors for the account capabilities handler regression and permission planning docs; Windows only reports LF/CRLF normalization warnings.
- The zgi-c API image rebuilt successfully from the current worktree. `api`, `web`, and `postgres` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Latest aggregate validation, 2026-06-21 12:17 +08:00:

```powershell
go test ./internal/modules/user/auth/handler ./internal/modules/user/auth/service ./internal/modules/workspace/handler ./internal/modules/datasource/handler ./internal/modules/datasource/service ./internal/modules/datasource/service/excelimport ./internal/modules/file_process/handler ./internal/modules/file_process/repository ./internal/modules/file_process/service/extractor/hyperparse ./internal/modules/dataset/handler ./internal/modules/dataset/graphflow/openie ./internal/modules/app/runtimeauth ./internal/modules/app/workflow ./internal/modules/app/workflowtest ./middleware -count=1
```

Observed result:

- The stable permission aggregate passes for account context/capabilities, workspace handlers, datasource/database access, file access, dataset access, runtime authorization, workflow/workflow-test authorization, and middleware.
- The previous aggregate command that targeted module root directories is stale for the current repository layout because several roots contain no Go files. Use the package-level command above for local permission regression sweeps.
- At this checkpoint, `CGO_ENABLED=1` could not validate sqlite-backed packages on this Windows host because `gcc` was not installed, and `CGO_ENABLED=0` kept sqlite-dependent `internal/modules/app/agents` tests out of the aggregate. The 12:22 validation below adds `app/agents` back after narrowing the sqlite-unavailable behavior.
- A broad `file_process/service/extractor/...` run can fail on Windows `.xls` temp cleanup with a held file handle. The stable aggregate intentionally keeps `hyperparse` coverage and avoids treating that extractor cleanup issue as a permission regression.

Latest aggregate validation with agent runtime surfaces, 2026-06-21 12:22 +08:00:

```powershell
go test ./internal/modules/app/agents -run "Test(AgentsService_GetRunnableWebApps|AgentsService_GetAgentRuntimeSurfaces|AgentsService_UpdateAgentRuntimeSurfaces|AgentRuntimeAuthorizationFromUpdateRequest|WebAppAgentRuntime|AgentRuntime|AgentWorkflowContinuation)" -count=1
go test ./internal/modules/app/agents -count=1
go test ./internal/modules/user/auth/handler ./internal/modules/user/auth/service ./internal/modules/workspace/handler ./internal/modules/datasource/handler ./internal/modules/datasource/service ./internal/modules/datasource/service/excelimport ./internal/modules/file_process/handler ./internal/modules/file_process/repository ./internal/modules/file_process/service/extractor/hyperparse ./internal/modules/dataset/handler ./internal/modules/dataset/graphflow/openie ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./internal/modules/app/workflowtest ./middleware -count=1
```

Observed result:

- Agent runtime/surface targeted regressions pass, covering runnable webapp filtering, runtime surface read/update authorization, invalid grant-subject rejection, webapp runtime caller scoping, console agent runtime caller scoping, and workflow-continuation approval binding.
- Full `internal/modules/app/agents` now passes in the local default environment. `agents_prompt_variables_test.go` skips only the sqlite scan fixture when the sqlite driver reports it is unavailable without cgo, matching the existing `workflow/pause` test behavior.
- The broader permission aggregate now includes `internal/modules/app/agents` and passes with account context/capabilities, workspace handlers, datasource/database access, file access, dataset access, runtime authorization, workflow/workflow-test authorization, and middleware.
- SQLite-specific execution still needs a `CGO_ENABLED=1` environment with a working C compiler when the goal is to validate the database-backed fixture itself rather than the permission surface.

Latest workflow subtree validation, 2026-06-21 12:30 +08:00:

```powershell
go test ./internal/modules/app/workflow/approval -count=1
go test ./internal/modules/app/workflow/nodes/llm -count=1
go test ./internal/modules/app/workflow/nodes/parameter_extractor -count=1
go test ./internal/modules/app/workflow/... -count=1
go test ./internal/modules/user/auth/handler ./internal/modules/user/auth/service ./internal/modules/workspace/handler ./internal/modules/datasource/handler ./internal/modules/datasource/service ./internal/modules/datasource/service/excelimport ./internal/modules/file_process/handler ./internal/modules/file_process/repository ./internal/modules/file_process/service/extractor/hyperparse ./internal/modules/dataset/handler ./internal/modules/dataset/graphflow/openie ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow/... ./internal/modules/app/workflowtest ./middleware -count=1
```

Observed result:

- The workflow subtree now passes locally, including workflow root handler/runtime authorization tests, runtime history/log tests, workflow webapp conversation caller-scope tests, approval/pause packages, LLM node tests, and node-level packages.
- `approval/service_sms_test.go` and `nodes/llm/llm_test.go` now skip only sqlite-backed fixtures when the sqlite driver reports it is unavailable without cgo, matching the existing `workflow/pause` skip behavior. These database-backed fixture semantics still need `CGO_ENABLED=1` plus a working C compiler for full sqlite execution.
- `nodes/parameter_extractor/prompt_generator_test.go` now normalizes line endings inside the test assertion so CRLF template checkout on Windows does not break the expected prompt contract.
- The broader permission aggregate now includes `internal/modules/app/workflow/...`, strengthening coverage for published/internal runtime paths and workflow bare-ID regressions beyond the root workflow package.

Latest extended backend aggregate validation, 2026-06-21 12:34 +08:00:

```powershell
go test ./internal/modules/api_key ./internal/modules/prompts/... ./internal/modules/contentparse/... ./internal/modules/system/handler ./internal/modules/system/service ./internal/modules/shared/service ./internal/modules/shared/visibility ./internal/modules/shared/workspacebootstrap ./routes/external ./routes/v1 ./tests/routes ./internal/migrations -count=1
go test ./internal/modules/user/auth/handler ./internal/modules/user/auth/service ./internal/modules/workspace/handler ./internal/modules/datasource/handler ./internal/modules/datasource/service ./internal/modules/datasource/service/excelimport ./internal/modules/file_process/handler ./internal/modules/file_process/repository ./internal/modules/file_process/service/extractor/hyperparse ./internal/modules/dataset/handler ./internal/modules/dataset/graphflow/openie ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow/... ./internal/modules/app/workflowtest ./internal/modules/api_key ./internal/modules/prompts/... ./internal/modules/contentparse/... ./internal/modules/system/handler ./internal/modules/system/service ./internal/modules/shared/service ./internal/modules/shared/visibility ./internal/modules/shared/workspacebootstrap ./routes/external ./routes/v1 ./tests/routes ./internal/migrations ./middleware -count=1
```

Observed result:

- The extra backend permission package set passes for API key management, prompt builder workspace guards, content parse playground/shadow service paths, dashboard/system handler coverage, shared authorization service, shared workspace visibility, owner bootstrap, external workflow routes, v1 route compatibility tests, route-level tests, and migrations.
- The combined aggregate passes with the previous account context/capabilities, workspace, datasource, file, dataset, runtimeauth, agents, workflow subtree, workflow-test, and middleware coverage.
- This strengthens the local regression gate for API key external-call compatibility, published webapp/API route compatibility, migration coverage for published runtime authorization, and organization/workspace visibility helpers without adding new product behavior.

Latest frontend contract validation, 2026-06-21 12:36 +08:00:

```powershell
pnpm test:route-access
pnpm type-check
```

Observed result:

- `pnpm test:route-access` passes against the current worktree. The script still locks shared route access metadata, the derived `src/app/console/work` page tree classification, console/workspace layout capability guards, organization-mode switcher behavior, app-center no-workspace loading behavior, and runtime-access UI surface contracts.
- `pnpm type-check` passes, including customer adapter preparation, sensitive-word generation, i18n route module coverage, and `tsc --noEmit`.
- No frontend code changes were required in this validation pass.

Latest frontend route-tree validation, 2026-06-21 12:41 +08:00:

```powershell
pnpm test:route-access
pnpm type-check
```

Observed result:

- `web/scripts/test-route-access.mjs` now derives the full `src/app/console` page tree, not only `src/app/console/work`, and snapshots the expected organization-scoped versus workspace-scoped route sets.
- Organization-scoped console pages are limited to `/console/settings`, `/console/work`, `/console/work/chat`, `/console/work/image`, `/console/work/app`, and `/console/work/app/:web_app_id`.
- Workspace-scoped console pages now include the console root, agent builder/detail pages, dataset pages, database pages, files, prompts, content-parse developer page, `/console/work/task`, and workspace management pages. This makes newly added console asset/helper routes fail the route-access regression until they are intentionally classified.
- `pnpm test:route-access` and `pnpm type-check` pass with the expanded route-tree contract.

Latest runtimeauth resource-index foundation, 2026-06-21 12:51 +08:00:

```powershell
go test ./internal/modules/app/runtimeauth -run "TestPublishedRuntimeStore(ListAuthorizedResourceIDsEvaluatesAudienceGrants|RejectsUnsupportedSurfaceGrantSubjectsBeforeSQL|OverlaysPersistedSurfacesAndBuiltinAudience)" -count=1
go test ./internal/modules/app/runtimeauth -count=1
go test ./internal/modules/app/agents -run "Test(AgentRuntimeAuthorizationFromUpdateRequest_RejectsInvalidSurfaceGrantSubjects|AgentsService_UpdateAgentRuntimeSurfaces_RejectsInternalDisable|AgentsService_UpdateAgentRuntimeSurfaces_RequiresBuiltinGrantWhenEnabled|AgentsService_UpdateAgentRuntimeSurfaces_RejectsAccountGrantOutsideOrganization|AgentsService_UpdateAgentRuntimeSurfaces_RejectsDepartmentGrantOutsideOrganization)" -count=1
```

Observed result:

- `runtimeauth.Store.ListAuthorizedResourceIDs` now provides the first persisted runtime resource-index primitive for future resource-specific app/workflow lists. It lists enabled persisted surface rows by `resource_type`, `surface`, and `organization_id`, then evaluates each row with the existing `SurfaceAuthorization.Allows` audience semantics.
- The focused regression covers open persisted rows with no grants, organization grants, account grants, department grants, non-matching account grants, and disabled-grant-only rows. This prevents the batch resource-index path from accidentally treating disabled grants as open access.
- Existing webapp/API public-only and builtin/internal runtime-surface request contracts still pass through the agents package regression, so the new list primitive does not broaden current management API semantics.

Latest runtimeauth candidate-filter foundation, 2026-06-21 12:56 +08:00:

```powershell
go test ./internal/modules/app/runtimeauth -run "TestPublishedRuntimeStore(FilterAuthorizedResourceIDsAppliesFallbackAndPersistedOverlay|ListAuthorizedResourceIDsEvaluatesAudienceGrants|RejectsUnsupportedSurfaceGrantSubjectsBeforeSQL)" -count=1
go test ./internal/modules/app/runtimeauth -count=1
```

Observed result:

- `runtimeauth.Store.FilterAuthorizedResourceIDs` now accepts caller-owned candidate resources plus a fallback policy per candidate, loads persisted surface overlays in a single batch, and evaluates the same authorization semantics used by single-resource checks.
- The focused regression locks the difference between the two list primitives: persisted-only listing is useful for explicit grant indexes, while candidate filtering preserves no-row fallback compatibility and legacy agent `builtin_app=false` seed compatibility for active webapps.
- The regression covers an active legacy fallback with no persisted row, a persisted disabled override, a matching account grant, a non-matching account grant, a legacy seeded `builtin_app` row that remains visible for compatibility, and an inactive fallback with no row.
- Business list endpoints are not yet switched to this batch filter because department-scoped grants need a deliberate audience-loading strategy. The new method is the safe handoff point for the next app-center/built-in catalog integration step.

Latest app-center runtime authorization batching, 2026-06-21 13:00 +08:00:

```powershell
go test ./internal/modules/app/agents -run "TestAgentsService_GetRunnableWebApps_(FiltersExplicitBuiltinAccountGrant|AllowsExplicitBuiltinDepartmentGrant)" -count=1
go test ./internal/modules/app/agents ./internal/modules/app/runtimeauth -count=1
```

Observed result:

- `agentsService.GetRunnableWebApps` now connects the organization product app-center list to `runtimeauth.Store.FilterAuthorizedResourceIDs`. It still builds the candidate set from normal workspaces in the current organization, so organization members without a joined workspace keep the intended product-surface entry point.
- Runtime `builtin_app` authorization is now evaluated for the app-center candidate set in one batch instead of one `GetResourceAuthorization` query per agent. Each candidate carries `PolicyFromAgentFields(item.WebAppStatus, false)`, preserving no-row fallback compatibility and legacy active-webapp app-center visibility.
- App-center department audience is loaded once before candidate filtering, so explicit department grants are handled by the batch path without falling back to per-resource probing.
- Focused regressions now lock both an explicit account grant that hides another user's app and an explicit department grant that allows the member's app through the batch candidate-filter path.

Latest built-in catalog runtime authorization batching, 2026-06-21 13:06 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestBuiltInWorkflow" -count=1
go test ./routes/v1 -run "TestWorkflowRoutes_BuiltInWorkflow" -count=1
go test ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./routes/v1 -count=1
```

Observed result:

- `builtInWorkflowService.GetAllBuiltInWorkflows` now connects the built-in workflow catalog to `runtimeauth.Store.FilterAuthorizedResourceIDs`. Repository discovery still owns the catalog candidate set, and each built-in candidate uses `defaultBuiltInWorkflowPolicy()` so no-row fallback remains visible for organization product users.
- Built-in catalog department audience is loaded once before batch filtering. Route-level regressions now expect the list path to load department audience once, then evaluate persisted `builtin_app` rows through the batch candidate query.
- Built-in workflow detail routes intentionally keep single-resource authorization through `allowsBuiltInWorkflow`, preserving direct-detail denial semantics while the list path becomes batched.
- The combined `runtimeauth`, `agents`, `workflow`, and `routes/v1` package run passes, covering app-center batching, built-in catalog batching, direct built-in detail access, runtime-surface management, and published runtime store contracts together.

Latest account capabilities runtime resource-list contract, 2026-06-21 13:16 +08:00:

```powershell
go test ./internal/modules/user/auth/service -run "TestGetAccountCapabilities" -count=1
go test ./internal/modules/user/auth/handler -run TestAccountHandlerGetAccountCapabilitiesReturnsContract -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./routes/v1 -count=1
pnpm test:route-access
pnpm type-check
```

Observed result:

- `GET /console/api/account/capabilities` now includes `runtime_resource_lists` metadata alongside `runtime_surfaces` and `runtime_audience`. The first contract keys are `app_center` and `built_in_workflows`; each declares `resource_type`, `surface`, `mode=runtimeauth_candidate_filter`, and the dedicated list endpoint.
- The resource-list contract is enabled for confirmed organization members and remains present with `enabled=false` when the account has no current organization. This keeps no-workspace/no-organization clients from branching on missing fields.
- The field is intentionally descriptive rather than a direct authorized-resource-ID payload. App-center and built-in workflow catalog authorization stays owned by their dedicated endpoints and the `runtimeauth` candidate filter, avoiding a reverse dependency from user/auth capabilities into app modules.
- Frontend `AccountCapabilitiesResponse` types and `web/scripts/test-route-access.mjs` now lock the `runtime_resource_lists` response shape so the route/access contract can discover these dedicated list surfaces without inferring them from local route names.

Latest frontend runtime resource-list consumption, 2026-06-21 13:21 +08:00:

```powershell
pnpm test:route-access
pnpm type-check
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./routes/v1 -count=1
```

Observed result:

- `useAccountCapabilities` now exposes `runtimeResourceLists` and `canUseRuntimeResourceList`, giving feature hooks a typed way to consume the resource-list contract.
- `useRunnableWebApps` gates both `/console/api/agents/runnable-webapps` and per-app public config hydration on `runtime_resource_lists.app_center.enabled`. When the contract is disabled, the hook suppresses stale query items and old query errors instead of letting app-center UI infer access locally.
- `useBuiltInWorkflows` gates `/console/api/built-in-workflows` on `runtime_resource_lists.built_in_workflows.enabled`. It also suppresses the week-long local built-in workflow cache while the contract is disabled, so no-organization or denied states cannot show cached built-in catalog entries.
- `web/scripts/test-route-access.mjs` now locks both hook gates in addition to the response type shape. The frontend route/access test and type-check pass, and the related backend capabilities/runtimeauth/app/workflow route package run remains green.

Latest workflow run history conversation-id boundary cleanup, 2026-06-21 13:26 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "TestWorkflowRun(SystemInputConversationID|InputConversationIDKeepsLegacyTopLevelFallback|NodeSystemInputConversationID)" -count=1
go test ./internal/modules/app/workflow -run "Test(GetWorkflowRun|ValidateWorkflowRun|StopWorkflowTask|WorkflowRunSystemInputConversationID|WorkflowRunInputConversationIDKeepsLegacyTopLevelFallback|WorkflowRunNodeSystemInputConversationID)" -count=1
go test ./internal/modules/app/workflow ./routes/v1 -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./routes/v1 -count=1
```

Observed result:

- Workflow run list/detail fallback conversation association now reads only `sys.conversation_id` from stored run or node inputs when no linked runtime message row exists. A task workflow business input named `conversation_id` is no longer surfaced as a workflow-run conversation association.
- `workflowRunInputConversationID` keeps the legacy top-level `conversation_id` fallback for approval-resume compatibility, and the new focused regression locks that behavior separately from the system-only history helper.
- Node-log fallback behaves the same way: node inputs with only a business `conversation_id` are skipped, and the first later `sys.conversation_id` is used as the run history association.
- This narrows one documented non-conversation `conversation_id` ambiguity in the read/display layer without changing streaming/run request compatibility. The broader request input contract cleanup remains a follow-up because non-conversation workflows can still define business variables named `conversation_id`.

Latest question-answer pause conversation-id boundary cleanup, 2026-06-21 13:31 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "Test(WorkflowStreamPauseConversationID|QuestionAnswerStateConversationID)" -count=1
go test ./internal/modules/app/workflow ./routes/v1 -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./routes/v1 -count=1
```

Observed result:

- Question-answer pause message events now resolve conversation IDs only from the variable pool system variables or `sys.conversation_id` request input. Top-level business `conversation_id` no longer becomes the pause message event conversation association.
- Question-answer pause persistence uses the same system-only boundary: it stores a pause `conversation_id` only from the variable pool system variables or `sys.conversation_id`, not from a top-level business `conversation_id`.
- Focused regressions lock variable-pool priority, `sys.conversation_id` fallback, and business-field suppression for both pause message events and persisted question-answer pause state.
- Approval resume compatibility is unchanged: the legacy workflow-run input helper still keeps top-level fallback only for approval resume paths that need older stored run compatibility.

Latest workflow stream system-input conversation-id boundary cleanup, 2026-06-21 13:39 +08:00:

```powershell
go test ./internal/modules/app/workflow -run "Test(WorkflowStreamSystemConversationInput|WorkflowStreamPauseConversationID|QuestionAnswerStateConversationID|PromoteWorkflowInputConversationIDToSystemInput|ValidateWebAppConversationAccess|RunDraftWorkflow_RejectsForeignSystemConversationBeforeRunning|RunPublishedWorkflow_RejectsForeignSystemConversationBeforeRunning)" -count=1
go test ./internal/modules/app/workflow ./routes/v1 -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./routes/v1 -count=1
git diff --check -- api/internal/modules/app/workflow/workflow_stream_conversation.go api/internal/modules/app/workflow/workflow_webapp_conversation_access.go api/internal/modules/app/workflow/workflow_draft_run_handler.go api/internal/modules/app/workflow/workflow_published_run_handler.go api/internal/modules/app/workflow/workflow_stream_helpers_test.go api/internal/modules/app/workflow/workflow_webapp_conversation_access_test.go
```

Observed result:

- Workflow stream system-input preparation now treats only `sys.conversation_id` as an existing system conversation. A top-level business input named `conversation_id` no longer becomes `sys.conversation_id` inside the lower stream preparation layer.
- Generic draft and published conversation workflow HTTP streaming handlers preserve the legacy `inputs.conversation_id` contract by promoting it to `sys.conversation_id` only after the workflow type is known to be conversational and the caller/conversation ownership check has passed.
- Public webapp and advanced-chat run paths already copy the protocol-level request `conversation_id` into `sys.conversation_id` after caller-scope validation, so their continuation semantics are unchanged.
- Focused regressions lock both sides of the boundary: lower stream preparation ignores business `conversation_id`, while the handler-level compatibility promotion preserves validated legacy conversation workflow requests.

Latest datasource table metadata permission-order hardening, 2026-06-21 13:47 +08:00:

```powershell
go test ./internal/modules/datasource/handler -run "Test(GetTableRequiresDatabaseViewBeforeTableLookup|CreateTableRequiresDatabaseManageBeforeBindingRequest|UpdateTableRequiresDatabaseManageBeforeBindingRequest|ExcelImport.*Permission|AddTableRecordsRequiresDatabaseEditBeforeBindingRequest|ImportTableRecordsRequiresDatabaseEditBeforeBindingRequest)" -count=1
go test ./internal/modules/datasource/handler ./internal/modules/datasource/service -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./internal/modules/datasource/handler ./internal/modules/datasource/service ./routes/v1 -count=1
git diff --check -- api/internal/modules/datasource/handler/datasource_handler.go api/internal/modules/datasource/handler/excel_import_handler_permission_test.go
```

Observed result:

- `GET /console/api/data-dbs/:id/tables/:table_id` now resolves the route datasource workspace and requires `database.view` before calling the table metadata service.
- The new handler regression locks denial before the bare `table_id` lookup, so a caller without datasource view permission cannot distinguish table existence or datasource/table binding from the table detail route.
- This is intentionally an HTTP handler hardening rather than a service-level `GetTable` permission change because the built-in database tool has a separate published runtime binding-grant path that may call `GetTable` after agent binding authorization instead of user workspace permission.

Latest datasource table list/delete permission-order hardening, 2026-06-21 13:57 +08:00:

```powershell
go test ./internal/modules/datasource/handler -run "Test(ListTablesRequiresDatabaseViewBeforeTableListing|DeleteTableRequiresDatabaseManageBeforeTableMutation|GetTableRequiresDatabaseViewBeforeTableLookup|CreateTableRequiresDatabaseManageBeforeBindingRequest|UpdateTableRequiresDatabaseManageBeforeBindingRequest|ExcelImport.*Permission|AddTableRecordsRequiresDatabaseEditBeforeBindingRequest|ImportTableRecordsRequiresDatabaseEditBeforeBindingRequest)" -count=1
go test ./internal/modules/datasource/handler ./internal/modules/datasource/service -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./internal/modules/datasource/handler ./internal/modules/datasource/service ./routes/v1 -count=1
```

Observed result:

- `GET /console/api/data-dbs/:id/tables` now requires a non-empty account context, resolves the route datasource workspace through the shared datasource permission helper, and requires `database.view` before calling the table listing service.
- `DELETE /console/api/data-dbs/:id/tables/:table_id` now resolves the route datasource workspace through the shared datasource permission helper and requires `database.manage` before calling the table deletion service.
- The handler regressions lock denial before the bare table list/detail/delete service paths, so a caller without datasource permission cannot use guessed datasource/table IDs to list tables, trigger deletion, or distinguish mutation-path validation.
- This remains an HTTP handler hardening rather than a datasource service-level `ListTables`/`DeleteTable` permission change so published/runtime database tool paths can continue to rely on their separate binding-grant authorization boundary.

Latest datasource table columns/query permission-order hardening, 2026-06-21 14:03 +08:00:

```powershell
go test ./internal/modules/datasource/handler -run "Test(GetTableColumnsRequiresDatabaseViewBeforeColumnLookup|QueryTableRecordsRequiresDatabaseViewBeforeRecordLookup|ListTablesRequiresDatabaseViewBeforeTableListing|DeleteTableRequiresDatabaseManageBeforeTableMutation|GetTableRequiresDatabaseViewBeforeTableLookup|CreateTableRequiresDatabaseManageBeforeBindingRequest|UpdateTableRequiresDatabaseManageBeforeBindingRequest|ExcelImport.*Permission|AddTableRecordsRequiresDatabaseEditBeforeBindingRequest|ImportTableRecordsRequiresDatabaseEditBeforeBindingRequest)" -count=1
go test ./internal/modules/datasource/handler ./internal/modules/datasource/service -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./internal/modules/datasource/handler ./internal/modules/datasource/service ./routes/v1 -count=1
```

Observed result:

- `GET /console/api/data-dbs/:id/tables/:table_id/columns` now requires a non-empty account context, resolves the route datasource workspace through the shared datasource permission helper, and requires `database.view` before calling the table column service.
- `GET /console/api/data-dbs/:id/tables/:table_id/records` now uses the same `database.view` preflight before calling the table record query service, keeping record-query feedback behind the datasource workspace boundary.
- Focused regressions lock both denial orders, so a caller without datasource view permission cannot use guessed datasource/table IDs to reach column or record lookup paths before workspace permission denial.
- Datasource handler direct `CheckWorkspacePermission` calls are cleared; datasource-id and workspace-id preflights now go through shared helpers. Creation pre-reads only `workspace_id` before target-workspace permission and full-binds the create DTO only after `database.manage` passes, so denied callers cannot get missing-name or other create request-shape feedback first.

Latest datasource delete permission-order hardening, 2026-06-21 14:06 +08:00:

```powershell
go test ./internal/modules/datasource/handler -run "Test(DeleteDataSourceRequiresDatabaseManageBeforeMutation|GetTableColumnsRequiresDatabaseViewBeforeColumnLookup|QueryTableRecordsRequiresDatabaseViewBeforeRecordLookup|ListTablesRequiresDatabaseViewBeforeTableListing|DeleteTableRequiresDatabaseManageBeforeTableMutation|GetTableRequiresDatabaseViewBeforeTableLookup|CreateTableRequiresDatabaseManageBeforeBindingRequest|UpdateTableRequiresDatabaseManageBeforeBindingRequest|ExcelImport.*Permission|AddTableRecordsRequiresDatabaseEditBeforeBindingRequest|ImportTableRecordsRequiresDatabaseEditBeforeBindingRequest)" -count=1
go test ./internal/modules/datasource/handler ./internal/modules/datasource/service -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./internal/modules/datasource/handler ./internal/modules/datasource/service ./routes/v1 -count=1
```

Observed result:

- `DELETE /console/api/data-dbs/:id` now resolves the route datasource workspace through the shared datasource permission helper and requires `database.manage` before calling the datasource deletion service.
- The handler regression locks denial before the datasource mutation service, so a caller without datasource management permission cannot use guessed datasource IDs to trigger deletion or reach mutation-path behavior before workspace permission denial.
- Remaining datasource handler handwritten permission blocks are narrowed to datasource creation and SQL-audit workspace/detail paths at this point; datasource delete no longer has the legacy workspace fallback.

Latest datasource workspace-permission helper cleanup, 2026-06-21 14:12 +08:00:

```powershell
go test ./internal/modules/datasource/handler -run "Test(CreateDataSourceRequiresDatabaseManageBeforeServiceMutation|SQLAuditRoutesRequireDatabaseManageBeforeServiceLookup|DeleteDataSourceRequiresDatabaseManageBeforeMutation|GetTableColumnsRequiresDatabaseViewBeforeColumnLookup|QueryTableRecordsRequiresDatabaseViewBeforeRecordLookup|ListTablesRequiresDatabaseViewBeforeTableListing|DeleteTableRequiresDatabaseManageBeforeTableMutation|GetTableRequiresDatabaseViewBeforeTableLookup|CreateTableRequiresDatabaseManageBeforeBindingRequest|UpdateTableRequiresDatabaseManageBeforeBindingRequest|ExcelImport.*Permission|AddTableRecordsRequiresDatabaseEditBeforeBindingRequest|ImportTableRecordsRequiresDatabaseEditBeforeBindingRequest)" -count=1
go test ./internal/modules/datasource/handler ./internal/modules/datasource/service -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./internal/modules/datasource/handler ./internal/modules/datasource/service ./routes/v1 -count=1
```

Observed result:

- Added a shared workspace-level database permission helper for routes that already carry `workspace_id`, using the same fail-closed organization/workspace/account permission path as datasource-id preflights.
- `POST /console/api/data-dbs` now uses that helper for target-workspace `database.manage` before calling the datasource creation service; the focused regression locks denial before creation mutation.
- `GET /console/api/workspaces/:workspace_id/sql-audit` and `GET /console/api/workspaces/:workspace_id/sql-audit/:operation_id` now use the same helper before query binding/list/count/detail service work; focused regressions lock denied list/detail routes before service lookup.
- `api/internal/modules/datasource/handler/datasource_handler.go` no longer has direct `CheckWorkspacePermission` or `CheckWorkspaceOrganizationAnyPermission` calls; permission checks are centralized in datasource-id and workspace-id helpers. Follow-up 2026-06-21 14:18 +08:00 removes the body-carried create-denial gap by pre-reading only `workspace_id` before full create DTO binding.

Latest datasource create workspace pre-read hardening, 2026-06-21 14:18 +08:00:

```powershell
go test ./internal/modules/datasource/handler -run "Test(CreateDataSourceRequiresDatabaseManageBefore(ServiceMutation|NameValidation)|SQLAuditRoutesRequireDatabaseManageBeforeServiceLookup|DeleteDataSourceRequiresDatabaseManageBeforeMutation)" -count=1
go test ./internal/modules/datasource/handler -run "Test(CreateDataSourceRequiresDatabaseManageBefore(ServiceMutation|NameValidation)|SQLAuditRoutesRequireDatabaseManageBeforeServiceLookup|DeleteDataSourceRequiresDatabaseManageBeforeMutation|GetTableColumnsRequiresDatabaseViewBeforeColumnLookup|QueryTableRecordsRequiresDatabaseViewBeforeRecordLookup|ListTablesRequiresDatabaseViewBeforeTableListing|DeleteTableRequiresDatabaseManageBeforeTableMutation|GetTableRequiresDatabaseViewBeforeTableLookup|CreateTableRequiresDatabaseManageBeforeBindingRequest|UpdateTableRequiresDatabaseManageBeforeBindingRequest|ExcelImport.*Permission|AddTableRecordsRequiresDatabaseEditBeforeBindingRequest|ImportTableRecordsRequiresDatabaseEditBeforeBindingRequest)" -count=1
go test ./internal/modules/datasource/handler ./internal/modules/datasource/service -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./internal/modules/datasource/handler ./internal/modules/datasource/service ./routes/v1 -count=1
```

Observed result:

- `POST /console/api/data-dbs` now first decodes only `workspace_id` from the cached JSON body, requires target-workspace `database.manage`, then full-binds `dto.CreateDataSourceRequest` and calls the creation service.
- A focused regression sends `{"workspace_id":"workspace-1"}` without `name` and locks permission denial ahead of missing-name validation when the caller lacks `database.manage`.
- Malformed JSON and missing `workspace_id` still fail as request-shape errors because the permission boundary cannot be resolved without a parseable target workspace.

Latest datasource template-download permission-order hardening, 2026-06-21 14:22 +08:00:

```powershell
go test ./internal/modules/datasource/handler -run "Test(DownloadTableTemplateRequiresDatabaseViewBeforeTemplateLookup|CreateDataSourceRequiresDatabaseManageBefore(ServiceMutation|NameValidation)|SQLAuditRoutesRequireDatabaseManageBeforeServiceLookup|DeleteDataSourceRequiresDatabaseManageBeforeMutation|GetTableColumnsRequiresDatabaseViewBeforeColumnLookup|QueryTableRecordsRequiresDatabaseViewBeforeRecordLookup|ListTablesRequiresDatabaseViewBeforeTableListing|DeleteTableRequiresDatabaseManageBeforeTableMutation|GetTableRequiresDatabaseViewBeforeTableLookup|CreateTableRequiresDatabaseManageBeforeBindingRequest|UpdateTableRequiresDatabaseManageBeforeBindingRequest|ExcelImport.*Permission|AddTableRecordsRequiresDatabaseEditBeforeBindingRequest|ImportTableRecordsRequiresDatabaseEditBeforeBindingRequest)" -count=1
go test ./internal/modules/datasource/handler ./internal/modules/datasource/service -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./internal/modules/datasource/handler ./internal/modules/datasource/service ./routes/v1 -count=1
```

Observed result:

- `GET /console/api/data-dbs/:id/tables/:table_id/template` now resolves the route datasource workspace and requires `database.view` before calling table lookup or template generation service methods.
- The focused handler regression locks denial before both `GetTable` and `GenerateTableTemplateExcel`, so a caller without datasource view permission cannot use guessed datasource/table IDs to discover table names or trigger template generation first.

Latest datasource analyze-file permission-order hardening, 2026-06-21 14:27 +08:00:

```powershell
go test ./internal/modules/datasource/handler -run "Test(AnalyzeFileForTableRequiresDatabaseManageBeforeModelValidation|DownloadTableTemplateRequiresDatabaseViewBeforeTemplateLookup|CreateDataSourceRequiresDatabaseManageBefore(ServiceMutation|NameValidation)|SQLAuditRoutesRequireDatabaseManageBeforeServiceLookup|DeleteDataSourceRequiresDatabaseManageBeforeMutation|GetTableColumnsRequiresDatabaseViewBeforeColumnLookup|QueryTableRecordsRequiresDatabaseViewBeforeRecordLookup|ListTablesRequiresDatabaseViewBeforeTableListing|DeleteTableRequiresDatabaseManageBeforeTableMutation|GetTableRequiresDatabaseViewBeforeTableLookup|CreateTableRequiresDatabaseManageBeforeBindingRequest|UpdateTableRequiresDatabaseManageBeforeBindingRequest|ExcelImport.*Permission|AddTableRecordsRequiresDatabaseEditBeforeBindingRequest|ImportTableRecordsRequiresDatabaseEditBeforeBindingRequest)" -count=1
go test ./internal/modules/datasource/handler ./internal/modules/datasource/service -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./internal/modules/datasource/handler ./internal/modules/datasource/service ./routes/v1 -count=1
```

Observed result:

- `POST /console/api/data-dbs/analyze-file-for-table` now pre-reads only `data_source_id`, requires route datasource `database.manage`, then full-binds `dto.AnalyzeFileForTableRequest` and calls the analysis service.
- A focused regression sends an invalid nested `model:{}` and locks permission denial ahead of full model provider/name validation when the caller lacks datasource management permission.

Latest datasource table-file ingest permission-order hardening, 2026-06-21 15:12 +08:00:

```powershell
go test ./internal/modules/datasource/handler -run "Test(DatabaseIngestionHelpersRequireDatabaseEditBeforeBodyValidation|AnalyzeFileForTableRequiresDatabaseManageBeforeModelValidation|DownloadTableTemplateRequiresDatabaseViewBeforeTemplateLookup)" -count=1
go test ./internal/modules/datasource/handler -run "Test(DatabaseIngestionHelpersRequireDatabaseEditBeforeBodyValidation|AnalyzeFileForTableRequiresDatabaseManageBeforeModelValidation|DownloadTableTemplateRequiresDatabaseViewBeforeTemplateLookup|CreateDataSourceRequiresDatabaseManageBefore(ServiceMutation|NameValidation)|SQLAuditRoutesRequireDatabaseManageBeforeServiceLookup|DeleteDataSourceRequiresDatabaseManageBeforeMutation|GetTableColumnsRequiresDatabaseViewBeforeColumnLookup|QueryTableRecordsRequiresDatabaseViewBeforeRecordLookup|ListTablesRequiresDatabaseViewBeforeTableListing|DeleteTableRequiresDatabaseManageBeforeTableMutation|GetTableRequiresDatabaseViewBeforeTableLookup|CreateTableRequiresDatabaseManageBeforeBindingRequest|UpdateTableRequiresDatabaseManageBeforeBindingRequest|ExcelImport.*Permission|AddTableRecordsRequiresDatabaseEditBeforeBindingRequest|ImportTableRecordsRequiresDatabaseEditBeforeBindingRequest)" -count=1
go test ./internal/modules/datasource/handler ./internal/modules/datasource/service -count=1
go test ./internal/modules/user/auth/service ./internal/modules/user/auth/handler ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow ./internal/modules/datasource/handler ./internal/modules/datasource/service ./routes/v1 -count=1
```

Observed result:

- `POST /console/api/data-dbs/parse-file-for-table-ingest`, `/extract-text-to-table-records`, `/ingest-file-to-table`, and `/batch-ingest-file-to-table` now pre-read `table_id`, resolve the owning datasource/workspace, require `database.manage|database.data_edit`, and only then bind the full request body or call ingestion services.
- A focused regression sends missing `file_id`/`file_ids` or invalid nested `model:{}` payloads and locks permission denial ahead of request-shape feedback and downstream parse/extract/ingest service calls.
- Current route shape still forces a minimal table-to-datasource resolution before the workspace permission can be selected because these legacy endpoints only carry `table_id`; a later API cleanup can carry datasource scope explicitly to remove that lookup.

Latest Docker validation, 2026-06-21 12:39 +08:00:

```powershell
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api web
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- The zgi-c API and web images rebuilt successfully from the current worktree. The web production build completed and emitted the expected app route tree, including `/console/work/chat`, `/console/work/image`, `/console/work/app`, `/console/work/app/[web_app_id]`, `/console/work/task`, workspace asset routes, dashboard organization routes, and published `/webapp/[version_uuid]` routes.
- `api`, `web`, and `postgres` are healthy after restart.
- API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Current completion audit, 2026-06-21 12:43 +08:00:

| Goal requirement | Current authoritative evidence | Audit status |
| --- | --- | --- |
| Account context mode API supports workspace/organization/no-workspace states | `account_service_context_test.go` and `account_handler_context_test.go` cover organization mode clearing workspace, workspace mode requiring workspace, context mode responses, no-workspace organization mode, and capabilities JSON contract. zgi-c API/browser no-workspace checks remain documented above. | Strong MVP evidence |
| Frontend has explicit organization mode/no-workspace entry and unified route guard | `web/scripts/test-route-access.mjs` now derives full `/console` and `/console/work` page trees, locks organization-scoped product/settings routes, keeps asset/builder/helper routes workspace-scoped, and statically checks console shell/workspace layout/work layout/team switcher/capability hook contracts plus runtime resource-list hook gates. `pnpm test:route-access` and `pnpm type-check` pass. | Strong MVP evidence |
| Organization members without workspace can use chat/image/app/settings | zgi-c no-workspace browser/API validation covers product routes, app center runnable apps, built-in workflows, and settings. Static route tests ensure product pages do not consume workspace permissions or gate product data on current workspace. | Strong MVP evidence |
| Workspace assets remain blocked without workspace and recover after switching back | Browser/API validation covers no-workspace blocking for agents/dataset/files and temporary workspace recovery. Static route tests now classify agents, dataset, db, files, prompts, content-parse, `/console/work/task`, workspace management, and `/console` root as workspace-scoped. | Strong MVP evidence |
| Published webapp/API/builtin/internal runtime storage and compatibility | Migration `20260620090000_create_published_runtime_authorization`, `runtimeauth` tests, agents runtime-surface tests, built-in workflow runtime-surface route tests, route v1 webapp/API compatibility tests, API key middleware/tests, and Docker migration checks cover MVP public webapp/API, builtin audience grants, and internal grant semantics. | Strong MVP evidence for current public-compatible semantics |
| User/department grant foundation for published runtime | Backend validates account and department grants against the current organization for builtin surfaces; frontend runtime grant rows support account/department pickers, hydration failures, stale grants, and incomplete rows. `runtimeauth.Store.ListAuthorizedResourceIDs` adds a persisted-resource index primitive, `FilterAuthorizedResourceIDs` now powers both app-center and built-in catalog candidate filtering while preserving fallback compatibility, account capabilities declare the dedicated runtime resource-list endpoints, and frontend app-center/built-in hooks consume those gates before querying or showing cached catalog data. | Foundation covered; bulk/non-technical UX and direct authorized-resource-ID payloads remain follow-up |
| Critical bare-ID asset paths reject unauthorized reads/writes | Expanded backend aggregate covers datasource, file, dataset, agent, workflow-test, workflow history/runtime/logs, API key management, prompt builder, content-parse, dashboard/system, shared visibility, routes, and migrations. Workflow run history fallback and question-answer pause/event association now ignore business `conversation_id` inputs and only use system conversation state when no linked runtime message exists. Endpoint inventory records the active high-risk groups as covered. | Strong MVP evidence for inventoried high-risk paths |
| Local validation and Docker migration/health evidence | Latest backend aggregate, frontend route/type validation, and Docker api+web rebuild/health/migration checks all pass against the current worktree. | Strong current validation |

Completion decision:

- The MVP foundation is now strongly evidenced for the requested boundary model and backtest paths, but the broader long-term goal should remain open because some requested future-facing behavior is intentionally constrained by product decisions rather than fully implemented.
- The current implementation deliberately keeps webapp/API audience grants public-only. Moving to user/department-targeted webapp/API access still requires the recorded product decision on public config/auth handshake, webapp migration ownership proof, and API key caller/owner semantics. The post-MVP decision brief is tracked in `docs/superpowers/plans/2026-06-21-runtime-authorization-next-phase.md`.
- The account capabilities contract exposes supported runtime subjects, current runtime audience, and dedicated runtime resource-list metadata for app-center and built-in workflow catalog endpoints. Frontend app-center and built-in workflow hooks now consume those gates before loading or returning cached list data, but the contract still does not inline a resource-specific authorized app/workflow ID list. `runtimeauth` now has both a persisted resource-ID listing primitive for explicit surface rows and a candidate + fallback batch filter. The app-center and built-in catalog endpoints use the compatibility-preserving batch filter; management UIs still rely on dedicated runtimeauth APIs.
- Remaining engineering follow-up is not a blocker for the MVP foundation, but should stay visible: bulk runtime-grant UX, broader migration of legacy handlers onto shared authorizers, and completing the non-conversation `conversation_id` request input cleanup beyond the history display boundary.

MVP freeze pass, 2026-06-21 16:49 +08:00:

```powershell
go test ./internal/modules/user/auth/handler ./internal/modules/user/auth/service ./internal/modules/workspace/handler ./internal/modules/datasource/handler ./internal/modules/datasource/service ./internal/modules/datasource/service/excelimport ./internal/modules/file_process/handler ./internal/modules/file_process/repository ./internal/modules/file_process/service/extractor/hyperparse ./internal/modules/dataset/handler ./internal/modules/dataset/graphflow/openie ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow/... ./internal/modules/app/workflowtest ./internal/modules/api_key ./internal/modules/prompts/... ./internal/modules/contentparse/... ./internal/modules/system/handler ./internal/modules/system/service ./internal/modules/shared/service ./internal/modules/shared/visibility ./internal/modules/shared/workspacebootstrap ./routes/external ./routes/v1 ./tests/routes ./internal/migrations ./middleware -count=1
pnpm test:route-access
pnpm type-check
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api web
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 5;"
```

Observed result:

- Backend permission aggregate passes against the current worktree. The first 120s run timed out without failure output; the same command passes with a longer timeout.
- `pnpm test:route-access` passes, keeping organization-scoped product/settings routes and workspace-scoped asset/helper routes classified.
- `pnpm type-check` passes after customer adapter preparation, sensitive-word generation, i18n route-module checks, and `tsc --noEmit`.
- zgi-c Docker API and web images rebuild from the current worktree. The production route tree includes `/console/work/chat`, `/console/work/image`, `/console/work/app`, `/console/work/app/[web_app_id]`, `/console/settings`, workspace asset routes, dashboard organization routes, and published `/webapp/[version_uuid]` routes.
- `zgi-c-api-1`, `zgi-c-web-1`, and `zgi-c-postgres-1` are healthy. API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration in local Docker remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index, chat-runtime skill-variable, default dataset permission, and short-link expiry migrations.

First backtest checklist:

- No-workspace member: remove all `workspace_members` rows for the test account while keeping active organization membership, refresh/login, and confirm `GET /account/context` reports organization mode with no current workspace.
- Organization product surfaces: visit `/console/work/chat`, `/console/work/image`, `/console/work/app`, `/console/work/app/[web_app_id]`, and `/console/settings`; product data should load without requiring a workspace.
- Workspace asset isolation: visit `/console/agents`, `/console/dataset`, `/console/db`, `/console/files`, `/console/prompts`, `/console/developer/content-parse`, `/console/work/task`, `/console/workspace`, and `/console`; each should show a workspace-required or no-access state instead of leaking asset lists.
- Switch-back recovery: add the same account to a workspace, switch to workspace mode through the UI or `PUT /account/context`, then confirm asset pages and `/account/capabilities` recover workspace scope.
- Published runtime compatibility: verify existing public webapp run, external API key call, built-in app/workflow list, and internal workflow/task invocation still follow their compatibility semantics after the `published_runtime_authorization` migration.
- Bare-ID negative probes: with an account lacking the relevant workspace permission, try known/guessed agent, dataset/document/segment, datasource/table/record/import, file/folder, prompt, workflow-test, API-key, workflow-run/history/log IDs; responses should deny or hide before request-shape, existence, mutation, or service-specific feedback.

Latest Docker validation, 2026-06-21 12:19 +08:00:

```powershell
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml up -d --build api
docker compose --env-file docker\.env -f docker\docker-compose.yaml -f docker\compose.zgi-c.local.yaml ps api web postgres
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2870/ping -TimeoutSec 30 | Select-Object -ExpandProperty Content
Invoke-WebRequest -UseBasicParsing -Uri http://127.0.0.1:2880/api/health -TimeoutSec 30 | Select-Object -ExpandProperty Content
docker exec zgi-c-postgres-1 psql -U postgres -d zgi -c "SELECT * FROM migrations ORDER BY id DESC LIMIT 3;"
```

Observed result:

- The zgi-c API image rebuilt successfully from the current worktree and restarted cleanly. `api`, `web`, and `postgres` are healthy.
- API `/ping` returns `{"message":"pong"}` and web `/api/health` returns `{"status":"ok", ...}`.
- The latest migration remains `20260620090000_create_published_runtime_authorization`, followed by the announcement index and chat-runtime skill-variable migrations.

Remaining risks for the next iteration:

- Agent and built-in published runtime management now have minimal frontend entries, and account/department targeting has picker/search flows plus saved-account label hydration in the main management paths. Empty rows, lookup failures, and stale account/department grants are surfaced inline with the retained raw ID where applicable. Remaining UX debt is bulk selection before broad non-technical rollout.
- Webapp config, external API key validation, and built-in workflow catalog consume persisted surface state. The current frontend supports public webapp/API toggles and builtin organization/account/department grants. Webapp/API audience restriction is intentionally not exposed yet because execution semantics need a product decision. The endpoint inventory now records the required decision table: keep `webapp` public-only, add optional-auth public config, or add a protected/gated config flow; keep API keys as public bearer capabilities, bind keys to an owner audience, or require a caller/user claim per request.
- `POST /console/api/workflows/migrate-user` is now covered as a legacy compatibility contract, but it is still not resource-specific because the route has no `web_app_id`. If future webapp audience restrictions become non-public, anonymous-to-authenticated migration must follow one of the recorded decision-table paths: keep the global virtual-account migration endpoint, introduce a `/:web_app_id/migrate-user` flow with surface checks, or add a signed ownership proof for virtual IDs.
- The account capabilities response now exposes supported runtime grant subjects, the current account runtime audience including active departments, and `runtime_resource_lists` metadata for the app-center and built-in workflow catalog endpoints. The frontend app-center and built-in workflow hooks consume those gates and suppress stale cached list data when the contract is disabled. The contract still does not inline a resource-specific authorized app/workflow ID list; `runtimeauth.Store.ListAuthorizedResourceIDs` and `FilterAuthorizedResourceIDs` are available as the lowest-level persisted index and compatibility-preserving candidate filter for that future contract. App-center and built-in catalog endpoints already use the candidate filter with one-time department audience loading, while management UI continues to rely on dedicated runtimeauth APIs.
- Published workflow webapp conversation detail/delete, webapp run/continue, advanced-chat run, generic draft/published conversation workflow run paths, async webapp conversation title generation, parent-message metadata, dialogue-count metadata, legacy builder agent chat-message reads, AGENT runtime conversation detail/update/delete/message-list/stop/history/run detail/step routes, legacy AGENT runtime-log listing, legacy workflow node logs, and workflow-run event streams are now covered by agent/caller/workspace scoped checks or denial-order regressions. AGENT runtime webapp conversation message list/update/delete/stop, event streaming, regeneration, continuation paths, approval-token-to-run binding, and the workflow-continuation stop hook have focused handler/contract regressions plus service-level caller-scope regressions in `chatruntime`; legacy workflow webapp conversation detail/delete, async title generation, and metadata helpers now also have denial-order regressions. The follow-up active-route inventory found no additional registered older workflow history/log endpoint outside `ConversationQueryHandler`, `AgentHistoryDispatchHandler`, `AgentRuntimeLogsHandler`, `RuntimeLogHandler`, `WorkflowHandler` run query/control/event handlers, or runtime caller-scope service methods. `WorkflowStatisticHandler` remains unregistered with a route-table regression; its direct handler path also requires route-agent `agent.view`, so future statistics route restoration must deliberately update the route test and wire `WithWorkflowStatisticAuthorization`.
- Non-conversation workflows can still have business input variables named `conversation_id`. Workflow run history fallback, question-answer pause/event association, and lower workflow stream system-input preparation now ignore business top-level `conversation_id` values and use only system conversation state when they need to infer or continue a conversation association. Generic draft/published conversation workflow streaming handlers preserve the legacy `inputs.conversation_id` request contract only after workflow type and caller/conversation ownership are validated, by promoting it to `sys.conversation_id` at the handler boundary. The legacy workflow-run input helper still keeps top-level fallback for approval-resume compatibility, and a broader input contract cleanup should still be handled separately.
- The current organization-mode switcher path now has type, route-access, Docker health, zgi-c API-level no-workspace account coverage, and local headless Chrome coverage for product routes, workspace-required asset routes, the switcher popover, mobile app/asset smoke, and temporary switch-back-to-workspace recovery. Remaining browser debt is cross-browser/manual visual polish for the switcher popover rather than functional coverage.

Suggested validation commands:

```powershell
go test ./internal/modules/user/auth/handler ./internal/modules/user/auth/service ./internal/modules/workspace/handler ./internal/modules/datasource/handler ./internal/modules/datasource/service ./internal/modules/datasource/service/excelimport ./internal/modules/file_process/handler ./internal/modules/file_process/repository ./internal/modules/file_process/service/extractor/hyperparse ./internal/modules/dataset/handler ./internal/modules/dataset/graphflow/openie ./internal/modules/app/runtimeauth ./internal/modules/app/agents ./internal/modules/app/workflow/... ./internal/modules/app/workflowtest ./internal/modules/api_key ./internal/modules/prompts/... ./internal/modules/contentparse/... ./internal/modules/system/handler ./internal/modules/system/service ./internal/modules/shared/service ./internal/modules/shared/visibility ./internal/modules/shared/workspacebootstrap ./routes/external ./routes/v1 ./tests/routes ./internal/migrations ./middleware -count=1
```

For sqlite-specific fixture execution, run the relevant targeted tests in an environment with `CGO_ENABLED=1` and a working C compiler. Do not use module root directories with no Go files as part of the aggregate command.

```powershell
pnpm test:route-access
pnpm type-check
```

## Operating Goal

Complete the permission-system foundation MVP, verified by a checked permission inventory, targeted backend/frontend regression tests, and scoped authorization helpers that prove organization-only members can use product surfaces while workspace assets remain isolated. Preserve existing published webapp, API key, and internal invocation compatibility. Between iterations, start with the narrowest failing authorization or context test, patch the lowest shared layer that explains it, then broaden validation only after the targeted evidence passes. If blocked, record the failing path, evidence gathered, and the specific product or compatibility decision needed.
