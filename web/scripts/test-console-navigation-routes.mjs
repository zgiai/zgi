import assert from 'node:assert/strict';
import {
  ZGI_CONSOLE_SITE_MAP,
  getAccessibleZGIConsoleSiteMap,
  getZGIConsoleNavigationAccess,
  getZGIConsoleNavigationDisplayState,
  getZGIConsoleNavigationTarget,
  normalizeZGIConsoleNavigationHref,
} from '../src/routes/console-navigation.ts';
import { getRecentWorkNavigationHref } from '../src/utils/console-recent-work.ts';

const siteMapRoutes = new Set(ZGI_CONSOLE_SITE_MAP.map(route => route.href));

for (const href of ['/console/workflows', '/console/skills', '/console/integrations']) {
  assert.equal(siteMapRoutes.has(href), true, `site map must include ${href}`);
  assert.equal(normalizeZGIConsoleNavigationHref(href), href, `${href} must be navigable`);
}

for (const href of [
  '/console/integrations/oauth/result',
  '/console/workflows/workflow-1',
  '/console/workflows/workflow-1/logs',
  '/console/workflows/workflow-1/api/keys',
  '/console/workflows/workflow-1/api/docs',
  '/console/workflows/workflow-1/batch-test',
]) {
  assert.equal(normalizeZGIConsoleNavigationHref(href), href, `${href} must be navigable`);
}

for (const href of ['/console/agents/agent-1/api', '/console/workflows/workflow-1/api']) {
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

const loadingAccess = getZGIConsoleNavigationAccess('/console/workflows', {
  ...readyContext,
  permissionsSettled: false,
});
assert.equal(getZGIConsoleNavigationDisplayState(loadingAccess), 'loading');
assert.equal(getZGIConsoleNavigationDisplayState(loadingAccess, true), 'error');
assert.equal(
  getZGIConsoleNavigationDisplayState(
    getZGIConsoleNavigationAccess('/console/workflows', {
      ...readyContext,
      workspaceStatus: 'workspace_required',
    })
  ),
  'setup_required'
);
assert.equal(
  getZGIConsoleNavigationDisplayState(
    getZGIConsoleNavigationAccess('/console/workflows', readyContext)
  ),
  'forbidden'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/workflows', {
    ...readyContext,
    workspaceStatus: 'workspace_required',
  }).status,
  'workspace_required'
);
assert.equal(
  getZGIConsoleNavigationTarget('/console/workflows', 'setup_required'),
  '/console/workspace',
  'setup-required feature links must open the workspace selection and creation surface'
);
assert.equal(
  getZGIConsoleNavigationTarget('/console/workflows', 'available'),
  '/console/workflows',
  'available feature links must retain their original destination'
);
assert.equal(
  getRecentWorkNavigationHref(false, 'agent', 'agent-1'),
  null,
  'recent workspace assets must not produce unusable detail links without an active workspace'
);
assert.equal(
  getRecentWorkNavigationHref(true, 'agent', 'agent-1'),
  '/console/agents/agent-1',
  'recent workspace assets must retain detail links in an active workspace'
);
assert.equal(
  getZGIConsoleNavigationAccess('/console/workflows', {
    ...readyContext,
    permissionsSettled: false,
  }).status,
  'permissions_loading'
);
for (const href of ['/console/workspace', '/console/files', '/console/prompts']) {
  const access = getZGIConsoleNavigationAccess(href, {
    ...readyContext,
    permissionsSettled: false,
  });
  assert.equal(
    access.status,
    'allowed',
    `${href} must remain available when the route does not require feature permissions`
  );
  assert.equal(
    getZGIConsoleNavigationDisplayState(access, true),
    'available',
    `${href} must not surface an unrelated permissions query failure`
  );
}
assert.equal(
  getZGIConsoleNavigationAccess('/console/workflows', {
    ...readyContext,
    permissionsSettled: false,
    organizationRole: 'admin',
  }).status,
  'allowed',
  'organization administrators must not depend on a redundant workspace permission query'
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
