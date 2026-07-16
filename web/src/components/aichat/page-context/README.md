# AIChat Page Context

This module is the stable frontend contract for pages that want AIChat to
understand the current ZGI surface.

It is intentionally a thin facade over the existing contextual AIChat runtime.
That keeps the Files page and Agent pages working while new pages migrate to a
clearer architecture name.

## Responsibilities

- Register bounded page resources with `usePageContextRegistration`.
- Expose a unified item shape through `PageContextItem` / `AIChatPageContextItem`.
- Build the existing AIChat request payloads with `buildPageContextEnvelope` and
  `buildPageOperationContext`.
- Honor registration options: `visibility: "selected"` / `"current"` places
  active user context ahead of ordinary visible resources, `priority` orders
  groups within the same visibility tier, `visibility: "background"` keeps a
  registration out of the AIChat item list, and
  `replace: false` appends an independent registration under the same logical
  page scope.
- Declare optional frontend-only `hints.refreshHints` and
  `hints.handledAssetTypes` so the AIChat dock can refresh the current page
  after confirmed asset operations without falling back to unsafe generic
  invalidation.
- Declare optional frontend-only `hints.presentation` and
  `hints.toolGovernance` so page adapters can own their AIChat home copy,
  suggestions, placeholders, and approval UI enablement without the dock
  hard-coding business routes.
- Keep the backend-compatible `operation_context` v1 shape intact.

## Compatibility Rules

Do not rename the request field from `operation_context`.

Keep the top-level operation context fields:

- `schema: "zgi.aichat.operation_context.v1"`
- `version: 1`
- `resources`
- `capabilities`
- `tool_governance.permission_tier`

Resource items must continue to provide stable identifiers through
`resource_id` or `id`, and resource type through `resource_type`, `type`, or
`kind`. File resources must preserve filename, selected state, visible order,
extension, file type, and workspace metadata because the backend resolver and
tool governance use those fields.

`hints.refreshHints`, `hints.handledAssetTypes`, `hints.presentation`, and
`hints.toolGovernance` are not authorization, governance decisions, or model
instructions. They are consumed only by the frontend. Pages may use refresh
hints to map asset effects such as `file:create` to React Query keys that
should be invalidated. Pages should use handled asset types when they need to
own an asset family and intentionally suppress generic fallback refreshes, such
as an Agent page with unsaved edits or a read-only Workflow editor. Pages may
use presentation hints to provide localized AIChat home copy and suggestions,
and tool governance hints to show the approval controls only when the current
page has governed asset operations.

## Migration Policy

New page adapters should import from `@/components/aichat/page-context`.
Existing `@/components/aichat/contextual` imports remain supported while older
code migrates.

The first expansion target is a read-only Workflow editor context. It should
register a bounded page summary, selected node, validation state, and redacted
runtime/log hints without exposing secrets or promising graph edits.
