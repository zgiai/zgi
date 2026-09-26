# Agent App Center categories

Agents and workflows can be assigned organization-scoped categories from the independent
**Manage categories** action on each management card. Category editing is intentionally separate
from basic information and publication settings. Categories are shown on management cards and
support filtering in the Agent list, Workflow list, and App Center, but never grant access to an
app or replace its runtime-surface audience rules.

Category changes take effect immediately and do not require the agent or workflow to be
republished. Each app can have at most eight categories, and each category name can contain at
most 32 characters. Names are trimmed, repeated whitespace is collapsed, and matching is
case-insensitive within an organization.

## Console API

- `GET /console/api/agents` includes a `tags` array on every list item so management cards can
  render categories without issuing one request per agent.
- `GET /console/api/agents/catalog-tags?workspace_id=<id>&asset_kind=<agent|workflow>` returns
  category facets and visible-item counts for management-list filters.
- `GET /console/api/agents?tag_ids=<id,id>` filters the permission-scoped management list.
  Multiple IDs use any-match semantics.
- `GET /console/api/agents/:agent_id/tags` returns the categories assigned to an agent.
- `PUT /console/api/agents/:agent_id/tags` replaces the complete category set. Each item may
  reference an existing category with `id` or create/reuse one with `name`.
- `GET /console/api/agents/runnable-webapps/tags` returns category facets and visible-app counts
  for the current account.
- `GET /console/api/agents/runnable-webapps?tag_ids=<id,id>` filters the already-authorized
  runnable catalog. Multiple IDs use any-match semantics.

Reading or changing an agent's categories requires its normal view or update permission.
Management facets always apply workspace visibility and resource-type permissions. Runnable
catalog queries apply publication state and App Center audience authorization before returning
apps or category counts.
