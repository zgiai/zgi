import assert from 'node:assert/strict';
import {
  ZGI_CONSOLE_SITE_MAP,
  getAccessibleZGIConsoleSiteMap,
  getZGIConsoleNavigationAccess,
  normalizeZGIConsoleNavigationHref,
} from '../src/routes/console-navigation.ts';

const siteMapRoutes = new Set(ZGI_CONSOLE_SITE_MAP.map(route => route.href));

for (const href of ['/console/workflows', '/console/skills']) {
  assert.equal(siteMapRoutes.has(href), true, `site map must include ${href}`);
  assert.equal(normalizeZGIConsoleNavigationHref(href), href, `${href} must be navigable`);
}

for (const href of [
  '/console/workflows/workflow-1',
  '/console/workflows/workflow-1/logs',
  '/console/workflows/workflow-1/api/keys',
  '/console/workflows/workflow-1/api/docs',
  '/console/workflows/workflow-1/batch-test',
]) {
  assert.equal(normalizeZGIConsoleNavigationHref(href), href, `${href} must be navigable`);
}

for (const href of [
  '/console/agents/agent-1/api',
  '/console/workflows/workflow-1/api',
]) {
  assert.equal(
    normalizeZGIConsoleNavigationHref(href),
    `${href}/keys`,
    `${href} must resolve to the API keys page`
  );
}

for (const href of [
  '/console/settings',
  '/console/db/database-1/table',
  '/console/files/file-1',
  '/console/workflows/workflow-1/api/unknown',
]) {
  assert.equal(normalizeZGIConsoleNavigationHref(href), null, `${href} must remain blocked`);
}

const readyContext = {
  workspaceStatus: 'ready',
  permissionsSettled: true,
  organizationRole: 'normal',
  workspaceRole: 'member',
  permissions: [],
};

assert.equal(
  getZGIConsoleNavigationAccess('/console/skills', {
    ...readyContext,
    workspaceStatus: 'workspace_required',
  }).status,
  'allowed',
  'organization routes must remain available without a workspace'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/workflows', {
    ...readyContext,
    workspaceStatus: 'workspace_required',
  }).status,
  'workspace_required'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/workflows', {
    ...readyContext,
    permissionsSettled: false,
  }).status,
  'permissions_loading'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/workflows', readyContext).status,
  'permission_denied'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/files', readyContext).status,
  'allowed',
  'workspace-scoped routes without feature permissions still require a ready workspace'
);

const workflowViewer = { ...readyContext, permissions: ['workflow.view'] };
assert.equal(getZGIConsoleNavigationAccess('/console/workflows', workflowViewer).status, 'allowed');
assert.equal(
  getZGIConsoleNavigationAccess('/console/workflows/workflow-1/batch-test', workflowViewer).status,
  'allowed'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/workflows/workflow-1', workflowViewer).status,
  'permission_denied',
  'view-only users must not be routed into the workflow editor'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/workflows/workflow-1/logs', workflowViewer).status,
  'permission_denied'
);

const workflowEditor = { ...readyContext, permissions: ['workflow.update'] };
assert.equal(
  getZGIConsoleNavigationAccess('/console/workflows/workflow-1', workflowEditor).status,
  'allowed'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/agents/legacy-workflow/workflow', workflowEditor).status,
  'allowed',
  'legacy workflow editor routes must use workflow permissions'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/agents/legacy-workflow', workflowViewer).status,
  'permission_denied',
  'bare legacy asset routes still resolve to an editor surface'
);

assert.equal(
  getZGIConsoleNavigationAccess('/console/workspace/members', readyContext).status,
  'permission_denied'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/workspace/members', {
    ...readyContext,
    workspaceRole: 'admin',
  }).status,
  'allowed'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/workflows', {
    ...readyContext,
    organizationRole: 'admin',
  }).status,
  'allowed',
  'organization administrators bypass workspace feature permission lists'
);

const accessibleRoutes = new Set(
  getAccessibleZGIConsoleSiteMap(readyContext).map(route => route.href)
);
assert.equal(accessibleRoutes.has('/console/workflows'), false);
assert.equal(accessibleRoutes.has('/console/skills'), true);
assert.equal(accessibleRoutes.has('/console/files'), true);

console.log('Console navigation route and permission contract checks passed.');
