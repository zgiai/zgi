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
const promptListPagePath = path.join(rootDir, 'src', 'app', 'console', 'prompts', 'page.tsx');
const promptDetailPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'prompts',
  '[promptId]',
  'page.tsx'
);
const promptUsageSummaryPath = path.join(
  rootDir,
  'src',
  'components',
  'prompts',
  'prompt-usage-summary.tsx'
);
const contentParsePagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'developer',
  'content-parse',
  'page.tsx'
);
const contentParsePlaygroundPath = path.join(
  rootDir,
  'src',
  'components',
  'content-parse',
  'playground',
  'content-parse-playground.tsx'
);
const dashboardLayoutPath = path.join(rootDir, 'src', 'app', 'dashboard', 'layout.tsx');
const dashboardModelSettingsPath = path.join(
  rootDir,
  'src',
  'app',
  'dashboard',
  'settings',
  'model',
  'page.tsx'
);
const dashboardParserSettingsPath = path.join(
  rootDir,
  'src',
  'app',
  'dashboard',
  'settings',
  'parsers',
  'page.tsx'
);
const dashboardChannelPagePath = path.join(rootDir, 'src', 'app', 'dashboard', 'channel', 'page.tsx');
const accountCapabilitiesHookPath = path.join(rootDir, 'src', 'hooks', 'use-account-capabilities.ts');
const taskPagePath = path.join(rootDir, 'src', 'app', 'console', 'work', 'task', 'page.tsx');
const taskWorkbenchPath = path.join(
  rootDir,
  'src',
  'components',
  'automation',
  'task-workbench.tsx'
);
const filePagePath = path.join(rootDir, 'src', 'app', 'console', 'files', 'page.tsx');
const fileDetailPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'files',
  '[fileId]',
  'page.tsx'
);
const fileDetailShellPath = path.join(
  rootDir,
  'src',
  'components',
  'files',
  'detail',
  'file-detail-shell.tsx'
);
const fileListPath = path.join(rootDir, 'src', 'components', 'files', 'file-list.tsx');
const relatedResourcesPopoverPath = path.join(
  rootDir,
  'src',
  'components',
  'files',
  'related-resources-popover.tsx'
);
const dbPagePath = path.join(rootDir, 'src', 'app', 'console', 'db', 'page.tsx');
const dbOverviewPath = path.join(rootDir, 'src', 'app', 'console', 'db', '[dbId]', 'page.tsx');
const dbLayoutPath = path.join(rootDir, 'src', 'app', 'console', 'db', '[dbId]', 'layout.tsx');
const dbImportExcelPath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'db',
  '[dbId]',
  'import-excel',
  'page.tsx'
);
const dbSearchPath = path.join(rootDir, 'src', 'app', 'console', 'db', '[dbId]', 'search', 'page.tsx');
const dbRecordPath = path.join(rootDir, 'src', 'app', 'console', 'db', '[dbId]', 'record', 'page.tsx');
const dbTablePagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'db',
  '[dbId]',
  'table',
  '[tableId]',
  'page.tsx'
);
const dbTableStructurePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'db',
  '[dbId]',
  'table',
  '[tableId]',
  'structure',
  'page.tsx'
);
const dbTableCreatePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'db',
  '[dbId]',
  'table',
  '[tableId]',
  'create',
  'page.tsx'
);
const dbTableDataPath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'db',
  '[dbId]',
  'table',
  '[tableId]',
  'data',
  'page.tsx'
);
const dbTableDataComponentPath = path.join(
  rootDir,
  'src',
  'components',
  'db',
  'table-data',
  'index.tsx'
);
const excelImportShellPath = path.join(
  rootDir,
  'src',
  'components',
  'db',
  'excel-import',
  'excel-import-shell.tsx'
);
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
const agentRuntimePageModelPath = path.join(
  rootDir,
  'src',
  'components',
  'agents',
  'agent-runtime',
  'hooks',
  'use-agent-runtime-page-model.tsx'
);
const agentRuntimePromptPanelPath = path.join(
  rootDir,
  'src',
  'components',
  'agents',
  'agent-runtime',
  'prompt-panel.tsx'
);
const agentRuntimeOrchestrationPanelPath = path.join(
  rootDir,
  'src',
  'components',
  'agents',
  'agent-runtime',
  'orchestration-panel.tsx'
);
const agentRuntimeDatabaseSectionPath = path.join(
  rootDir,
  'src',
  'components',
  'agents',
  'agent-runtime',
  'sections',
  'database-section.tsx'
);
const agentRuntimeKnowledgeSectionPath = path.join(
  rootDir,
  'src',
  'components',
  'agents',
  'agent-runtime',
  'sections',
  'knowledge-section.tsx'
);
const agentsPagePath = path.join(rootDir, 'src', 'app', 'console', 'agents', 'page.tsx');
const createAgentDialogPath = path.join(
  rootDir,
  'src',
  'components',
  'agents',
  'agent-dialog',
  'create-dialog.tsx'
);
const agentCardPath = path.join(rootDir, 'src', 'components', 'agents', 'agent-card.tsx');
const agentEntryPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'agents',
  '[agentId]',
  'page.tsx'
);
const agentRuntimePagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'agents',
  '[agentId]',
  'agent',
  'page.tsx'
);
const agentLayoutPath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'agents',
  '[agentId]',
  'layout.tsx'
);
const datasetPagePath = path.join(rootDir, 'src', 'app', 'console', 'dataset', 'page.tsx');
const datasetCardPath = path.join(rootDir, 'src', 'components', 'datasets', 'dataset-card.tsx');
const datasetFolderCardPath = path.join(
  rootDir,
  'src',
  'components',
  'datasets',
  'folder-card.tsx'
);
const datasetHooksPath = path.join(rootDir, 'src', 'hooks', 'dataset', 'use-datasets.ts');
const datasetHitResultItemPath = path.join(
  rootDir,
  'src',
  'components',
  'datasets',
  'hit-testing',
  'components',
  'result-item.tsx'
);
const datasetDetailRootPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'dataset',
  '[datasetId]',
  'page.tsx'
);
const datasetDetailLayoutPath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'dataset',
  '[datasetId]',
  'layout.tsx'
);
const templateGalleryDialogPath = path.join(
  rootDir,
  'src',
  'components',
  'agents',
  'templates',
  'template-gallery-dialog.tsx'
);
const createFromTemplateHookPath = path.join(
  rootDir,
  'src',
  'components',
  'agents',
  'templates',
  'use-create-from-template.ts'
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
const agentBatchTestPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'agents',
  '[agentId]',
  'batch-test',
  'page.tsx'
);
const agentBatchTestBatchesPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'agents',
  '[agentId]',
  'batch-test',
  'batches',
  'page.tsx'
);
const agentBatchTestNewBatchPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'agents',
  '[agentId]',
  'batch-test',
  'batches',
  'new',
  'page.tsx'
);
const agentBatchTestBatchPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'agents',
  '[agentId]',
  'batch-test',
  '[batchId]',
  'page.tsx'
);
const agentBatchTestBatchItemPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'agents',
  '[agentId]',
  'batch-test',
  '[batchId]',
  'items',
  '[itemId]',
  'page.tsx'
);
const workflowEditorPagePath = path.join(
  rootDir,
  'src',
  'app',
  'console',
  'agents',
  '[agentId]',
  'workflow',
  'page.tsx'
);
const workflowEditorPath = path.join(rootDir, 'src', 'components', 'workflow', 'index.tsx');
const workflowLifecyclePath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'hooks',
  'use-workflow-lifecycle.ts'
);
const workflowNodeDataUpdateHookPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'hooks',
  'use-node-data-update.ts'
);
const workflowOperationsHookPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'hooks',
  'use-workflow-operations.ts'
);
const workflowCanvasWithDndPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'canvas-with-dnd.tsx'
);
const workflowGlobalContainerOverlayPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'global-container-overlay.tsx'
);
const workflowCustomHandlePath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'ui',
  'custom-handle.tsx'
);
const workflowContainerNodePath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'nodes',
  'container',
  'index.tsx'
);
const workflowNodeResizeHandlePath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'nodes',
  'custom',
  'node-resize-handle.tsx'
);
const workflowAutoDimensionsSyncPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'nodes',
  'hooks',
  'use-auto-dimensions-sync.ts'
);
const workflowNoteNodePath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'nodes',
  'note',
  'index.tsx'
);
const workflowCreateNodeModalHostPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'ui',
  'create-node-modal',
  'index.tsx'
);
const workflowContextMenuPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'ui',
  'context-menu.tsx'
);
const workflowBottomToolbarPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'ui',
  'workflow-bottom-toolbar.tsx'
);
const workflowKeyboardHookPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'hooks',
  'use-workflow-keyboard.ts'
);
const workflowApprovalManagerPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'nodes',
  'approval',
  'manager',
  'index.tsx'
);
const workflowCreateNodeModalPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'ui',
  'create-node-modal',
  'create-node-modal.tsx'
);
const workflowCreationActionsPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'ui',
  'create-node-modal',
  'hooks',
  'use-creation-actions.ts'
);
const workflowDatabasePickerPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'common',
  'datasource-picker-dialog',
  'index.tsx'
);
const workflowCallDatabaseManagerPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'nodes',
  'call-database',
  'manager',
  'index.tsx'
);
const workflowCallDatabaseInsertMenusPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'nodes',
  'call-database',
  'manager',
  'sql-editor',
  'insert-menus',
  'index.tsx'
);
const workflowCallDatabaseExpandedDialogPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'nodes',
  'call-database',
  'manager',
  'sql-editor',
  'expanded-dialog.tsx'
);
const workflowSqlGeneratorManagerPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'nodes',
  'sql-generator',
  'manager',
  'index.tsx'
);
const workflowKnowledgeRetrievalManagerPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'nodes',
  'knowledge-retrieval',
  'manager',
  'index.tsx'
);
const workflowKnowledgeRecallSettingsPath = path.join(
  rootDir,
  'src',
  'components',
  'workflow',
  'nodes',
  'knowledge-retrieval',
  'manager',
  'recall-settings-dialog',
  'index.tsx'
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
const permissionConstantsPath = path.join(rootDir, 'src', 'constants', 'permissions.ts');
const commonI18nPaths = [
  path.join(rootDir, 'src', 'i18n', 'modules', 'common', 'zh-Hans.ts'),
  path.join(rootDir, 'src', 'i18n', 'modules', 'common', 'en-US.ts'),
];
const webAppI18nPaths = [
  path.join(rootDir, 'src', 'i18n', 'modules', 'webapp', 'zh-Hans.ts'),
  path.join(rootDir, 'src', 'i18n', 'modules', 'webapp', 'en-US.ts'),
];
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

function regexpEscape(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function collectStringLiterals(sourceText) {
  return [...sourceText.matchAll(/['"]([^'"]+)['"]/g)].map(match => match[1]);
}

function sourceSliceBetween(sourceText, startMarker, endMarker) {
  const start = sourceText.indexOf(startMarker);
  assert.notEqual(start, -1, `missing source marker: ${startMarker}`);
  const end = sourceText.indexOf(endMarker, start);
  assert.notEqual(end, -1, `missing source marker: ${endMarker}`);
  return sourceText.slice(start, end);
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
const {
  canShowAgentApiKeys,
  canShowAgentBatchTest,
  canShowAgentRuntimeAccess,
  getAgentDetailDefaultHref,
  getAgentDetailRouteAccess,
} = agentRouteSandbox.module.exports;

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
  '/console/files/:fileId',
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
  '/console/files',
  '/console/files/file-1',
  '/console/prompts',
  '/console/prompts/prompt-1',
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
const promptListPageSource = fs.readFileSync(promptListPagePath, 'utf8');
const promptDetailPageSource = fs.readFileSync(promptDetailPagePath, 'utf8');
const promptUsageSummarySource = fs.readFileSync(promptUsageSummaryPath, 'utf8');
const contentParsePageSource = fs.readFileSync(contentParsePagePath, 'utf8');
const contentParsePlaygroundSource = fs.readFileSync(contentParsePlaygroundPath, 'utf8');
const dashboardLayoutSource = fs.readFileSync(dashboardLayoutPath, 'utf8');
const dashboardModelSettingsSource = fs.readFileSync(dashboardModelSettingsPath, 'utf8');
const dashboardParserSettingsSource = fs.readFileSync(dashboardParserSettingsPath, 'utf8');
const dashboardChannelPageSource = fs.readFileSync(dashboardChannelPagePath, 'utf8');
const accountCapabilitiesHookSource = fs.readFileSync(accountCapabilitiesHookPath, 'utf8');
const taskPageSource = fs.readFileSync(taskPagePath, 'utf8');
const taskWorkbenchSource = fs.readFileSync(taskWorkbenchPath, 'utf8');
const filePageSource = fs.readFileSync(filePagePath, 'utf8');
const fileDetailPageSource = fs.readFileSync(fileDetailPagePath, 'utf8');
const fileDetailShellSource = fs.readFileSync(fileDetailShellPath, 'utf8');
const fileListSource = fs.readFileSync(fileListPath, 'utf8');
const relatedResourcesPopoverSource = fs.readFileSync(relatedResourcesPopoverPath, 'utf8');
const dbPageSource = fs.readFileSync(dbPagePath, 'utf8');
const dbOverviewSource = fs.readFileSync(dbOverviewPath, 'utf8');
const dbLayoutSource = fs.readFileSync(dbLayoutPath, 'utf8');
const dbImportExcelSource = fs.readFileSync(dbImportExcelPath, 'utf8');
const dbSearchSource = fs.readFileSync(dbSearchPath, 'utf8');
const dbRecordSource = fs.readFileSync(dbRecordPath, 'utf8');
const dbTablePageSource = fs.readFileSync(dbTablePagePath, 'utf8');
const dbTableStructureSource = fs.readFileSync(dbTableStructurePath, 'utf8');
const dbTableCreateSource = fs.readFileSync(dbTableCreatePath, 'utf8');
const dbTableDataSource = fs.readFileSync(dbTableDataPath, 'utf8');
const dbTableDataComponentSource = fs.readFileSync(dbTableDataComponentPath, 'utf8');
const excelImportShellSource = fs.readFileSync(excelImportShellPath, 'utf8');
const consoleRecentWorkSource = fs.readFileSync(consoleRecentWorkPath, 'utf8');
const agentLogsPageSource = fs.readFileSync(agentLogsPagePath, 'utf8');
const workflowEditorSource = fs.readFileSync(workflowEditorPath, 'utf8');
const workflowLifecycleSource = fs.readFileSync(workflowLifecyclePath, 'utf8');
const workflowNodeDataUpdateHookSource = fs.readFileSync(workflowNodeDataUpdateHookPath, 'utf8');
const workflowOperationsHookSource = fs.readFileSync(workflowOperationsHookPath, 'utf8');
const workflowCanvasWithDndSource = fs.readFileSync(workflowCanvasWithDndPath, 'utf8');
const workflowGlobalContainerOverlaySource = fs.readFileSync(
  workflowGlobalContainerOverlayPath,
  'utf8'
);
const workflowCustomHandleSource = fs.readFileSync(workflowCustomHandlePath, 'utf8');
const workflowContainerNodeSource = fs.readFileSync(workflowContainerNodePath, 'utf8');
const workflowNodeResizeHandleSource = fs.readFileSync(workflowNodeResizeHandlePath, 'utf8');
const workflowAutoDimensionsSyncSource = fs.readFileSync(workflowAutoDimensionsSyncPath, 'utf8');
const workflowNoteNodeSource = fs.readFileSync(workflowNoteNodePath, 'utf8');
const workflowCreateNodeModalHostSource = fs.readFileSync(
  workflowCreateNodeModalHostPath,
  'utf8'
);
const workflowContextMenuSource = fs.readFileSync(workflowContextMenuPath, 'utf8');
const workflowBottomToolbarSource = fs.readFileSync(workflowBottomToolbarPath, 'utf8');
const workflowKeyboardHookSource = fs.readFileSync(workflowKeyboardHookPath, 'utf8');
const workflowApprovalManagerSource = fs.readFileSync(workflowApprovalManagerPath, 'utf8');
const workflowCreateNodeModalSource = fs.readFileSync(workflowCreateNodeModalPath, 'utf8');
const workflowCreationActionsSource = fs.readFileSync(workflowCreationActionsPath, 'utf8');
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
assert.match(
  workspacePageSource,
  /workspace\.overview\.permissions\.membership/,
  'workspace overview should describe workspace membership as a role state instead of a selectable workspace permission'
);
assert.match(
  workspacePageSource,
  /workspace\.overview\.permissions\.governanceAccess/,
  'workspace overview should describe workspace management as governance authority'
);
assert.doesNotMatch(
  workspacePageSource,
  /workspace\.overview\.permissions\.workspace(View|Manage)/,
  'workspace overview should not present retired workspace view/manage codes as ordinary member permissions'
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
  promptListPageSource,
  /useAccountPermissions\(\)/,
  'prompt list should consume the shared workspace access contract'
);
assert.match(
  promptListPageSource,
  /hasWorkspaceAccess\(\)/,
  'prompt list should gate retired prompt tools by workspace access, not prompt permissions'
);
assert.match(
  promptListPageSource,
  /usePrompts\([\s\S]*canView\s*\)/,
  'prompt list should disable prompt queries when workspace access is unavailable'
);
assert.doesNotMatch(
  promptListPageSource,
  /['"]prompt\./,
  'prompt list should not reintroduce prompt.* member permission codes'
);
assert.match(
  promptDetailPageSource,
  /useAccountPermissions\(\)/,
  'prompt detail should consume the shared workspace access contract'
);
assert.match(
  promptDetailPageSource,
  /hasWorkspaceAccess\(\)/,
  'prompt detail should gate retired prompt tools by workspace access, not prompt permissions'
);
assert.match(
  promptDetailPageSource,
  /usePrompt\(promptId,\s*canView\)/,
  'prompt detail should disable prompt queries when workspace access is unavailable'
);
assert.match(
  promptDetailPageSource,
  /WorkspaceMismatchGuard[\s\S]*targetWorkspaceId=\{targetWorkspaceId\}/,
  'prompt detail should keep workspace-owned prompts behind the workspace mismatch guard'
);
assert.doesNotMatch(
  promptDetailPageSource,
  /['"]prompt\./,
  'prompt detail should not reintroduce prompt.* member permission codes'
);
assert.match(
  promptUsageSummarySource,
  /href=\{`\/console\/agents\/\$\{reference\.agent_id\}\?nodeId=\$\{reference\.node_id\}`\}/,
  'prompt usage references should route through the permission-aware agent detail root while preserving node focus'
);
assert.doesNotMatch(
  promptUsageSummarySource,
  /\/console\/agents\/\$\{reference\.agent_id\}\/workflow\?nodeId=/,
  'prompt usage references should not bypass agent detail root permissions'
);
assert.match(
  contentParsePageSource,
  /<ContentParsePlayground \/>/,
  'content-parse route should delegate to the playground behind the workspace-scoped console guard'
);
assert.doesNotMatch(
  contentParsePlaygroundSource,
  /useAccountPermissions|hasPermission|hasAnyPermission|['"]content_parse\./,
  'content-parse playground should not reintroduce content_parse.* member permission codes'
);
assert.match(
  dashboardLayoutSource,
  /<ProtectedRoute requireAdmin fallback=\{<DashboardAccessDenied \/>}/,
  'dashboard management routes should stay behind the organization admin route guard'
);
assert.match(
  accountCapabilitiesHookSource,
  /canManageModelConfig:\s*[\s\S]*capabilities\?\.organization\.can_manage_model_config[\s\S]*isOrganizationAdmin/,
  'model and parser configuration entry capability should come from organization admin capabilities'
);
for (const [dashboardSource, dashboardName] of [
  [dashboardModelSettingsSource, 'model settings'],
  [dashboardParserSettingsSource, 'parser settings'],
  [dashboardChannelPageSource, 'channel settings'],
]) {
  assert.doesNotMatch(
    dashboardSource,
    /useAccountPermissions|has(?:Any|All)?Permission\s*\(/,
    `dashboard ${dashboardName} page should not consume workspace member permission helpers`
  );
  assert.doesNotMatch(
    dashboardSource,
    /has(?:Any|All)?Permission\s*\([\s\S]{0,240}['"](?:workspace|prompt|content_parse|dashboard)\./,
    `dashboard ${dashboardName} page should not reintroduce retired workspace/content/dashboard permissions`
  );
}
assert.match(
  taskPageSource,
  /<TaskWorkbench \/>/,
  'scheduled-task route should delegate to the task workbench behind the workspace-scoped work guard'
);
assert.match(
  taskWorkbenchSource,
  /useAccountPermissions\(\)/,
  'scheduled-task workbench should consume account permission helpers'
);
assert.match(
  taskWorkbenchSource,
  /const canManageTasks\s*=\s*Boolean\(workspaceId\)\s*&&\s*isWorkspaceManager\(\)/,
  'scheduled-task mutation UI should use workspace manager authority'
);
assert.doesNotMatch(
  taskWorkbenchSource,
  /hasPermission\(['"]workspace\.|hasAnyPermission\(\[['"]workspace\./,
  'scheduled-task workbench should not reintroduce ordinary workspace.* member permissions'
);
assert.match(
  filePageSource,
  /hasAnyPermission\(FILE_VISIBLE_PERMISSION_CODES\)/,
  'file list page should gate direct access by file visible permissions'
);
assert.match(
  filePageSource,
  /<PermissionDeniedState \/>/,
  'file list page should show the shared access-denied state when file permissions are absent'
);
assert.match(
  fileDetailPageSource,
  /<FileDetailShell fileId=\{fileId\} \/>/,
  'file direct page should render the shared file detail shell'
);
assert.match(
  fileDetailShellSource,
  /FILE_PERMISSION_ACTIONS\.metadataView[\s\S]*FILE_PERMISSION_ACTIONS\.preview[\s\S]*FILE_PERMISSION_ACTIONS\.download/,
  'file detail access should be based on readable file action permissions'
);
assert.doesNotMatch(
  fileDetailShellSource.match(/const canOpenFileDetail[\s\S]*?\n\s*\]\);/)?.[0] ?? '',
  /FILE_PERMISSION_ACTIONS\.upload/,
  'file detail access should not treat upload-only permission as existing-file view access'
);
assert.match(
  fileDetailShellSource,
  /const canDownload\s*=\s*hasAnyPermission\(FILE_PERMISSION_ACTIONS\.download\)/,
  'file detail download action should use the exact file.download action group'
);
assert.doesNotMatch(
  fileDetailShellSource,
  /hasPermission\(['"]file\.download['"]\)/,
  'file detail download action should not bypass the file action matrix with a raw permission literal'
);
assert.match(
  fileDetailShellSource,
  /const canUpdateFile\s*=\s*hasAnyPermission\(FILE_PERMISSION_ACTIONS\.update\)/,
  'file detail update action should use the exact file.update permission'
);
assert.doesNotMatch(
  fileDetailShellSource,
  /['"]file\.(?:view|manage|upload_create|move_create)['"]/,
  'file detail should not consume legacy aggregate file permission codes'
);
assert.match(
  fileListSource,
  /const canOpenFileDetailByPermission\s*=\s*hasAnyPermission\(\[[\s\S]*FILE_PERMISSION_ACTIONS\.metadataView[\s\S]*FILE_PERMISSION_ACTIONS\.download[\s\S]*FILE_PERMISSION_ACTIONS\.update/,
  'file list detail entry should use existing-file readable/action permissions'
);
assert.doesNotMatch(
  fileListSource.match(/const canOpenFileDetailByPermission[\s\S]*?\n\s*\]\);/)?.[0] ?? '',
  /FILE_PERMISSION_ACTIONS\.upload/,
  'file list detail entry should not be visible for upload-only permission'
);
assert.match(
  fileListSource,
  /const canViewDetail\s*=\s*!selectionMode && canOpenFileDetailByPermission/,
  'file list detail link should combine mode and permission gate'
);
assert.match(
  relatedResourcesPopoverSource,
  /router\.push\(`\/console\/dataset\/\$\{resource\.id\}`\)/,
  'file related-resource dataset links should route through the permission-aware dataset detail root'
);
assert.doesNotMatch(
  relatedResourcesPopoverSource,
  /router\.push\(`\/console\/dataset\/\$\{resource\.id\}\/documents`\)/,
  'file related-resource dataset links should not assume document access'
);
assert.match(
  dbPageSource,
  /const canCreateDatabase\s*=\s*hasAnyPermission\(DATABASE_PERMISSION_ACTIONS\.create\)/,
  'database list create action should use the exact database.create action group'
);
assert.match(
  dbPageSource,
  /const canUpdateDatabase\s*=\s*hasAnyPermission\(DATABASE_PERMISSION_ACTIONS\.update\)/,
  'database list edit action should use the exact database.update action group'
);
assert.match(
  dbPageSource,
  /const openEdit = \(db: Db\) => \{[\s\S]*?if \(!canUpdateDatabase\) return;/,
  'database list edit callback should not require database.create'
);
assert.doesNotMatch(
  dbPageSource,
  /hasPermission\(['"]database\.create['"]\)/,
  'database list should not gate create UI with a raw permission literal'
);
assert.match(
  dbLayoutSource,
  /const canViewTableMetadata\s*=\s*hasAnyPermission\(DATABASE_TABLE_METADATA_PERMISSION_CODES\)/,
  'database detail layout should derive table-list visibility from the shared table metadata permission group'
);
assert.match(
  dbLayoutSource,
  /useDbTables\(dbId,\s*\{[\s\S]*enabled:\s*canViewTableMetadata && !isMismatch/,
  'database detail layout should not fetch tables for users without table metadata permissions'
);
assert.match(
  dbLayoutSource,
  /\{canViewTableMetadata && \([\s\S]*<button[\s\S]*\{t\('dbs\.tables'\)\}/,
  'database detail layout should hide the table navigation group without table metadata permissions'
);
assert.match(
  dbOverviewSource,
  /const canViewTableMetadata\s*=\s*hasAnyPermission\(DATABASE_TABLE_METADATA_PERMISSION_CODES\)/,
  'database overview should use the shared table metadata permission group'
);
assert.match(
  dbOverviewSource,
  /useDbTables\(dbId as string,\s*\{[\s\S]*enabled:\s*canViewTableMetadata/,
  'database overview should not fetch table list without table metadata permissions'
);
assert.match(
  dbSearchSource,
  /const canAiQuery\s*=\s*hasAnyPermission\(\[[\s\S]*DATABASE_PERMISSION_ACTIONS\.aiQueryRead[\s\S]*DATABASE_PERMISSION_ACTIONS\.aiQueryWrite/,
  'database BI search direct page should require AI query read or write permission'
);
assert.match(
  dbRecordSource,
  /const canViewOperationLogs\s*=\s*hasAnyPermission\(DATABASE_PERMISSION_ACTIONS\.operationLogsView\)/,
  'database operation-log direct page should require database.operation_logs.view'
);
assert.match(
  dbImportExcelSource,
  /<ExcelImportShell dbId=\{dbId\} \/>/,
  'database Excel import direct page should delegate to the shared guarded import shell'
);
assert.match(
  excelImportShellSource,
  /const canAnalyzeImport\s*=\s*hasAnyPermission\(DATABASE_PERMISSION_ACTIONS\.importAnalyze\)/,
  'database Excel import shell should check the import analyze permission'
);
assert.match(
  excelImportShellSource,
  /const canExecuteImport\s*=\s*hasAnyPermission\(DATABASE_PERMISSION_ACTIONS\.importExecute\)/,
  'database Excel import shell should check the import execute permission'
);
assert.match(
  excelImportShellSource,
  /if \(!canAnalyzeImport && !canExecuteImport\) \{[\s\S]*ShieldAlert/,
  'database Excel import shell should block direct access when import permissions are absent'
);
assert.match(
  dbTablePageSource,
  /const canOpenRecords\s*=\s*hasAnyPermission\(\[[\s\S]*DATABASE_PERMISSION_ACTIONS\.recordView[\s\S]*DATABASE_PERMISSION_ACTIONS\.recordCreate[\s\S]*DATABASE_PERMISSION_ACTIONS\.recordUpdate[\s\S]*DATABASE_PERMISSION_ACTIONS\.recordDelete/,
  'database table record direct page should require a record action permission'
);
assert.match(
  dbTableStructureSource,
  /const canOpenSchema\s*=\s*hasAnyPermission\(\[[\s\S]*DATABASE_PERMISSION_ACTIONS\.schemaView[\s\S]*DATABASE_PERMISSION_ACTIONS\.schemaManage/,
  'database table structure direct page should require schema view or schema manage'
);
assert.match(
  dbTableCreateSource,
  /const canManageSchema\s*=\s*hasAnyPermission\(DATABASE_PERMISSION_ACTIONS\.schemaManage\)/,
  'database AI table creation direct page should require schema manage'
);
assert.match(
  dbTableDataSource,
  /const canAnalyzeImport\s*=\s*hasAnyPermission\(DATABASE_PERMISSION_ACTIONS\.importAnalyze\)/,
  'database smart ingest direct page should require import analyze permission'
);
assert.match(
  dbTableDataSource,
  /const canCreateRecord\s*=\s*hasAnyPermission\(DATABASE_PERMISSION_ACTIONS\.recordCreate\)/,
  'database smart ingest direct page should require record creation permission'
);
assert.match(
  dbTableDataSource,
  /const canUseSmartIngest\s*=\s*canAnalyzeImport && canCreateRecord/,
  'database smart ingest direct page should combine import analyze with record creation'
);
assert.match(
  dbTableDataSource,
  /const canViewTablePrompt\s*=\s*hasAnyPermission\(\[[\s\S]*DATABASE_PERMISSION_ACTIONS\.tablePromptView[\s\S]*DATABASE_PERMISSION_ACTIONS\.tablePromptManage/,
  'database table prompt panel should require table prompt view or manage'
);
assert.match(
  dbTableDataComponentSource,
  /const canAnalyzeImport\s*=\s*hasAnyPermission\(DATABASE_PERMISSION_ACTIONS\.importAnalyze\)/,
  'database table component should derive smart ingest from import analyze permission'
);
assert.match(
  dbTableDataComponentSource,
  /const canExecuteImport\s*=\s*hasAnyPermission\(DATABASE_PERMISSION_ACTIONS\.importExecute\)/,
  'database table component should derive direct batch import from import execute permission'
);
assert.match(
  dbTableDataComponentSource,
  /const canBatchImport\s*=\s*canExecuteImport/,
  'database table batch import entry should match the backend direct import execute permission'
);
assert.match(
  dbTableDataComponentSource,
  /const canSmartIngest\s*=\s*canAnalyzeImport && canCreateRecord/,
  'database table smart ingest entry should require analyze plus record creation'
);
assert.doesNotMatch(
  dbTableDataComponentSource,
  /canSmartIngest\s*=\s*hasAnyPermission\(\[[\s\S]*DATABASE_PERMISSION_ACTIONS\.aiQueryWrite/,
  'database smart ingest should not be gated by database AI-query write permission'
);
assert.match(
  dbLayoutSource,
  /const canViewOperationLogs\s*=\s*hasAnyPermission\(DATABASE_PERMISSION_ACTIONS\.operationLogsView\)/,
  'database detail logs navigation should use the operationLogsView action group'
);
assert.doesNotMatch(
  dbLayoutSource,
  /hasPermission\(['"]database\.operation_logs\.view['"]\)/,
  'database detail layout should not gate logs navigation with a raw permission literal'
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
const permissionConstantsSource = fs.readFileSync(permissionConstantsPath, 'utf8');
assert.match(
  permissionConstantsSource,
  /export const DATABASE_TABLE_METADATA_PERMISSION_CODES = \[[\s\S]*DATABASE_PERMISSION_ACTIONS\.schemaView[\s\S]*DATABASE_PERMISSION_ACTIONS\.recordView[\s\S]*DATABASE_PERMISSION_ACTIONS\.importAnalyze[\s\S]*DATABASE_PERMISSION_ACTIONS\.tablePromptView[\s\S]*DATABASE_PERMISSION_ACTIONS\.aiQueryRead/,
  'database table metadata permission group should include schema, record, import, table prompt, and AI-query readers'
);
assert.match(
  permissionConstantsSource,
  /export const KNOWLEDGE_BASE_READ_PERMISSION_CODES = \[[\s\S]*KNOWLEDGE_BASE_PERMISSION_ACTIONS\.folderView[\s\S]*KNOWLEDGE_BASE_PERMISSION_ACTIONS\.documentView[\s\S]*KNOWLEDGE_BASE_PERMISSION_ACTIONS\.segmentView[\s\S]*KNOWLEDGE_BASE_PERMISSION_ACTIONS\.graphView[\s\S]*KNOWLEDGE_BASE_PERMISSION_ACTIONS\.indexManage/,
  'knowledge base runtime binding read group should mirror backend readable knowledge permissions'
);
assert.doesNotMatch(
  sourceSliceBetween(
    permissionConstantsSource,
    'export const KNOWLEDGE_BASE_READ_PERMISSION_CODES = [',
    '] as const satisfies readonly PermissionCode[];'
  ),
  /KNOWLEDGE_BASE_PERMISSION_ACTIONS\.(?:create|retrievalTest)\b/,
  'knowledge base runtime binding read group should not include pure create or retrieval-test permissions'
);
const permissionAllCodesSource = sourceSliceBetween(
  permissionConstantsSource,
  'export const ALL_PERMISSION_CODES = [',
  '] as const;'
);
const permissionActionMatrixSource = sourceSliceBetween(
  permissionConstantsSource,
  'export const AGENT_PERMISSION_ACTIONS = {',
  'const permissionItem = (code: PermissionCode)'
);
const frontendSourceFiles = listFiles(path.join(rootDir, 'src')).filter(filePath =>
  /\.(?:ts|tsx)$/.test(filePath)
);
const permissionActionAggregateNames = [
  'AGENT_MANAGE_PERMISSION_CODES',
  'WORKFLOW_MANAGE_PERMISSION_CODES',
  'KNOWLEDGE_BASE_MANAGE_PERMISSION_CODES',
  'DATABASE_MANAGE_PERMISSION_CODES',
  'FILE_MANAGE_PERMISSION_CODES',
];
const retiredPermissionPrefixes = ['workspace.', 'prompt.', 'content_parse.', 'dashboard.'];
const legacyAggregatePermissionCodes = [
  'agent.view',
  'agent.manage',
  'knowledge_base.view',
  'knowledge_base.manage',
  'database.view',
  'database.manage',
  'database.data_edit',
  'database.ai_query',
  'file.view',
  'file.manage',
  'file.upload_create',
  'file.move_create',
];
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
assert.match(
  permissionConstantsSource,
  /COMPATIBILITY_PERMISSION_CODES[\s\S]*'database\.data_edit'[\s\S]*'database\.ai_query'[\s\S]*'file\.upload_create'[\s\S]*'file\.move_create'/,
  'legacy aggregate permissions should be declared compatibility-only'
);
assert.match(
  permissionConstantsSource,
  /!COMPATIBILITY_PERMISSION_CODE_VALUES\.has\(code\)/,
  'selectable permission list should exclude compatibility-only aggregate permissions'
);
assert.match(
  permissionConstantsSource,
  /!isRetiredWorkspacePermissionCode\(code\)/,
  'selectable permission list should exclude retired workspace tool/governance permission prefixes'
);
assert.match(
  permissionConstantsSource,
  /COMPATIBILITY_PERMISSION_EXPANSIONS[\s\S]*'database\.data_edit'[\s\S]*'database\.record\.create'[\s\S]*'database\.record\.update'[\s\S]*'database\.record\.delete'[\s\S]*'database\.import\.execute'[\s\S]*'database\.import\.errors\.view'/,
  'role/member permission normalization should preserve database.data_edit by expanding it to exact action permissions'
);
assert.match(
  permissionConstantsSource,
  /COMPATIBILITY_PERMISSION_CODE_VALUES\.has\(permission\)[\s\S]*COMPATIBILITY_PERMISSION_EXPANSIONS/,
  'role/member permission normalization should expand compatibility-only aggregate permissions before saving'
);
assert.doesNotMatch(
  permissionConstantsSource,
  /legacyDataEdit|legacyAiQuery|uploadCreate|moveCreate/,
  'frontend action matrix should not expose compatibility-only aggregate permissions as action groups'
);
for (const code of collectStringLiterals(permissionAllCodesSource)) {
  assert.ok(
    !retiredPermissionPrefixes.some(prefix => code.startsWith(prefix)),
    `ALL_PERMISSION_CODES should not contain retired or governance permission ${code}`
  );
}
for (const code of legacyAggregatePermissionCodes) {
  assert.doesNotMatch(
    permissionActionMatrixSource,
    new RegExp(`['"]${regexpEscape(code)}['"]`),
    `frontend action matrix should not use legacy aggregate permission ${code}`
  );
}
for (const filePath of frontendSourceFiles) {
  if (filePath === permissionConstantsPath) continue;
  const fileSource = fs.readFileSync(filePath, 'utf8');
  const stringLiterals = collectStringLiterals(fileSource);
  for (const aggregateName of permissionActionAggregateNames) {
    assert.doesNotMatch(
      fileSource,
      new RegExp(`\\b${aggregateName}\\b`),
      `${path.relative(rootDir, filePath)} should not use aggregate manage permission group ${aggregateName} for UI actions`
    );
  }
  for (const code of legacyAggregatePermissionCodes) {
    assert.equal(
      stringLiterals.includes(code),
      false,
      `${path.relative(rootDir, filePath)} should not consume legacy aggregate permission ${code} outside the compatibility constants`
    );
  }
  assert.doesNotMatch(
    fileSource,
    /has(?:Any|All)?Permission\s*\([\s\S]{0,240}['"](?:workspace|prompt|content_parse|dashboard)\./,
    `${path.relative(rootDir, filePath)} should not gate ordinary UI with retired workspace governance/tool permissions`
  );
}

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
const agentRuntimePageModelSource = fs.readFileSync(agentRuntimePageModelPath, 'utf8');
const agentRuntimePromptPanelSource = fs.readFileSync(agentRuntimePromptPanelPath, 'utf8');
const agentRuntimeOrchestrationPanelSource = fs.readFileSync(
  agentRuntimeOrchestrationPanelPath,
  'utf8'
);
const agentRuntimeDatabaseSectionSource = fs.readFileSync(
  agentRuntimeDatabaseSectionPath,
  'utf8'
);
const agentRuntimeKnowledgeSectionSource = fs.readFileSync(
  agentRuntimeKnowledgeSectionPath,
  'utf8'
);
const workflowDatabasePickerSource = fs.readFileSync(workflowDatabasePickerPath, 'utf8');
const workflowCallDatabaseManagerSource = fs.readFileSync(
  workflowCallDatabaseManagerPath,
  'utf8'
);
const workflowCallDatabaseInsertMenusSource = fs.readFileSync(
  workflowCallDatabaseInsertMenusPath,
  'utf8'
);
const workflowCallDatabaseExpandedDialogSource = fs.readFileSync(
  workflowCallDatabaseExpandedDialogPath,
  'utf8'
);
const workflowSqlGeneratorManagerSource = fs.readFileSync(
  workflowSqlGeneratorManagerPath,
  'utf8'
);
const workflowKnowledgeRetrievalManagerSource = fs.readFileSync(
  workflowKnowledgeRetrievalManagerPath,
  'utf8'
);
const workflowKnowledgeRecallSettingsSource = fs.readFileSync(
  workflowKnowledgeRecallSettingsPath,
  'utf8'
);

assert.match(
  consolePageSource,
  /const canOpenModelConfig\s*=\s*canAccessOrganizationDashboard && canManageModelConfig/,
  'console overview model configuration entry should require organization dashboard and model-config capabilities'
);
assert.doesNotMatch(
  consolePageSource,
  /has(?:Any|All)?Permission\s*\([\s\S]{0,240}['"](?:dashboard|content_parse)\./,
  'console overview should not gate model readiness or parser/dashboard entry points with retired workspace permissions'
);
const agentsPageSource = fs.readFileSync(agentsPagePath, 'utf8');
const createAgentDialogSource = fs.readFileSync(createAgentDialogPath, 'utf8');
const agentCardSource = fs.readFileSync(agentCardPath, 'utf8');
const agentEntryPageSource = fs.readFileSync(agentEntryPagePath, 'utf8');
const agentRuntimePageSource = fs.readFileSync(agentRuntimePagePath, 'utf8');
const agentLayoutSource = fs.readFileSync(agentLayoutPath, 'utf8');
const datasetPageSource = fs.readFileSync(datasetPagePath, 'utf8');
const datasetCardSource = fs.readFileSync(datasetCardPath, 'utf8');
const datasetFolderCardSource = fs.readFileSync(datasetFolderCardPath, 'utf8');
const datasetHooksSource = fs.readFileSync(datasetHooksPath, 'utf8');
const datasetHitResultItemSource = fs.readFileSync(datasetHitResultItemPath, 'utf8');
const datasetDetailRootPageSource = fs.readFileSync(datasetDetailRootPagePath, 'utf8');
const datasetDetailLayoutSource = fs.readFileSync(datasetDetailLayoutPath, 'utf8');
const templateGalleryDialogSource = fs.readFileSync(templateGalleryDialogPath, 'utf8');
const createFromTemplateHookSource = fs.readFileSync(createFromTemplateHookPath, 'utf8');
const agentSidebarSource = fs.readFileSync(agentSidebarPath, 'utf8');
const agentApiPageSource = fs.readFileSync(agentApiPagePath, 'utf8');
const workflowEditorPageSource = fs.readFileSync(workflowEditorPagePath, 'utf8');
const agentBatchTestPageSource = fs.readFileSync(agentBatchTestPagePath, 'utf8');
const agentBatchTestBatchesPageSource = fs.readFileSync(
  agentBatchTestBatchesPagePath,
  'utf8'
);
const agentBatchTestNewBatchPageSource = fs.readFileSync(
  agentBatchTestNewBatchPagePath,
  'utf8'
);
const agentBatchTestBatchPageSource = fs.readFileSync(agentBatchTestBatchPagePath, 'utf8');
const agentBatchTestBatchItemPageSource = fs.readFileSync(
  agentBatchTestBatchItemPagePath,
  'utf8'
);
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
  agentRuntimeHeaderSource,
  /const canManageRuntimeAccess\s*=\s*!disablePublishSettingsActions/,
  'agent runtime header should derive runtime access controls separately from publish controls'
);
assert.match(
  agentRuntimeHeaderSource,
  /disabled=\{!canUsePublishDropdown\}/,
  'agent runtime publish dropdown trigger should remain usable for runtime access settings or webapp links without publish permission'
);
assert.match(
  agentRuntimeHeaderSource,
  /disabled=\{!canPublish \|\| isPublishing \|\| saveState === 'saving'\}/,
  'agent runtime publish action should remain gated by agent.publish'
);
assert.match(
  agentRuntimeHeaderSource,
  /if \(!canManageRuntimeAccess\) \{[\s\S]*?return;[\s\S]*?\}/,
  'agent runtime webapp status mutation should require runtime access management'
);
assert.match(
  agentRuntimePageModelSource,
  /const canOpenAgentRuntimeEditor[\s\S]*canCreateAgent[\s\S]*canImportAgent[\s\S]*canUpdateAgent[\s\S]*canConfigureAgentRuntime[\s\S]*canPublishAgent[\s\S]*canManageAgentRuntimeAccess/,
  'agent runtime page model should require an editor-related agent permission before opening the runtime editor'
);
assert.match(
  agentRuntimePageModelSource,
  /useAgent\(agentId,\s*canOpenAgentRuntimeEditor\)/,
  'agent runtime page model should not fetch agent metadata before editor access is present'
);
assert.match(
  agentRuntimePageModelSource,
  /useAgentConfig\(\s*agentId,\s*canOpenAgentRuntimeEditor\s*\)/,
  'agent runtime page model should not fetch runtime config before editor access is present'
);
assert.match(
  agentRuntimePageSource,
  /if \(!model\.canOpenAgentRuntimeEditor\) \{[\s\S]*<PermissionDeniedState \/>/,
  'agent runtime direct page should deny direct access without editor-related agent permission'
);
assert.match(
  agentRuntimePageModelSource,
  /const isRuntimeConfigReadOnly\s*=\s*isVersionPreviewing \|\| !canConfigureAgentRuntime/,
  'agent runtime page model should derive a read-only state from runtime config permission'
);
assert.match(
  agentRuntimePageModelSource,
  /prompt:\s*\{[\s\S]*?readOnly:\s*isRuntimeConfigReadOnly/,
  'agent runtime page model should pass read-only state to the prompt panel'
);
assert.match(
  agentRuntimePageModelSource,
  /orchestration:\s*\{[\s\S]*?readOnly:\s*isRuntimeConfigReadOnly/,
  'agent runtime page model should pass read-only state to orchestration controls'
);
assert.match(
  agentRuntimePromptPanelSource,
  /<WorkflowValueEditor[\s\S]*?readOnly=\{readOnly\}/,
  'agent prompt editor should become read-only without runtime config permission'
);
assert.match(
  agentRuntimeOrchestrationPanelSource,
  /<AgentRuntimeModelSection[\s\S]*?readOnly=\{readOnly\}/,
  'agent orchestration panel should forward read-only state into runtime sections'
);
assert.match(
  agentRuntimePageModelSource,
  /const canBindKnowledge\s*=\s*hasAnyPermission\(KNOWLEDGE_BASE_READ_PERMISSION_CODES\)/,
  'agent runtime page model should derive knowledge binding access from the knowledge readable group'
);
assert.match(
  agentRuntimePageModelSource,
  /enabled:\s*Boolean\(datasetId\) && canBindKnowledge/,
  'agent runtime selected knowledge detail queries should not run without knowledge binding access'
);
assert.match(
  agentRuntimePageModelSource,
  /enabled:\s*knowledgeDialogOpen && canBindKnowledge/,
  'agent runtime knowledge selector should not list candidates without knowledge binding access'
);
assert.match(
  agentRuntimePageModelSource,
  /!canBindKnowledge[\s\S]*t\('knowledge\.bindingPermissionRequired'\)/,
  'agent runtime selected knowledge fallbacks should show a permission warning when binding access is missing'
);
assert.match(
  agentRuntimeOrchestrationPanelSource,
  /<AgentRuntimeKnowledgeSection[\s\S]*?canBindKnowledge=\{canBindKnowledge\}/,
  'agent orchestration panel should pass knowledge binding access to the knowledge section'
);
assert.match(
  agentRuntimeKnowledgeSectionSource,
  /readOnly=\{readOnly \|\| !canBindKnowledge\}/,
  'agent runtime knowledge add action should be disabled without knowledge binding access'
);
assert.match(
  agentRuntimeKnowledgeSectionSource,
  /disabled=\{readOnly \|\| !canBindKnowledge\}/,
  'agent runtime selected knowledge mutation controls should stay disabled without knowledge binding access'
);
assert.match(
  workflowKnowledgeRetrievalManagerSource,
  /useKnowledgeNodePermissions\(\)/,
  'workflow knowledge-retrieval manager should consume the shared knowledge node permission helper'
);
assert.match(
  workflowKnowledgeRetrievalManagerSource,
  /enabled:\s*canReadKnowledgeBinding/,
  'workflow knowledge-retrieval dataset candidates should not load without knowledge readable permissions'
);
assert.match(
  workflowKnowledgeRetrievalManagerSource,
  /const canEditKnowledgeSelection\s*=\s*!readOnly && canReadKnowledgeBinding/,
  'workflow knowledge-retrieval dataset selection should require workflow edit and knowledge readable permissions'
);
assert.match(
  workflowKnowledgeRetrievalManagerSource,
  /<RecallSettingsDialog id=\{nodeId\} readOnly=\{readOnly\}/,
  'workflow knowledge-retrieval recall settings should receive the workflow read-only state'
);
assert.match(
  workflowKnowledgeRetrievalManagerSource,
  /enabled:[\s\S]*!readOnly[\s\S]*reranking_enable[\s\S]*reranking_mode === 'reranking_model'/,
  'workflow knowledge-retrieval default rerank initialization should not mutate read-only workflows'
);
assert.match(
  workflowKnowledgeRecallSettingsSource,
  /if \(readOnly \|\| !nodeData\) return;/,
  'workflow knowledge-retrieval recall settings mutations should no-op in read-only mode'
);
assert.match(
  workflowKnowledgeRecallSettingsSource,
  /disabled=\{readOnly\}/,
  'workflow knowledge-retrieval recall settings controls should be disabled in read-only mode'
);
assert.match(
  agentRuntimeDatabaseSectionSource,
  /const \{ hasAnyPermission, hasAllPermissions \} = useAccountPermissions\(\)/,
  'agent runtime database section should be able to require both database read-binding permissions'
);
assert.match(
  permissionConstantsSource,
  /DATABASE_READ_BINDING_PERMISSION_CODES[\s\S]*DATABASE_PERMISSION_ACTIONS\.aiQueryRead[\s\S]*DATABASE_PERMISSION_ACTIONS\.recordView/,
  'shared database binding permission group should require both database.ai_query.read and database.record.view'
);
assert.match(
  agentRuntimeDatabaseSectionSource,
  /const canBindReadableDatabase\s*=\s*hasAllPermissions\(DATABASE_READ_BINDING_PERMISSION_CODES\)/,
  'agent runtime database binding should consume the shared database read-binding permission group'
);
assert.match(
  agentRuntimeDatabaseSectionSource,
  /useDbsBasic\([\s\S]*enabled:\s*open && canBindReadableDatabase/,
  'agent runtime database selector should not load database candidates without binding read permissions'
);
assert.match(
  agentRuntimeDatabaseSectionSource,
  /readOnly=\{readOnly \|\| !canBindReadableDatabase\}/,
  'agent runtime database add action should be disabled without binding read permissions'
);
assert.match(
  agentRuntimeDatabaseSectionSource,
  /useDbTables\(dataSourceID,[\s\S]*enabled:\s*canReadBinding && tableIDs\.length > 0/,
  'agent runtime selected database cards should not load table metadata without binding read permissions'
);
assert.match(
  agentRuntimeDatabaseSectionSource,
  /disabled=\{readOnly \|\| cannotReadBinding\}/,
  'agent runtime selected database mutation controls should stay disabled without binding read permissions'
);
assert.match(
  workflowDatabasePickerSource,
  /useDatabaseNodePermissions\(\)/,
  'workflow database picker should consume the shared database node permission helper'
);
assert.match(
  workflowDatabasePickerSource,
  /enabled:\s*open && canBrowseDatabaseMetadata/,
  'workflow database picker should not list database candidates without read-binding permissions'
);
assert.match(
  workflowDatabasePickerSource,
  /enabled:\s*Boolean\(dbId\) && canBrowseDatabaseMetadata/,
  'workflow database picker should not list tables without read-binding permissions'
);
assert.match(
  workflowDatabasePickerSource,
  /enabled:\s*expanded && canBrowseDatabaseMetadata/,
  'workflow database picker should not list table columns without read-binding permissions'
);
assert.match(
  workflowCallDatabaseManagerSource,
  /const canEditDatabaseSource\s*=\s*!readOnly && canReadDatabaseBinding/,
  'workflow call-database source picker should require workflow edit and database read-binding permissions'
);
assert.match(
  workflowCallDatabaseManagerSource,
  /readOnly=\{!canEditDatabaseSource\}/,
  'workflow call-database picker should be read-only when database read-binding permissions are absent'
);
assert.match(
  workflowCallDatabaseInsertMenusSource,
  /enabled:\s*Boolean\(dbId\) && canBrowseDatabaseMetadata/,
  'workflow SQL insert table menu should not fetch tables without database read-binding permissions'
);
assert.match(
  workflowCallDatabaseInsertMenusSource,
  /canBrowseDatabaseMetadata && dbId && table\.id && \(forcedOpen \|\| expanded\)/,
  'workflow SQL insert column menu should not fetch columns without database read-binding permissions'
);
assert.match(
  workflowCallDatabaseExpandedDialogSource,
  /enabled:\s*Boolean\(dbId && open && canReadDatabaseBinding\)/,
  'workflow expanded SQL editor should not fetch tables without database read-binding permissions'
);
assert.match(
  workflowCallDatabaseExpandedDialogSource,
  /canBrowseDatabaseMetadata && dbId && table\.id && expanded/,
  'workflow expanded SQL editor should not fetch columns without database read-binding permissions'
);
assert.match(
  workflowSqlGeneratorManagerSource,
  /enabled:\s*Boolean\(canReadDatabaseBinding && \(pendingSelection \|\| pickerOpen\) && currentDbId\)/,
  'workflow SQL generator should not enrich table metadata without database read-binding permissions'
);
assert.match(
  workflowSqlGeneratorManagerSource,
  /readOnly=\{!canEditDatabaseSource\}/,
  'workflow SQL generator picker should be read-only when database read-binding permissions are absent'
);
assert.match(
  agentsPageSource,
  /const canCreateAgent\s*=\s*hasAnyPermission\(AGENT_PERMISSION_ACTIONS\.create\)/,
  'agent list blank-create entry should check agent.create explicitly'
);
assert.match(
  agentsPageSource,
  /const canCreateWorkflow\s*=\s*hasAnyPermission\(WORKFLOW_PERMISSION_ACTIONS\.create\)/,
  'agent list blank-create entry should check workflow.create explicitly'
);
assert.match(
  agentsPageSource,
  /const canImportWorkflow\s*=\s*hasAnyPermission\(WORKFLOW_PERMISSION_ACTIONS\.import\)/,
  'agent list import/template entry should use workflow.import because the dialog calls workflow import'
);
assert.doesNotMatch(
  agentsPageSource,
  /hasPermission\(['"]agent\.import['"]\)/,
  'agent list should not gate workflow import UI with agent.import'
);
assert.match(
  templateGalleryDialogSource,
  /canCreateBlank/,
  'template gallery should hide blank creation when the user only has workflow import/template access'
);
assert.match(
  createAgentDialogSource,
  /canCreateAgent\s*=\s*hasAnyPermission\(AGENT_PERMISSION_ACTIONS\.create\)/,
  'create dialog should gate AGENT mode by agent.create'
);
assert.match(
  createAgentDialogSource,
  /canCreateWorkflow\s*=\s*hasAnyPermission\(WORKFLOW_PERMISSION_ACTIONS\.create\)/,
  'create dialog should gate workflow modes by workflow.create'
);
assert.match(
  createAgentDialogSource,
  /isAgentTypeAllowed/,
  'create dialog should validate the selected runtime type before submitting'
);
assert.match(
  createAgentDialogSource,
  /router\.push\(`\/console\/agents\/\$\{newId\}`\)/,
  'create dialog should route new agents through the permission-aware detail root'
);
assert.doesNotMatch(
  createAgentDialogSource,
  /\/console\/agents\/\$\{newId\}\/(?:agent|workflow)/,
  'create dialog should not bypass permission-aware child routing after creation'
);
assert.match(
  createFromTemplateHookSource,
  /router\.push\(`\/console\/agents\/\$\{agentId\}`\)/,
  'template-created workflows should route through the permission-aware detail root'
);
assert.doesNotMatch(
  createFromTemplateHookSource,
  /\/console\/agents\/\$\{agentId\}\/workflow/,
  'template-created workflows should not bypass permission-aware child routing'
);
assert.match(
  agentCardSource,
  /const agentHref = `\/console\/agents\/\$\{agent\.id\}`/,
  'agent cards should link to the permission-aware detail root instead of directly opening the editor'
);
assert.doesNotMatch(
  agentCardSource,
  /getAgentDetailEditHref/,
  'agent cards should not bypass permission-aware child routing by linking directly to the editor'
);
assert.match(
  agentCardSource,
  /const exportPermissionCodes = isWorkflowRuntime[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.export[\s\S]*:\s*\[\];/,
  'agent cards should expose YAML export only for workflow runtimes because the backend export endpoint requires workflow.export'
);
assert.doesNotMatch(
  agentCardSource,
  /AGENT_PERMISSION_ACTIONS\.export/,
  'agent cards should not wire agent.export to the workflow YAML export endpoint'
);
assert.match(
  datasetPageSource,
  /const canManage\s*=\s*hasAnyPermission\(KNOWLEDGE_BASE_PERMISSION_ACTIONS\.create\)/,
  'dataset list create action should use the knowledge-base create action group'
);
assert.doesNotMatch(
  datasetPageSource,
  /hasPermission\(['"]knowledge_base\.(?:create|folder_manage)['"]\)/,
  'dataset list should not bypass the knowledge-base action matrix with raw permission literals'
);
assert.match(
  datasetFolderCardSource,
  /const canManageFolders\s*=\s*hasAnyPermission\(KNOWLEDGE_BASE_PERMISSION_ACTIONS\.folderManage\)/,
  'dataset folder card edit/delete actions should use the folder manage action group'
);
assert.doesNotMatch(
  datasetFolderCardSource,
  /hasPermission\(['"]knowledge_base\.folder_manage['"]\)/,
  'dataset folder card should not bypass the knowledge-base action matrix with a raw folder permission literal'
);
assert.match(
  datasetCardSource,
  /const canMoveDataset = hasAnyPermission\(KNOWLEDGE_BASE_PERMISSION_ACTIONS\.move\)/,
  'dataset card workspace move should use knowledge_base.move'
);
assert.match(
  datasetCardSource,
  /const canManageDatasetFolders = hasAnyPermission\(KNOWLEDGE_BASE_PERMISSION_ACTIONS\.folderManage\)/,
  'dataset card folder move should check folder management capability'
);
assert.match(
  datasetCardSource,
  /const canMoveDatasetToFolder = canMoveDataset && canManageDatasetFolders/,
  'dataset card folder move should require both dataset move and folder manage permissions'
);
assert.match(
  datasetCardSource,
  /href=\{`\/console\/dataset\/\$\{dataset\.id\}`\}/,
  'dataset cards should link to the permission-aware detail root instead of always opening documents'
);
assert.doesNotMatch(
  datasetCardSource,
  /href=\{`\/console\/dataset\/\$\{dataset\.id\}\/documents`\}/,
  'dataset cards should not bypass permission-aware child routing by linking directly to documents'
);
assert.match(
  datasetHooksSource,
  /router\.push\(`\/console\/dataset\/\$\{response\.data\.id\}`\)/,
  'dataset creation should route through the permission-aware detail root'
);
assert.doesNotMatch(
  datasetHooksSource,
  /router\.push\(`\/console\/dataset\/\$\{response\.data\.id\}\/documents`\)/,
  'dataset creation should not assume the creator can open the documents child page'
);
assert.match(
  datasetHitResultItemSource,
  /hasAnyPermission\(KNOWLEDGE_BASE_PERMISSION_ACTIONS\.documentView\)/,
  'hit-testing document detail links should be gated by document view permission'
);
assert.match(
  datasetHitResultItemSource,
  /canViewDocumentDetails \? \([\s\S]*\/documents\/\$\{result\.segment\.document\.id\}/,
  'hit-testing result UI should only render document detail links when the document page can be opened'
);
assert.match(
  datasetCardSource,
  /canMoveDatasetToFolder[\s\S]*setMoveOpen\(true\)/,
  'dataset card folder move menu item should be gated by the combined folder move permission'
);
assert.match(
  datasetCardSource,
  /canMoveDataset[\s\S]*setWorkspaceMoveOpen\(true\)/,
  'dataset card workspace move menu item should remain gated by knowledge_base.move'
);
assert.doesNotMatch(
  datasetDetailRootPageSource,
  /from 'next\/navigation';[\s\S]*\bredirect\(/,
  'dataset detail root should not use a fixed server redirect to documents'
);
assert.match(
  datasetDetailRootPageSource,
  /if \(canViewDocuments\) return `\/console\/dataset\/\$\{datasetId\}\/documents`/,
  'dataset detail root should prefer documents when document access exists'
);
assert.match(
  datasetDetailRootPageSource,
  /if \(canUseRetrievalTest\) return `\/console\/dataset\/\$\{datasetId\}\/hit-testing`/,
  'dataset detail root should fall back to hit-testing for retrieval-test-only users'
);
assert.match(
  datasetDetailRootPageSource,
  /if \(canViewGraph && isGraphEnabled\) return `\/console\/dataset\/\$\{datasetId\}\/graph`/,
  'dataset detail root should route graph-only users to graph when graph flow is enabled'
);
assert.match(
  datasetDetailRootPageSource,
  /if \(canOpenSettings\) return `\/console\/dataset\/\$\{datasetId\}\/settings`/,
  'dataset detail root should route settings-only users to settings'
);
assert.match(
  datasetDetailRootPageSource,
  /router\.replace\(targetHref\)/,
  'dataset detail root should replace to the first permission-compatible child page'
);
assert.match(
  datasetDetailLayoutSource,
  /useDataset\(datasetId,\s*\{[\s\S]*enabled:\s*canView[\s\S]*refetchInterval:\s*10000/,
  'dataset detail layout should not fetch dataset metadata before visible knowledge-base permission is present'
);
assert.doesNotMatch(
  agentEntryPageSource,
  /function getAgentDefaultHref/,
  'agent detail root should not keep a local type-only default redirect helper'
);
assert.match(
  agentEntryPageSource,
  /getAgentDetailDefaultHref\(agentId,[\s\S]*canOpenEditor:/,
  'agent detail root should choose its default child page through the permission-aware route helper'
);
assert.match(
  agentEntryPageSource,
  /targetHrefWithSearch[\s\S]*searchParams\.toString\(\)[\s\S]*router\.replace\(targetHrefWithSearch\)/,
  'agent detail root should replace to the first permission-compatible child page while preserving query params'
);
assert.match(
  agentLayoutSource,
  /const canViewAnyAgentAsset\s*=\s*hasAnyPermission\(AGENT_ASSET_VISIBLE_PERMISSION_CODES\)[\s\S]*useAgent\(agentId,\s*canViewAnyAgentAsset\)/,
  'agent detail layout should not fetch agent metadata before an agent/workflow visible permission is present'
);
assert.match(
  agentEntryPageSource,
  /const canViewAnyAgentAsset\s*=\s*hasAnyPermission\(AGENT_ASSET_VISIBLE_PERMISSION_CODES\)[\s\S]*useAgent\(agentId,\s*canViewAnyAgentAsset\)/,
  'agent detail root should gate metadata fetches by the shared agent/workflow visible permission group'
);
assert.match(
  agentEntryPageSource,
  /const canCreateAgent\s*=\s*hasAnyPermission\(AGENT_PERMISSION_ACTIONS\.create\)[\s\S]*const canImportAgent\s*=\s*hasAnyPermission\(AGENT_PERMISSION_ACTIONS\.import\)[\s\S]*const canUpdateAgent\s*=\s*hasAnyPermission\(AGENT_PERMISSION_ACTIONS\.update\)/,
  'agent runtime root should derive create/import/update detail-entry permissions explicitly'
);
assert.match(
  agentEntryPageSource,
  /canOpenAgentRuntimeEditor[\s\S]*canCreateAgent[\s\S]*canImportAgent[\s\S]*canUpdateAgent[\s\S]*AGENT_PERMISSION_ACTIONS\.runtimeConfigManage[\s\S]*AGENT_PERMISSION_ACTIONS\.publish[\s\S]*AGENT_PERMISSION_ACTIONS\.runtimeAccessManage/,
  'agent runtime root should keep create/import/update users able to open the detail page'
);
assert.match(
  agentEntryPageSource,
  /const canCreateWorkflow\s*=\s*hasAnyPermission\(WORKFLOW_PERMISSION_ACTIONS\.create\)[\s\S]*const canImportWorkflow\s*=\s*hasAnyPermission\(WORKFLOW_PERMISSION_ACTIONS\.import\)/,
  'workflow root should derive create/import detail-entry permissions explicitly'
);
assert.match(
  agentEntryPageSource,
  /canOpenWorkflowEditor[\s\S]*canCreateWorkflow[\s\S]*canImportWorkflow[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.update[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.runDraft[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.runStop[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.debug[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.publish[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.runtimeConfigManage[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.runtimeAccessManage/,
  'workflow root should keep create/import users able to open the detail page'
);
assert.match(
  agentEntryPageSource,
  /canViewBatchTest:\s*isWorkflowRuntime && \(canViewWorkflowTestLibrary \|\| canViewWorkflowLogs\)/,
  'workflow root should expose batch-test child pages only through workflow.view or workflow.logs.view'
);
assert.doesNotMatch(
  agentEntryPageSource,
  /canViewBatchTest:\s*isWorkflowRuntime && \(canViewWorkflowTestLibrary \|\| canViewWorkflowLogs \|\| canRunWorkflowBatchTest\)/,
  'workflow root should not treat workflow.debug as batch-test page visibility'
);
assert.match(
  workflowEditorPageSource,
  /const canOpenWorkflowEditor[\s\S]*canCreateWorkflow[\s\S]*canImportWorkflow[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.update[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.runDraft[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.runStop[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.debug[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.publish[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.runtimeConfigManage[\s\S]*WORKFLOW_PERMISSION_ACTIONS\.runtimeAccessManage/,
  'workflow editor direct page should require an editor-related workflow permission'
);
assert.match(
  workflowEditorPageSource,
  /useAgent\(agentId,\s*canOpenWorkflowEditor\)/,
  'workflow editor direct page should not fetch agent metadata before editor access is present'
);
assert.match(
  workflowEditorPageSource,
  /if \(!canOpenWorkflowEditor\) \{[\s\S]*<PermissionDeniedState \/>/,
  'workflow editor direct page should deny direct access without editor-related workflow permission'
);
assert.match(
  workflowEditorPageSource,
  /supportsWorkflowDetailPages\(agent\.data\.agent_type\)/,
  'workflow editor direct page should only render the workflow editor for workflow runtimes'
);
assert.match(
  agentSidebarSource,
  /if \(routeAccess\.canShowEditor\)[\s\S]*agents\.actions\.edit/,
  'agent sidebar should only show the edit/editor entry when that child page is permission-compatible'
);
assert.match(
  agentSidebarSource,
  /canCreateAgent[\s\S]*canImportAgent[\s\S]*canUpdateAgent[\s\S]*canConfigureAgentRuntime[\s\S]*canPublishAgent[\s\S]*canManageAgentRuntimeAccess/,
  'agent sidebar should keep create/import/update users able to open the detail page'
);
assert.match(
  agentSidebarSource,
  /const canConfigureWorkflowRuntime\s*=\s*hasAnyPermission\(\s*WORKFLOW_PERMISSION_ACTIONS\.runtimeConfigManage\s*\)[\s\S]*canConfigureWorkflowRuntime[\s\S]*canManageWorkflowRuntimeAccess/,
  'agent sidebar should keep workflow runtime-config managers able to open the editor page'
);
assert.match(
  agentSidebarSource,
  /const canView\s*=\s*hasAnyPermission\(AGENT_ASSET_VISIBLE_PERMISSION_CODES\)[\s\S]*useAgent\(agentId,\s*canView\)/,
  'agent sidebar should not refetch agent metadata without the shared agent/workflow visible permission group'
);
assert.match(
  agentSidebarSource,
  /routeAccess\.canShowApiKeys/,
  'agent sidebar API entry should now be tied to workflow API key visibility'
);
assert.match(
  agentSidebarSource,
  /canViewBatchTest:/,
  'agent sidebar should distinguish workflow batch-test visibility from execution permission'
);
assert.match(
  agentSidebarSource,
  /batchTestHref\s*=\s*canViewWorkflowTestLibrary[\s\S]*batch-test[\s\S]*batch-test\/batches/,
  'agent sidebar should route logs-only batch-test users to the batches/results view'
);
assert.match(
  agentSidebarSource,
  /canViewBatchTest:\s*isWorkflowRuntime && \(canViewWorkflowTestLibrary \|\| canViewWorkflowTestBatches\)/,
  'agent sidebar should expose batch-test child pages only through workflow.view or workflow.logs.view'
);
assert.doesNotMatch(
  agentSidebarSource,
  /canViewBatchTest:\s*isWorkflowRuntime && \(canViewWorkflowTestLibrary \|\| canViewWorkflowTestBatches \|\| canRunWorkflowBatchTest\)/,
  'agent sidebar should not treat workflow.debug as batch-test page visibility'
);
assert.match(
  agentSidebarSource,
  /if\s*\(canViewWorkflowTestLibrary\)[\s\S]*subnav\.caseLibrary/,
  'agent sidebar should show the case-library batch-test child only with workflow.view'
);
assert.match(
  agentSidebarSource,
  /if\s*\(canViewWorkflowTestBatches\)[\s\S]*subnav\.batches/,
  'agent sidebar should show the batch results child only with workflow.logs.view'
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
assert.match(
  agentApiPageSource,
  /hasAnyPermission\(WORKFLOW_PERMISSION_ACTIONS\.runtimeAccessManage\)/,
  'agent API page should require workflow runtime-access management for workflow API key/docs tabs'
);
assert.doesNotMatch(
  agentApiPageSource,
  /AGENT_PERMISSION_ACTIONS\.runtimeAccessManage/,
  'agent API page should not mix AGENT runtime-access permissions into the workflow-only API key/docs route'
);
assert.match(
  agentApiPageSource,
  /canShowAgentApiKeys\(agentType,[\s\S]*canManageRuntimeAccess/,
  'agent API page should delegate workflow API-key visibility through the shared route helper'
);
assert.match(
  agentApiPageSource,
  /useAgent\(agentId,\s*canManageRuntimeAccess\)/,
  'agent API direct page should not fetch agent metadata before workflow runtime-access permission is present'
);
assert.match(
  agentLogsPageSource,
  /hasAnyPermission\(AGENT_PERMISSION_ACTIONS\.logsView\)/,
  'agent logs page should use agent.logs.view for AGENT runtime logs'
);
assert.match(
  agentLogsPageSource,
  /hasAnyPermission\(WORKFLOW_PERMISSION_ACTIONS\.logsView\)/,
  'agent logs page should use workflow.logs.view for workflow runtime logs'
);
assert.match(
  agentLogsPageSource,
  /const canOpenRuntimeLogs\s*=\s*canViewAgentRuntimeLogs \|\| canViewWorkflowRuntimeLogs[\s\S]*useAgent\(agentId,\s*canOpenRuntimeLogs\)/,
  'agent logs direct page should not fetch agent metadata before an AGENT or workflow logs permission is present'
);
assert.match(
  agentLogsPageSource,
  /const canQueryWorkflowLogs\s*=\s*canAccessRuntimeLogs && isPublished && !isAgentRuntime/,
  'agent logs page should only query workflow run logs after workflow log access and publication are confirmed'
);
assert.match(
  agentLogsPageSource,
  /const canQueryAgentRuntimeLogs\s*=\s*canAccessRuntimeLogs && isPublished && isAgentRuntime/,
  'agent logs page should only query AGENT runtime logs after agent log access and publication are confirmed'
);
assert.match(
  agentLogsPageSource,
  /useWorkflowRunsInfinite\([\s\S]*agentId:\s*canQueryWorkflowLogs\s*\?\s*agentId\s*:\s*null[\s\S]*enabled:\s*canQueryWorkflowLogs/,
  'agent logs page should gate workflow log list requests with workflow.logs.view'
);
assert.match(
  agentLogsPageSource,
  /useAgentRuntimeRunsInfinite\([\s\S]*agentId:\s*canQueryAgentRuntimeLogs\s*\?\s*agentId\s*:\s*null[\s\S]*enabled:\s*canQueryAgentRuntimeLogs/,
  'agent logs page should gate AGENT runtime log list requests with agent.logs.view'
);
assert.match(
  agentLogsPageSource,
  /useWorkflowRunDetail\([\s\S]*agentId:\s*canQueryWorkflowLogs\s*\?\s*agentId\s*:\s*null[\s\S]*enabled:\s*Boolean\(canQueryWorkflowLogs && isDetailOpen && effectiveRunId\)/,
  'agent logs page should gate workflow log detail requests with workflow.logs.view'
);
assert.match(
  agentLogsPageSource,
  /useAgentRuntimeRunDetail\([\s\S]*agentId:\s*canQueryAgentRuntimeLogs\s*\?\s*agentId\s*:\s*null[\s\S]*enabled:\s*Boolean\(canQueryAgentRuntimeLogs && isDetailOpen && effectiveRunId\)/,
  'agent logs page should gate AGENT runtime log detail requests with agent.logs.view'
);
assert.match(
  agentBatchTestPageSource,
  /const canOpenBatchTestOverview\s*=\s*canViewBatchTestLibrary[\s\S]*useAgent\(agentId,\s*canOpenBatchTestOverview\)/,
  'workflow batch-test overview direct page should fetch agent metadata only with workflow.view'
);
assert.match(
  agentBatchTestBatchesPageSource,
  /const canOpenBatchResults\s*=\s*canViewBatchTestLogs[\s\S]*useAgent\(agentId,\s*canOpenBatchResults\)/,
  'workflow batch-test batches direct page should fetch agent metadata only with workflow.logs.view'
);
assert.match(
  agentBatchTestNewBatchPageSource,
  /const canCreateAndRunBatch\s*=\s*canViewBatchTestLibrary && canViewBatchTestLogs && canUpdateBatchTest && canDebugBatchTest[\s\S]*useAgent\(agentId,\s*canCreateAndRunBatch\)/,
  'workflow batch-test new-batch direct page should require the full create-and-run permission set before fetching metadata'
);
assert.match(
  agentBatchTestBatchPageSource,
  /const canOpenBatchResult\s*=\s*canViewBatchTestLogs[\s\S]*useAgent\(agentId,\s*canOpenBatchResult\)/,
  'workflow batch-test result direct page should fetch agent metadata only with workflow.logs.view'
);
assert.match(
  agentBatchTestBatchItemPageSource,
  /const canOpenBatchResult\s*=\s*canViewBatchTestLogs[\s\S]*useAgent\(agentId,\s*canOpenBatchResult\)/,
  'workflow batch-test result item direct page should fetch agent metadata only with workflow.logs.view'
);
assert.match(
  workflowEditorSource,
  /const canPublishCurrentDraft\s*=\s*canEditWorkflow && canPublishWorkflow/,
  'workflow editor publish action should require workflow.publish plus workflow.update because publishing first saves the current draft'
);
assert.match(
  workflowEditorSource,
  /canPublishWorkflow=\{canPublishCurrentDraft\}/,
  'workflow header should receive the combined publish-current-draft permission'
);
assert.match(
  workflowEditorSource,
  /canEditDraft:\s*canEditWorkflow/,
  'workflow lifecycle should receive workflow.update permission instead of relying on store default edit state'
);
assert.match(
  workflowEditorSource,
  /isPermissionLoading:\s*isPermissionLoading \|\| isPermissionFetching/,
  'workflow lifecycle should wait for permission resolution before treating missing update permission as read-only'
);
assert.match(
  workflowLifecycleSource,
  /if \(!canEditDraft\) \{[\s\S]*if \(isPermissionLoading\) return;[\s\S]*loadWorkflow\(initialWorkflowData, agentId, false\);/,
  'workflow lifecycle should not bootstrap a draft graph for read-only viewers before permissions resolve'
);
assert.match(
  workflowLifecycleSource,
  /if \(!canEditDraft\) return;[\s\S]*hasNormalizedDefaultPromptsRef/,
  'workflow lifecycle should not normalize default prompts in read-only workflow detail'
);
assert.match(
  workflowLifecycleSource,
  /if \(!canEditDraft\) return;[\s\S]*hasInitializedModelsRef/,
  'workflow lifecycle should not inject default node models in read-only workflow detail'
);
assert.match(
  workflowNodeDataUpdateHookSource,
  /if \(storeState\.mode === 'history' \|\| !storeState\.canEdit\) return;/,
  'workflow node data update helper should not mutate node data without workflow.update'
);
assert.match(
  workflowOperationsHookSource,
  /const isWorkflowStoreReadOnly = \(\) => \{[\s\S]*return mode === 'history' \|\| !canEdit;/,
  'workflow operations helper should share the same history/permission read-only predicate'
);
assert.match(
  workflowOperationsHookSource,
  /if \(isWorkflowStoreReadOnly\(\)\) return null;[\s\S]*let finalData/,
  'workflow node creation operations should not add nodes without workflow.update'
);
assert.match(
  workflowOperationsHookSource,
  /const deleteNodeSafe = useCallback\([\s\S]*if \(isWorkflowStoreReadOnly\(\)\) return false;/,
  'workflow delete operation should not remove nodes without workflow.update'
);
assert.match(
  workflowOperationsHookSource,
  /const pasteClipboardAtPointer = useCallback\(\(\) => \{[\s\S]*if \(isWorkflowStoreReadOnly\(\)\) return;/,
  'workflow paste operation should not mutate the graph without workflow.update'
);
assert.match(
  workflowOperationsHookSource,
  /const duplicateNode = useCallback\([\s\S]*if \(isWorkflowStoreReadOnly\(\)\) return;/,
  'workflow duplicate operation should not mutate the graph without workflow.update'
);
assert.match(
  workflowOperationsHookSource,
  /const handleResetWorkflow = useCallback\(\(\) => \{[\s\S]*if \(isWorkflowStoreReadOnly\(\)\) return;/,
  'workflow reset operation should not mutate the graph without workflow.update'
);
assert.match(
  workflowCanvasWithDndSource,
  /<GlobalContainerOverlay isReadOnly=\{isReadOnly\} \/>/,
  'workflow canvas should pass read-only state to container drop overlays'
);
assert.match(
  workflowGlobalContainerOverlaySource,
  /if \(isReadOnly \|\| isNestingBlocked\) \{/,
  'workflow container drop overlay should not create nested nodes in read-only mode'
);
assert.match(
  workflowGlobalContainerOverlaySource,
  /if \(isReadOnly \|\| !draggingNodeType\) return null;/,
  'workflow container drop overlay should not render active drop targets in read-only mode'
);
assert.match(
  workflowCustomHandleSource,
  /const isReadOnly = isHistory \|\| !canEdit;/,
  'workflow custom handles should derive read-only state from permission edit authority as well as history mode'
);
assert.match(
  workflowCustomHandleSource,
  /onClick=\{!isReadOnly && type === 'source' \? handleClick : undefined\}/,
  'workflow custom handles should not open create-node modal without workflow.update'
);
assert.match(
  workflowContainerNodeSource,
  /const isReadOnly = mode === 'history' \|\| !canEdit;/,
  'workflow container nodes should derive read-only state from permission edit authority as well as history mode'
);
assert.match(
  workflowContainerNodeSource,
  /\{onlyHasStart && !isReadOnly && \(/,
  'workflow container empty-state add button should not render without workflow.update'
);
assert.match(
  workflowNodeResizeHandleSource,
  /const isReadOnly = mode === 'history' \|\| !canEdit;/,
  'workflow manual resize handles should derive read-only state from permission edit authority as well as history mode'
);
assert.match(
  workflowNodeResizeHandleSource,
  /if \(isReadOnly\) return;/,
  'workflow manual resize handles should not update node dimensions without workflow.update'
);
assert.match(
  workflowAutoDimensionsSyncSource,
  /const canEdit = useWorkflowStore\.use\.canEdit\(\);/,
  'workflow auto dimension sync should read workflow.update edit authority'
);
assert.match(
  workflowAutoDimensionsSyncSource,
  /const isReadOnly = mode === 'history' \|\| !canEdit;/,
  'workflow auto dimension sync should derive read-only state from permission edit authority as well as history mode'
);
assert.match(
  workflowAutoDimensionsSyncSource,
  /if \(isReadOnly\) return;/,
  'workflow auto dimension sync should not re-measure and persist node dimensions without workflow.update'
);
assert.match(
  workflowNoteNodeSource,
  /const isReadOnly = mode === 'history' \|\| !canEdit;/,
  'workflow note nodes should derive read-only state from permission edit authority as well as history mode'
);
assert.match(
  workflowNoteNodeSource,
  /disabled=\{isReadOnly\}/,
  'workflow note textarea should be disabled without workflow.update'
);
assert.match(
  workflowNoteNodeSource,
  /\{selected && !isReadOnly && <ManualResizeHandle/,
  'workflow note resize handle should not render without workflow.update'
);
assert.match(
  workflowCreateNodeModalHostSource,
  /if \(open && isReadOnly\) \{[\s\S]*closeModal\(\);[\s\S]*\}/,
  'workflow create-node modal host should close stale creation modals when workflow becomes read-only'
);
assert.match(
  workflowCreateNodeModalSource,
  /isReadOnly,\s*\n\s*\}\);/,
  'workflow create-node modal should pass read-only state into creation actions'
);
assert.match(
  workflowCreationActionsSource,
  /const isWorkflowCreationReadOnly = \(\) => \{[\s\S]*return mode === 'history' \|\| !canEdit;/,
  'workflow creation actions should re-check current store edit authority before mutating the graph'
);
assert.match(
  workflowCreationActionsSource,
  /if \(isReadOnly \|\| isWorkflowCreationReadOnly\(\)\) \{[\s\S]*onClose\(\);[\s\S]*return;/,
  'workflow creation actions should reject modal selections in read-only mode'
);
assert.match(
  workflowContextMenuSource,
  /const effectiveDisabled = disabled \|\| isReadOnly;/,
  'workflow context menu should combine parent disabled state with workflow.update edit authority'
);
assert.match(
  workflowContextMenuSource,
  /const handleAddNote = useCallback\(\(\) => \{[\s\S]*if \(effectiveDisabled\) return;[\s\S]*addNode\(\{ type: 'note', text: '' \}, position\);/,
  'workflow context menu add-note action should not add nodes without workflow.update'
);
assert.match(
  workflowContextMenuSource,
  /\{!effectiveDisabled && \(isCanvasMenuOpen \|\| contextNodeId\) && \(/,
  'workflow context menu should not render action menu without workflow.update'
);
assert.match(
  workflowBottomToolbarSource,
  /const isReadOnly = mode === 'history' \|\| !canEdit;/,
  'workflow bottom toolbar should derive read-only state from permission edit authority as well as history mode'
);
assert.match(
  workflowBottomToolbarSource,
  /const canUndo = !isReadOnly && historyPast\.length > 0;/,
  'workflow bottom toolbar undo action should be disabled without workflow.update'
);
assert.match(
  workflowBottomToolbarSource,
  /const handleAddNote = \(\) => \{[\s\S]*if \(isReadOnly\) return;[\s\S]*addNode\(\{ type: 'note', text: '' \}, center\);/,
  'workflow bottom toolbar add-note action should not add nodes without workflow.update'
);
assert.match(
  workflowKeyboardHookSource,
  /const isReadOnly = Boolean\(disabled \|\| mode === 'history' \|\| !canEdit\);/,
  'workflow keyboard shortcuts should derive read-only state from permission edit authority as well as caller disabled state'
);
assert.match(
  workflowKeyboardHookSource,
  /if \(isReadOnly\) return;[\s\S]*document\.addEventListener\('keydown', handleKeyDown\);/,
  'workflow keyboard shortcuts should not attach mutation shortcuts without workflow.update'
);
assert.match(
  workflowApprovalManagerSource,
  /if \(readOnly\) \{[\s\S]*pendingActionHandleUpdatesRef\.current = new Map\(\);[\s\S]*return;[\s\S]*\}/,
  'workflow approval action handle edge sync should not flush graph mutations in read-only mode'
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

for (const commonI18nPath of commonI18nPaths) {
  const commonI18nSource = fs.readFileSync(commonI18nPath, 'utf8');
  assert.doesNotMatch(
    commonI18nSource,
    /chats,\s*apps,\s*image generation|对话、应用、绘图/,
    `${path.relative(rootDir, commonI18nPath)} workspace-required copy should not describe organization product entries as workspace-only`
  );
}

for (const webAppI18nPath of webAppI18nPaths) {
  const webAppI18nSource = fs.readFileSync(webAppI18nPath, 'utf8');
  assert.doesNotMatch(
    webAppI18nSource,
    /not joined any workspace|未加入任何工作空间|Apps available in the current workspace|not runnable in your current workspace|当前工作空间可直接使用的应用|当前工作区下不可运行/,
    `${path.relative(rootDir, webAppI18nPath)} App Center and runtime copy should not block no-workspace app-center audiences by wording`
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
assert.equal(
  canShowAgentBatchTest('WORKFLOW', { canView: true, canViewBatchTest: true }),
  true,
  'Workflow batch-test area should be visible to read-only batch/result users'
);
assert.equal(
  canShowAgentBatchTest('WORKFLOW', { canView: true, canViewBatchTest: false, canRunBatchTest: true }),
  false,
  'Batch-test page visibility should not be widened by execution permission'
);
assert.equal(
  canShowAgentBatchTest('WORKFLOW', { canView: true, canRunBatchTest: true }),
  false,
  'Workflow batch-test area should not be visible with only execution permission'
);
const logsOnlyWorkflowRouteAccess = getAgentDetailRouteAccess('agent-1', 'WORKFLOW', {
  canView: true,
  canViewBatchTest: true,
  canRunBatchTest: false,
});
assert.equal(
  logsOnlyWorkflowRouteAccess.canShowBatchTest,
  true,
  'Workflow batch-test navigation should be visible with logs/view-only access'
);
assert.equal(
  Object.hasOwn(logsOnlyWorkflowRouteAccess, 'canRunBatchTest'),
  false,
  'Workflow route access should expose batch-test visibility separately from execution-only state'
);
assert.equal(
  getAgentDetailDefaultHref('agent-1', 'AGENT', {
    canView: true,
    canOpenEditor: false,
    canViewRuntimeLogs: true,
    isPublished: true,
  }),
  '/console/agents/agent-1/logs',
  'AGENT root should fall back to published runtime logs for logs-only users'
);
assert.equal(
  getAgentDetailDefaultHref('agent-1', 'WORKFLOW', {
    canView: true,
    canOpenEditor: false,
    canManageRuntimeAccess: true,
  }),
  '/console/agents/agent-1/api',
  'Workflow root should fall back to API key/docs when runtime-access is available but the editor is not exposed'
);
assert.equal(
  getAgentDetailDefaultHref('agent-1', 'WORKFLOW', {
    canView: true,
    canOpenEditor: false,
    canViewBatchTest: true,
    preferBatchTestLibrary: true,
  }),
  '/console/agents/agent-1/batch-test',
  'Workflow root should prefer the case library when workflow.view is available'
);
assert.equal(
  getAgentDetailDefaultHref('agent-1', 'WORKFLOW', {
    canView: true,
    canOpenEditor: false,
    canViewBatchTest: true,
    preferBatchTestLibrary: false,
  }),
  '/console/agents/agent-1/batch-test/batches',
  'Workflow root should fall back to batch results for logs-only batch-test access'
);
assert.equal(
  getAgentDetailDefaultHref('agent-1', 'WORKFLOW', {
    canView: true,
    canOpenEditor: false,
    canRunBatchTest: true,
  }),
  null,
  'Workflow root should not route execution-only users into batch-test read pages'
);
assert.equal(
  getAgentDetailDefaultHref('agent-1', 'WORKFLOW', {
    canView: true,
    canOpenEditor: false,
  }),
  null,
  'Agent detail root should return no target when no child page is available'
);

console.log('route access scope check passed.');
