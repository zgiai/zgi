import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const require = createRequire(import.meta.url);
const ts = require('typescript');

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(__dirname, '..');
const accessPath = path.join(rootDir, 'src', 'routes', 'access.ts');
const agentDetailRoutesPath = path.join(rootDir, 'src', 'utils', 'agent-detail-routes.ts');
const consoleRecentWorkPath = path.join(rootDir, 'src', 'utils', 'console-recent-work.ts');
const consolePagePath = path.join(rootDir, 'src', 'app', 'console', 'page.tsx');
const workspaceStorePath = path.join(rootDir, 'src', 'store', 'workspace-store.ts');
const workLayoutPath = path.join(rootDir, 'src', 'app', 'console', 'work', 'layout.tsx');
const workspaceLayoutPath = path.join(rootDir, 'src', 'app', 'console', 'workspace', 'layout.tsx');
const workspacePagePath = path.join(rootDir, 'src', 'app', 'console', 'workspace', 'page.tsx');
const defaultCustomerPath = path.join(rootDir, 'src', 'customer', 'default.tsx');
const accountServicePath = path.join(rootDir, 'src', 'services', 'account.service.ts');
const webAppServicePath = path.join(rootDir, 'src', 'services', 'webapp.service.ts');
const runnableWebAppsHookPath = path.join(
  rootDir,
  'src',
  'hooks',
  'agent',
  'use-runnable-webapps.ts'
);
const builtInWorkflowsHookPath = path.join(
  rootDir,
  'src',
  'hooks',
  'workflow',
  'use-built-in-workflows.ts'
);
const webAppHookPath = path.join(rootDir, 'src', 'hooks', 'webapp', 'use-webapp.ts');
const webAppMigrationHookPath = path.join(
  rootDir,
  'src',
  'hooks',
  'webapp',
  'use-maybe-migrate-user.ts'
);
const webAppLayoutPath = path.join(rootDir, 'src', 'app', 'webapp', '[version_uuid]', 'layout.tsx');
const teamSwitcherPath = path.join(rootDir, 'src', 'components', 'console', 'team-switcher.tsx');
const userMenuPath = path.join(rootDir, 'src', 'components', 'console', 'user-menu.tsx');
const publishSettingsDialogPath = path.join(
  rootDir,
  'src',
  'components',
  'agents',
  'agent-runtime',
  'publish-settings-dialog.tsx'
);
const runtimeAudiencePickerPath = path.join(
  rootDir,
  'src',
  'components',
  'runtime-auth',
  'runtime-audience-picker-dialog.tsx'
);
const runtimeGrantSubjectRowPath = path.join(
  rootDir,
  'src',
  'components',
  'runtime-auth',
  'runtime-grant-subject-row.tsx'
);
const agentRuntimeHeaderPath = path.join(
  rootDir,
  'src',
  'components',
  'agents',
  'agent-runtime',
  'header.tsx'
);
const agentSidebarPath = path.join(rootDir, 'src', 'components', 'agents', 'agent-sidebar.tsx');
const agentApiPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'agents',
  '[agentId]',
  'api',
  'page.tsx'
);
const agentLogsPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'agents',
  '[agentId]',
  'logs',
  'page.tsx'
);
const enterOrganizationModeHookPath = path.join(
  rootDir,
  'src',
  'hooks',
  'workspace',
  'use-enter-organization-mode.ts'
);
const consoleSidebarPath = path.join(
  rootDir,
  'src',
  'components',
  'console',
  'console-sidebar.tsx'
);
const appCenterPaths = [
  path.join(rootDir, 'src', 'app', 'console', 'work', 'app', 'page.tsx'),
  path.join(rootDir, 'src', 'app', 'console', 'work', 'app', 'layout.tsx'),
  path.join(rootDir, 'src', 'app', 'console', 'work', 'app', '[web_app_id]', 'page.tsx'),
];
const organizationProductPagePaths = [
  path.join(rootDir, 'src', 'app', 'console', 'work', 'chat', 'page.tsx'),
  path.join(rootDir, 'src', 'app', 'console', 'work', 'image', 'page.tsx'),
  path.join(rootDir, 'src', 'app', 'console', 'settings', 'page.tsx'),
  ...appCenterPaths,
];

function listFiles(dir) {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  return entries.flatMap(entry => {
    const fullPath = path.join(dir, entry.name);
    return entry.isDirectory() ? listFiles(fullPath) : [fullPath];
  });
}

function routeFromPageFile(routeRoot, routePrefix, filePath) {
  const relativePath = path.relative(routeRoot, filePath);
  if (path.basename(relativePath) !== 'page.tsx') {
    return null;
  }
  const routeDir = path.dirname(relativePath);
  const segments =
    routeDir === '.'
      ? []
      : routeDir.split(path.sep).map(segment => {
          if (segment.startsWith('[') && segment.endsWith(']')) {
            return `:${segment.slice(1, -1)}`;
          }
          return segment;
        });
  return [routePrefix, ...segments].join('/');
}

const source = fs.readFileSync(accessPath, 'utf8');
const compiled = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2020,
  },
}).outputText;

const sandbox = {
  exports: {},
  module: { exports: {} },
};
sandbox.exports = sandbox.module.exports;
vm.runInNewContext(compiled, sandbox, { filename: accessPath });

const {
  getConsoleRouteAccess,
  isOrganizationScopedConsoleRoute,
  isOrganizationScopedWorkRoute,
  ORGANIZATION_SCOPED_CONSOLE_ROUTES,
  ORGANIZATION_SCOPED_CONSOLE_ROUTE_PREFIXES,
  ORGANIZATION_SCOPED_WORK_ROUTES,
  ORGANIZATION_SCOPED_WORK_ROUTE_PREFIXES,
} = sandbox.module.exports;

const agentDetailRoutesSource = fs.readFileSync(agentDetailRoutesPath, 'utf8').replace(
  /import \{ AgentType \} from '@\/services\/types\/agent';\r?\n/,
  `const AgentType = {
  AGENT: 'AGENT',
  WORKFLOW: 'WORKFLOW',
  CONVERSATIONAL_AGENT: 'CONVERSATIONAL_WORKFLOW',
};
`
);
const compiledAgentDetailRoutes = ts.transpileModule(agentDetailRoutesSource, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2020,
  },
}).outputText;
const agentRouteSandbox = {
  exports: {},
  module: { exports: {} },
};
agentRouteSandbox.exports = agentRouteSandbox.module.exports;
vm.runInNewContext(compiledAgentDetailRoutes, agentRouteSandbox, {
  filename: agentDetailRoutesPath,
});
const { canShowAgentApiKeys, canShowAgentRuntimeAccess, getAgentDetailRouteAccess } =
  agentRouteSandbox.module.exports;

const organizationRoutes = [
  '/console',
  '/console/settings',
  '/console/work',
  '/console/work/chat',
  '/console/work/image',
  '/console/work/app',
  '/console/work/app/agent-1',
];

const workRouteRoot = path.join(rootDir, 'src', 'app', 'console', 'work');
const consoleRouteRoot = path.join(rootDir, 'src', 'app', 'console');
const expectedOrganizationConsolePageRoutes = [
  '/console',
  '/console/settings',
  '/console/work',
  '/console/work/app',
  '/console/work/app/:web_app_id',
  '/console/work/chat',
  '/console/work/image',
];
const expectedWorkspaceConsolePageRoutes = [
  '/console/agents',
  '/console/agents/:agentId',
  '/console/agents/:agentId/agent',
  '/console/agents/:agentId/api',
  '/console/agents/:agentId/batch-test',
  '/console/agents/:agentId/batch-test/:batchId',
  '/console/agents/:agentId/batch-test/:batchId/items/:itemId',
  '/console/agents/:agentId/batch-test/batches',
  '/console/agents/:agentId/batch-test/batches/new',
  '/console/agents/:agentId/logs',
  '/console/agents/:agentId/workflow',
  '/console/dataset',
  '/console/dataset/:datasetId',
  '/console/dataset/:datasetId/batch-testing',
  '/console/dataset/:datasetId/batch-testing/:taskId',
  '/console/dataset/:datasetId/documents',
  '/console/dataset/:datasetId/documents/:documentId',
  '/console/dataset/:datasetId/graph',
  '/console/dataset/:datasetId/hit-testing',
  '/console/dataset/:datasetId/settings',
  '/console/db',
  '/console/db/:dbId',
  '/console/db/:dbId/import-excel',
  '/console/db/:dbId/record',
  '/console/db/:dbId/search',
  '/console/db/:dbId/table/:tableId',
  '/console/db/:dbId/table/:tableId/create',
  '/console/db/:dbId/table/:tableId/data',
  '/console/db/:dbId/table/:tableId/structure',
  '/console/developer/content-parse',
  '/console/files',
  '/console/prompts',
  '/console/prompts/:promptId',
  '/console/work/task',
  '/console/workspace',
  '/console/workspace/members',
  '/console/workspace/settings',
];
const actualConsolePageRoutes = listFiles(consoleRouteRoot)
  .map(filePath => routeFromPageFile(consoleRouteRoot, '/console', filePath))
  .filter(Boolean)
  .sort();
const actualWorkPageRoutes = listFiles(workRouteRoot)
  .map(filePath => routeFromPageFile(workRouteRoot, '/console/work', filePath))
  .filter(Boolean)
  .sort();
assert.deepEqual(
  actualConsolePageRoutes,
  [...expectedWorkspaceConsolePageRoutes, ...expectedOrganizationConsolePageRoutes].sort(),
  'console route tree should be explicitly classified by shared access metadata'
);
assert.deepEqual(
  actualConsolePageRoutes.filter(route => isOrganizationScopedConsoleRoute(route)).sort(),
  expectedOrganizationConsolePageRoutes,
  'console organization routes should include the personal workbench, settings, and product work surfaces'
);
assert.deepEqual(
  actualConsolePageRoutes.filter(route => !isOrganizationScopedConsoleRoute(route)).sort(),
  expectedWorkspaceConsolePageRoutes,
  'console asset, builder, workspace-management, and helper routes should remain workspace scoped'
);
assert.deepEqual(
  actualWorkPageRoutes,
  [
    '/console/work',
    '/console/work/app',
    '/console/work/app/:web_app_id',
    '/console/work/chat',
    '/console/work/image',
    '/console/work/task',
  ],
  'console work route tree should be explicitly classified by shared access metadata'
);
assert.deepEqual(
  actualWorkPageRoutes.filter(route => isOrganizationScopedWorkRoute(route)).sort(),
  [
    '/console/work',
    '/console/work/app',
    '/console/work/app/:web_app_id',
    '/console/work/chat',
    '/console/work/image',
  ],
  'console work product routes should remain organization scoped'
);
assert.deepEqual(
  actualWorkPageRoutes.filter(route => !isOrganizationScopedWorkRoute(route)).sort(),
  ['/console/work/task'],
  'console work asset/helper routes should remain workspace scoped'
);

assert.deepEqual(
  [...ORGANIZATION_SCOPED_CONSOLE_ROUTES],
  [
    '/console',
    '/console/settings',
    '/console/work',
    '/console/work/chat',
    '/console/work/image',
    '/console/work/app',
  ],
  'console organization-scoped exact route metadata should include the personal workbench, settings, and product routes'
);
assert.deepEqual(
  [...ORGANIZATION_SCOPED_CONSOLE_ROUTE_PREFIXES],
  ['/console/work/app/'],
  'console organization-scoped prefix metadata should include app detail routes'
);
assert.deepEqual(
  [...ORGANIZATION_SCOPED_WORK_ROUTES],
  ['/console/work', '/console/work/chat', '/console/work/image', '/console/work/app'],
  'work layout organization-scoped exact route metadata should include product routes only'
);
assert.deepEqual(
  [...ORGANIZATION_SCOPED_WORK_ROUTE_PREFIXES],
  ['/console/work/app/'],
  'work layout organization-scoped prefix metadata should include app detail routes'
);
assert.equal(
  ORGANIZATION_SCOPED_WORK_ROUTES.includes('/console/settings'),
  false,
  'settings should be organization-scoped at the console shell, not inside the work layout'
);

for (const route of organizationRoutes) {
  assert.equal(
    isOrganizationScopedConsoleRoute(route),
    true,
    `${route} should be console organization scoped`
  );
  const access = getConsoleRouteAccess(route);
  assert.equal(access.scope, 'organization', `${route} should be organization scoped`);
  assert.equal(access.requiresWorkspace, false, `${route} should not require a workspace`);
}

const organizationWorkRoutes = organizationRoutes.filter(route =>
  route.startsWith('/console/work')
);
for (const route of organizationWorkRoutes) {
  assert.equal(
    isOrganizationScopedWorkRoute(route),
    true,
    `${route} should bypass workspace-only work layout guard`
  );
}

const workspaceRoutes = [
  '/console/work/agents',
  '/console/work/knowledge',
  '/console/work/databases',
  '/console/work/files',
  '/console/work/task',
  '/console/workspace',
  '/console/workspace/members',
  '/console/workspace/settings',
  '/console/developer/content-parse',
  '/console/dashboard',
];

for (const route of workspaceRoutes) {
  assert.equal(
    isOrganizationScopedConsoleRoute(route),
    false,
    `${route} should not be console organization scoped`
  );
  const access = getConsoleRouteAccess(route);
  assert.equal(access.scope, 'workspace', `${route} should be workspace scoped`);
  assert.equal(access.requiresWorkspace, true, `${route} should require a workspace`);
}

for (const route of workspaceRoutes.filter(route => route.startsWith('/console/work'))) {
  assert.equal(
    isOrganizationScopedWorkRoute(route),
    false,
    `${route} should stay behind the workspace-only work layout guard`
  );
}

const workLayoutSource = fs.readFileSync(workLayoutPath, 'utf8');
assert.match(
  workLayoutSource,
  /useAccountCapabilities/,
  'work layout guard should consume account capabilities'
);
assert.doesNotMatch(
  workLayoutSource,
  /useAccountPermissions/,
  'work layout guard should not consume workspace permission hook directly'
);
assert.match(
  workLayoutSource,
  /canUseOrganizationScope/,
  'work layout guard should check organization scope capability'
);
assert.match(
  workLayoutSource,
  /canUseWorkspaceScope/,
  'work layout guard should check workspace scope capability'
);

const workspaceLayoutSource = fs.readFileSync(workspaceLayoutPath, 'utf8');
const workspacePageSource = fs.readFileSync(workspacePagePath, 'utf8');
const consoleRecentWorkSource = fs.readFileSync(consoleRecentWorkPath, 'utf8');
const agentLogsPageSource = fs.readFileSync(agentLogsPagePath, 'utf8');
assert.match(
  workspaceLayoutSource,
  /useAccountCapabilities/,
  'workspace management layout guard should consume account capabilities'
);
assert.match(
  workspaceLayoutSource,
  /canUseWorkspaceScope/,
  'workspace management layout guard should check workspace scope capability'
);
assert.match(
  workspaceLayoutSource,
  /isWorkspaceRequired/,
  'workspace management layout guard should keep missing workspace state distinct from denied capability'
);
assert.match(
  workspaceLayoutSource,
  /WorkspaceRequiredState/,
  'workspace management layout should render the shared missing-workspace state'
);
assert.match(
  workspaceLayoutSource,
  /useAccountPermissions/,
  'workspace management layout should still consume concrete workspace permission state'
);
assert.match(
  workspaceLayoutSource,
  /hasWorkspaceAccess\(\)/,
  'workspace management layout should derive access from workspace membership after capability gating'
);
assert.doesNotMatch(
  workspaceLayoutSource,
  /router\.replace\('\/console'\)/,
  'workspace management layout should block locally instead of silently redirecting away from denied workspace routes'
);
assert.doesNotMatch(
  workspaceLayoutSource,
  /contextStatus !== 'ready' \|\| !currentWorkspace \|\| !hasPermission\('workspace\.view'\)/,
  'workspace management layout should not rely only on local workspace store and permission hook state'
);
assert.match(
  workspacePageSource,
  /useWorkspaceStatistics\(workspaceId,\s*Boolean\(workspaceId\)\)/,
  'workspace overview should load current workspace statistics through the workspace statistics endpoint'
);
assert.match(
  workspacePageSource,
  /workspace\.overview\.management/,
  'workspace overview should render management entry points instead of redirecting'
);
assert.match(
  workspacePageSource,
  /workspace\.overview\.permissions/,
  'workspace overview should summarize current workspace permissions'
);
assert.doesNotMatch(
  workspacePageSource,
  /dashboardService\.getRecentWork|DASHBOARD_KEYS\.recentWork/,
  'workspace overview should not duplicate the personal workbench recent-work feed'
);
assert.doesNotMatch(
  workspacePageSource,
  /redirect\(/,
  'workspace overview should render in place instead of redirecting to members'
);
assert.match(
  consoleRecentWorkSource,
  /conversation_id=\$\{encodeURIComponent\(resourceId\)\}/,
  'recent conversation links should include a conversation_id query parameter'
);
assert.match(
  consoleRecentWorkSource,
  /\/console\/agents\/\$\{parentId\}\/logs\?\$\{query\}/,
  'recent conversation links should target agent logs when an agent id is available'
);
assert.match(
  consoleRecentWorkSource,
  /return `\/console\/agents\/\$\{resourceId\}`;/,
  'recent agent links should use the canonical agent detail entry route'
);
assert.match(
  agentLogsPageSource,
  /searchParams\.get\('conversation_id'\)/,
  'agent logs should read recent-work conversation_id deep links'
);
assert.match(
  agentLogsPageSource,
  /setConversationFilterInput\(nextConversationFilter\)[\s\S]*setConversationFilter\(nextConversationFilter\)/,
  'agent logs should apply conversation_id deep links to the runtime log filter'
);

const defaultCustomerSource = fs.readFileSync(defaultCustomerPath, 'utf8');
assert.match(
  defaultCustomerSource,
  /useAccountCapabilities/,
  'console shell should consume account capabilities for all console route guards'
);
assert.match(
  defaultCustomerSource,
  /routeAccess\.scope === 'organization' && canUseOrganizationScope/,
  'console shell should allow organization routes through organization scope capability'
);
assert.match(
  defaultCustomerSource,
  /routeAccess\.scope === 'workspace'[\s\S]*!isWorkspaceRequired[\s\S]*canUseWorkspaceScope/,
  'console shell should allow workspace routes only when workspace scope capability is available'
);
assert.match(
  defaultCustomerSource,
  /shouldShowWorkspaceRequired\s*=[\s\S]*routeAccess\.scope === 'workspace'[\s\S]*isWorkspaceRequired/,
  'console shell should keep missing workspace state distinct from denied workspace capability'
);
assert.match(
  defaultCustomerSource,
  /shouldShowAccessDenied\s*=[\s\S]*!canUseOrganizationScope[\s\S]*!canUseWorkspaceScope/,
  'console shell should render access denied when the backend capability contract denies the route scope'
);
assert.doesNotMatch(
  defaultCustomerSource,
  /canRenderConsoleChildren\s*=\s*canUseWorkspaceContext\s*\|\|\s*!routeAccess\.requiresWorkspace/,
  'console shell should not infer route access only from local workspace store state'
);

const consoleSidebarSource = fs.readFileSync(consoleSidebarPath, 'utf8');
assert.match(
  consoleSidebarSource,
  /getConsoleRouteAccess/,
  'console sidebar should use shared route access metadata for nav visibility'
);
assert.match(
  consoleSidebarSource,
  /routeAccess\.scope === 'organization'/,
  'console sidebar should keep only organization-scoped nav items in organization mode'
);
assert.doesNotMatch(
  consoleSidebarSource,
  /if\s*\(!isWorkspaceRequired\)\s*{[^}]*hasPermission/s,
  'console sidebar should not skip workspace permission filtering while in organization mode'
);

const consolePageSource = fs.readFileSync(consolePagePath, 'utf8');
const workspaceStoreSource = fs.readFileSync(workspaceStorePath, 'utf8');
const accountServiceSource = fs.readFileSync(accountServicePath, 'utf8');
const runnableWebAppsHookSource = fs.readFileSync(runnableWebAppsHookPath, 'utf8');
const builtInWorkflowsHookSource = fs.readFileSync(builtInWorkflowsHookPath, 'utf8');
const teamSwitcherSource = fs.readFileSync(teamSwitcherPath, 'utf8');
const userMenuSource = fs.readFileSync(userMenuPath, 'utf8');
const enterOrganizationModeHookSource = fs.readFileSync(enterOrganizationModeHookPath, 'utf8');
const publishSettingsDialogSource = fs.readFileSync(publishSettingsDialogPath, 'utf8');
const runtimeAudiencePickerSource = fs.readFileSync(runtimeAudiencePickerPath, 'utf8');
const runtimeGrantSubjectRowSource = fs.readFileSync(runtimeGrantSubjectRowPath, 'utf8');
const agentRuntimeHeaderSource = fs.readFileSync(agentRuntimeHeaderPath, 'utf8');
const agentSidebarSource = fs.readFileSync(agentSidebarPath, 'utf8');
const agentApiPageSource = fs.readFileSync(agentApiPagePath, 'utf8');
assert.match(
  accountServiceSource,
  /export type RuntimeResourceList = 'app_center' \| 'built_in_workflows';/,
  'account capabilities type should name the dedicated runtime resource-list contract keys'
);
assert.match(
  accountServiceSource,
  /export type RuntimeSurface = 'webapp' \| 'api' \| 'app_center' \| 'builtin_app' \| 'internal';/,
  'account capabilities runtime surface type should include app_center separately from builtin_app'
);
assert.match(
  accountServiceSource,
  /runtime_resource_lists:\s*Record<\s*RuntimeResourceList,/,
  'account capabilities response should expose runtime resource-list metadata'
);
assert.match(
  accountServiceSource,
  /can_access_dashboard\?:\s*boolean/,
  'account capabilities type should expose organization dashboard access'
);
assert.match(
  accountServiceSource,
  /can_manage_model_config\?:\s*boolean/,
  'account capabilities type should expose model configuration access'
);
assert.match(
  accountServiceSource,
  /surface:\s*RuntimeSurface;/,
  'runtime resource-list metadata should point back to the authorized runtime surface'
);
assert.match(
  accountServiceSource,
  /endpoint:\s*string;/,
  'runtime resource-list metadata should declare the dedicated list endpoint'
);
assert.match(
  runnableWebAppsHookSource,
  /useAccountCapabilities/,
  'runnable webapp hook should consume account capabilities before loading app-center resources'
);
assert.match(
  runnableWebAppsHookSource,
  /canUseRuntimeResourceList\('app_center'\)/,
  'runnable webapp hook should use the app_center runtime resource-list capability'
);
assert.match(
  runnableWebAppsHookSource,
  /enabled:\s*queryEnabled/,
  'runnable webapp hook should pass the runtime resource-list gate into the list query'
);
assert.match(
  runnableWebAppsHookSource,
  /items:\s*queryEnabled\s*\?\s*items\s*:\s*\[\]/,
  'runnable webapp hook should suppress stale items when the resource-list contract is disabled'
);
assert.match(
  builtInWorkflowsHookSource,
  /useAccountCapabilities/,
  'built-in workflow hook should consume account capabilities before loading catalog resources'
);
assert.match(
  builtInWorkflowsHookSource,
  /canUseRuntimeResourceList\('built_in_workflows'\)/,
  'built-in workflow hook should use the built_in_workflows runtime resource-list capability'
);
assert.match(
  builtInWorkflowsHookSource,
  /enabled:\s*enabled\s*&&\s*resourceListEnabled\s*&&\s*!hasCachedData/,
  'built-in workflow hook should pass the runtime resource-list gate into the catalog query'
);
assert.match(
  builtInWorkflowsHookSource,
  /if\s*\(!resourceListEnabled\)\s*return\s*\[\];/,
  'built-in workflow hook should suppress cached workflows when the resource-list contract is disabled'
);
assert.match(
  consolePageSource,
  /useAccountCapabilities/,
  'console home should consume account capabilities'
);
assert.match(
  consolePageSource,
  /canAccessOrganizationDashboard/,
  'console home should derive dashboard entry visibility from account capabilities'
);
assert.match(
  consolePageSource,
  /canManageModelConfig/,
  'console home should derive model configuration entry visibility from account capabilities'
);
assert.match(
  consolePageSource,
  /useRunnableWebApps\(\{\s*workspaceId:\s*null\s*\}\)/,
  'console home should load runnable apps through organization scope instead of the current workspace'
);
assert.match(
  consolePageSource,
  /href:\s*'\/console\/work\/chat'/,
  'console home should keep chat as an organization-scoped product entry'
);
assert.match(
  consolePageSource,
  /DASHBOARD_KEYS\.recentWork\('overview'\)/,
  'console home should query the organization overview recent-work feed'
);
assert.match(
  consolePageSource,
  /dashboardService\.getRecentWork\(\{[\s\S]*scope:\s*'overview'/,
  'console home recent-work request should use overview scope'
);
assert.match(
  consolePageSource,
  /handleOpenRecentWork/,
  'console home should open recent work through the workspace-aware handler'
);
assert.match(
  workspaceStoreSource,
  /hasPermission:\s*\(permission: PermissionCode\)\s*=>\s*{[\s\S]*?if\s*\(contextStatus !== 'ready'\)\s*{[\s\S]*?return false;/,
  'workspace store hasPermission should fail closed without a ready workspace context'
);
assert.match(
  workspaceStoreSource,
  /hasAnyPermission:\s*\(permissions: (?:readonly )?PermissionCode\[\]\)\s*=>\s*{[\s\S]*?if\s*\(contextStatus !== 'ready'\)\s*{[\s\S]*?return false;/,
  'workspace store hasAnyPermission should fail closed without a ready workspace context'
);
assert.match(
  workspaceStoreSource,
  /hasAllPermissions:\s*\(permissions: (?:readonly )?PermissionCode\[\]\)\s*=>\s*{[\s\S]*?if\s*\(contextStatus !== 'ready'\)\s*{[\s\S]*?return false;/,
  'workspace store hasAllPermissions should fail closed without a ready workspace context'
);
assert.doesNotMatch(
  workspaceStoreSource,
  /contextStatus === 'workspace_required'[\s\S]*permission\.endsWith\('\.view'\)/,
  'workspace store should not synthesize view permissions in organization mode'
);
assert.doesNotMatch(
  workspaceStoreSource,
  /contextStatus === 'workspace_required'[\s\S]*organizationRole === 'owner'/,
  'workspace store should not synthesize admin permissions in organization mode'
);
assert.match(
  teamSwitcherSource,
  /useEnterOrganizationMode/,
  'workspace switcher should expose an explicit organization-mode transition'
);
assert.match(
  userMenuSource,
  /useAccountCapabilities/,
  'user menu dashboard entry should consume account capabilities'
);
assert.match(
  userMenuSource,
  /canAccessOrganizationDashboard/,
  'user menu dashboard entry should use the dashboard access capability'
);
assert.doesNotMatch(
  userMenuSource,
  /organization_role[\s\S]*href="\/dashboard"/,
  'user menu dashboard entry should not be gated directly by local organization_role'
);
assert.match(
  teamSwitcherSource,
  /getConsoleRouteAccess\(pathname\)\.requiresWorkspace/,
  'workspace switcher should use shared route access metadata when deciding organization-mode redirects'
);
assert.match(
  teamSwitcherSource,
  /router\.replace\('\/console\/work\/app'\)/,
  'workspace switcher should redirect workspace-required routes to an organization product route after entering organization mode'
);
assert.match(
  enterOrganizationModeHookSource,
  /mode:\s*'organization'/,
  'enter-organization-mode hook should call the account context API in organization mode'
);
assert.match(
  enterOrganizationModeHookSource,
  /markWorkspaceRequired\(\)/,
  'enter-organization-mode hook should clear the concrete workspace context optimistically'
);
assert.match(
  enterOrganizationModeHookSource,
  /PROFILE_KEYS\.capabilities\(\)/,
  'enter-organization-mode hook should refresh the unified capabilities contract'
);
assert.match(
  enterOrganizationModeHookSource,
  /sessionManager\.broadcastContextChanged\([\s\S]*mode:\s*'organization'/,
  'enter-organization-mode hook should broadcast organization context changes to other tabs'
);
assert.match(
  runtimeGrantSubjectRowSource,
  /departmentIncludesId/,
  'runtime grant subject row should resolve saved department grants against the current organization tree'
);
assert.match(
  runtimeGrantSubjectRowSource,
  /accountGrantUnresolved/,
  'runtime grant subject row should explicitly label saved account grants that no longer hydrate'
);
assert.match(
  runtimeGrantSubjectRowSource,
  /selectionRequired/,
  'runtime grant subject row should show an inline warning before saving incomplete account or department grants'
);
assert.match(
  runtimeGrantSubjectRowSource,
  /accountGrantLookupFailed/,
  'runtime grant subject row should distinguish account lookup failures from stale account grants'
);
assert.match(
  runtimeGrantSubjectRowSource,
  /departmentGrantLookupFailed/,
  'runtime grant subject row should distinguish department lookup failures from stale department grants'
);
assert.match(
  runtimeGrantSubjectRowSource,
  /grantStateMessage/,
  'runtime grant subject row should show an inline warning for incomplete, failed, or stale grants'
);
assert.match(
  publishSettingsDialogSource,
  /RuntimeAudiencePickerDialog/,
  'agent publication settings should edit scoped audiences in a second-level dialog'
);
assert.match(
  publishSettingsDialogSource,
  /RuntimeAudienceChipList/,
  'agent publication settings should show selected audiences as removable chips'
);
assert.match(
  publishSettingsDialogSource,
  /EDITABLE_AUDIENCE_SUBJECTS\s*=\s*\['organization', 'department', 'workspace', 'account'\]/,
  'agent publication settings should support workspace scoped runtime grants'
);
assert.match(
  publishSettingsDialogSource,
  /type WebAppAudienceMode = 'public' \| 'scoped'/,
  'agent publication access should let WebApp switch between public and scoped audiences'
);
assert.match(
  publishSettingsDialogSource,
  /const buildWebAppGrants = \(\): UpdateAgentRuntimeSurfaceGrant\[\] \| null => \{[\s\S]*?webAppAudienceMode === 'public'[\s\S]*?subject_type:\s*'public'[\s\S]*?buildEditableAudienceGrants\(webAppGrants/,
  'agent publication access should build either public or editable scoped WebApp grants'
);
assert.match(
  publishSettingsDialogSource,
  /surface:\s*'webapp',[\s\S]*?enabled:\s*webAppEnabled,[\s\S]*?grants:\s*webApp/,
  'agent publication access should send the selected WebApp audience grants'
);
assert.match(
  publishSettingsDialogSource,
  /surface:\s*'api',[\s\S]*?enabled:\s*apiEnabled,[\s\S]*?grants:\s*\[\{\s*subject_type:\s*'public',\s*enabled:\s*apiEnabled\s*\}\]/,
  'agent publication access should keep API grants public-only while API audience policy is out of scope'
);
assert.match(
  publishSettingsDialogSource,
  /surface:\s*'app_center',[\s\S]*?enabled:\s*appCenterEnabled,[\s\S]*?grants:\s*appCenter/,
  'agent publication access should send editable audience grants for app_center'
);
assert.doesNotMatch(
  publishSettingsDialogSource,
  /surface:\s*'builtin_app'/,
  'ordinary agent publication access should not send builtin_app'
);
assert.match(
  publishSettingsDialogSource,
  /surface:\s*'internal',[\s\S]*?enabled:\s*true,[\s\S]*?grants:\s*\[\{\s*subject_type:\s*'internal',\s*enabled:\s*true\s*\}\]/,
  'agent publication access should preserve the internal runtime grant contract'
);
assert.match(
  publishSettingsDialogSource,
  /wholeOrganizationConfirm/,
  'agent publication access should confirm before replacing scoped audiences with whole-organization access'
);
assert.match(
  publishSettingsDialogSource,
  /closeGuard/,
  'agent publication settings should guard closing the primary dialog with unsaved changes'
);
assert.match(
  publishSettingsDialogSource,
  /hasUnsavedChanges/,
  'agent publication settings should detect unsaved changes before closing'
);
assert.doesNotMatch(
  publishSettingsDialogSource,
  /\b(setApiGrants|apiGrants)\b/,
  'agent publication access should not expose editable API audience grant state while API private access is out of scope'
);
assert.match(
  runtimeAudiencePickerSource,
  /Checkbox/,
  'runtime audience picker should use checkbox selection for mixed audiences'
);
assert.match(
  runtimeAudiencePickerSource,
  /useDepartments/,
  'runtime audience picker should list departments from the organization tree'
);
assert.match(
  runtimeAudiencePickerSource,
  /useWorkspaces/,
  'runtime audience picker should list workspaces from the organization'
);
assert.match(
  runtimeAudiencePickerSource,
  /useCurrentOrganizationMembers/,
  'runtime audience picker should list organization members'
);
assert.match(
  runtimeAudiencePickerSource,
  /subject_type:\s*'department'/,
  'runtime audience picker should support department grants'
);
assert.match(
  runtimeAudiencePickerSource,
  /subject_type:\s*'workspace'/,
  'runtime audience picker should support workspace grants'
);
assert.match(
  runtimeAudiencePickerSource,
  /subject_type:\s*'account'/,
  'runtime audience picker should support account grants'
);
assert.doesNotMatch(
  runtimeAudiencePickerSource,
  /\$\{name\}\s*\(\$\{member\.email\}\)/,
  'runtime audience picker should not append email to member display names'
);
assert.match(
  runtimeAudiencePickerSource,
  /useCurrentOrganizationMember/,
  'runtime audience chips should hydrate saved account grants when possible'
);
assert.match(
  agentRuntimeHeaderSource,
  /PublishSettingsDialog/,
  'agent runtime header should expose publication settings from the publish dropdown'
);
assert.match(
  agentRuntimeHeaderSource,
  /publishSettingsOpen/,
  'agent runtime header should control the publication settings dialog locally'
);
assert.match(
  agentSidebarSource,
  /routeAccess\.canShowApiKeys/,
  'agent sidebar API entry should now be tied to workflow API key visibility'
);
assert.doesNotMatch(
  agentSidebarSource,
  /runtimeAccess\.navTitle/,
  'agent sidebar should no longer show a standalone publication access navigation item'
);
assert.doesNotMatch(
  agentApiPageSource,
  /RuntimeAccessTab/,
  'agent API page should no longer render the publication access tab'
);
assert.match(
  agentApiPageSource,
  /defaultValue="api-keys"/,
  'agent API page should default to workflow API keys after publication access moved to the publish dialog'
);

for (const appCenterPath of appCenterPaths) {
  const appCenterSource = fs.readFileSync(appCenterPath, 'utf8');
  assert.match(
    appCenterSource,
    /useRunnableWebApps\((?:\s*\{[\s\S]*?workspaceId:\s*null[\s\S]*?\}\s*)?\)/,
    `${path.relative(rootDir, appCenterPath)} should load runnable apps through organization scope by default`
  );
  assert.doesNotMatch(
    appCenterSource,
    /enabled:\s*!!workspaceId/,
    `${path.relative(rootDir, appCenterPath)} should not gate runnable apps on current workspace`
  );
  assert.doesNotMatch(
    appCenterSource,
    /useCurrentWorkspace/,
    `${path.relative(rootDir, appCenterPath)} should not require a workspace to render app center data`
  );
}

for (const productPagePath of organizationProductPagePaths) {
  const productPageSource = fs.readFileSync(productPagePath, 'utf8');
  assert.doesNotMatch(
    productPageSource,
    /useAccountPermissions/,
    `${path.relative(rootDir, productPagePath)} should not use workspace permissions at the product page boundary`
  );
  assert.doesNotMatch(
    productPageSource,
    /workspace_required|WorkspaceRequiredState/,
    `${path.relative(rootDir, productPagePath)} should let the shared route guard own workspace-required states`
  );
  assert.doesNotMatch(
    productPageSource,
    /enabled:\s*!!workspace/,
    `${path.relative(rootDir, productPagePath)} should not disable product data loading when workspace is empty`
  );
}

const appCenterDetailSource = fs.readFileSync(appCenterPaths[2], 'utf8');
assert.match(
  appCenterDetailSource,
  /const shouldLoadConfig\s*=\s*!isListLoading\s*&&\s*isRunnable/,
  'app center detail should wait for runnable authorization before loading public webapp config'
);
assert.match(
  appCenterDetailSource,
  /useWebAppConfig\(\s*webAppId,\s*\{\s*enabled:\s*shouldLoadConfig,\s*\}\s*\)/s,
  'app center detail should pass the runnable gate into the webapp config query'
);
assert.doesNotMatch(
  appCenterDetailSource,
  /useWebAppConfig\(\s*webAppId\s*\)/,
  'app center detail should not fetch public webapp config before builtin-app authorization'
);

const webAppHookSource = fs.readFileSync(webAppHookPath, 'utf8');
const webAppMigrationHookSource = fs.readFileSync(webAppMigrationHookPath, 'utf8');
const webAppLayoutSource = fs.readFileSync(webAppLayoutPath, 'utf8');
const webAppServiceSource = fs.readFileSync(webAppServicePath, 'utf8');
const webAppConfigHookSource = webAppHookSource.slice(
  webAppHookSource.indexOf('export function useWebAppConfig'),
  webAppHookSource.indexOf('interface UseWebAppCapabilityOptions')
);
assert.match(
  webAppHookSource,
  /interface UseWebAppConfigOptions[\s\S]*enabled\?: boolean/,
  'webapp config hook should expose an enabled option for caller-owned authorization gates'
);
assert.match(
  webAppHookSource,
  /enabled:\s*enabled\s*&&\s*Boolean\(versionUuid\)/,
  'webapp config hook should combine caller enabled state with the version id guard'
);
assert.match(
  webAppServiceSource,
  /getCapability[\s\S]*\/console\/api\/webapps\/\$\{webAppId\}\/capability/,
  'webapp service should expose the protected capability endpoint for gated runtime flows'
);
assert.match(
  webAppHookSource,
  /function useWebAppCapability[\s\S]*enabled = false[\s\S]*WebAppService\.getCapability/,
  'webapp capability hook should exist as an opt-in query for private webapp gates'
);
assert.match(
  webAppLayoutSource,
  /useWebAppCapability\(runtimeWebAppId,[\s\S]*enabled:\s*Boolean\(isAgentWebApp && runtimeWebAppId\)/,
  'webapp layout should gate agent webapp runtime pages through the protected capability endpoint'
);
assert.match(
  webAppLayoutSource,
  /getWebAppAccessStateKind\(capabilityQuery\.data\?\.data\)/,
  'webapp layout should map capability responses into reader-facing private access states'
);
assert.match(
  webAppLayoutSource,
  /<WebAppAccessState\s+kind=\{accessStateKind\}\s+onLogin=\{handleLogin\}/,
  'webapp layout should render login-required, no-access, and offline states before mounting runtime pages'
);
assert.match(
  webAppLayoutSource,
  /useMaybeMigrateUser\(version_uuid\)/,
  'webapp layout should scope anonymous user migration to the current webapp when possible'
);
assert.match(
  webAppMigrationHookSource,
  /WebAppService\.migrateUser\(localToken,\s*webAppId\)/,
  'webapp migration hook should pass the current webapp id to the migration service'
);
assert.match(
  webAppServiceSource,
  /\/console\/api\/workflows\/\$\{encodeURIComponent\(normalizedWebAppId\)\}\/migrate-user/,
  'webapp service should use the resource-scoped migrate-user route when a webapp id is available'
);
assert.doesNotMatch(
  webAppConfigHookSource,
  /getCapability/,
  'webapp config hook should remain a plain config fetch so callers own their authorization gate'
);

assert.equal(
  canShowAgentApiKeys('AGENT', { canView: true, canManage: true }),
  false,
  'AGENT mode should not show workflow API key/docs tabs'
);
assert.equal(
  canShowAgentRuntimeAccess('AGENT', { canView: true, canManage: true }),
  true,
  'AGENT mode should expose publication access'
);
assert.equal(
  canShowAgentRuntimeAccess('AGENT', { canView: true, canManage: false }),
  false,
  'AGENT publication access should require manage permission'
);
const agentRouteAccess = getAgentDetailRouteAccess('agent-1', 'AGENT', {
  canView: true,
  canManage: true,
});
assert.equal(agentRouteAccess.canShowApiKeys, false, 'AGENT mode should not show API keys');
assert.equal(
  agentRouteAccess.canShowRuntimeAccess,
  true,
  'AGENT mode should still show publication access'
);
const workflowRouteAccess = getAgentDetailRouteAccess('agent-1', 'WORKFLOW', {
  canView: true,
  canManage: true,
});
assert.equal(workflowRouteAccess.canShowApiKeys, true, 'Workflow mode should show API keys');
assert.equal(
  workflowRouteAccess.canShowRuntimeAccess,
  true,
  'Workflow mode should show publication access'
);

console.log('route access scope check passed.');
