import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';
import ts from 'typescript';

const filename = new URL('../src/hooks/console/use-workspace-home.ts', import.meta.url);
const code = ts.transpileModule(readFileSync(filename, 'utf8'), {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  fileName: filename.pathname,
}).outputText;

function runHome({ dashboard = false, modelConfig = false, loading = false, error = null,
  workspace = { id: 'workspace-a' }, contextStatus = 'ready', permissions = [], role = 'normal',
  permissionError = null } = {}) {
  const requests = [];
  const query = (kind, params, options) => {
    requests.push({ kind, params, options });
    return { data: null, isLoading: false, isFetching: false, isError: false, refetch() {} };
  };
  const mocks = {
    react: { useMemo: fn => fn() },
    '@/hooks/use-account-capabilities': { useAccountCapabilities: () => ({
      canAccessOrganizationDashboard: dashboard, canManageModelConfig: modelConfig,
      isLoading: loading, error, refetch() {},
    }) },
    '@/hooks/dashboard/use-dashboard': {
      useDashboardStats: (params, options) => query('stats', params, options),
      useDashboardRecentWork: (params, options) => query('recent', params, options),
    },
    '@/hooks/organization/use-account-permissions': { useAccountPermissions: () => ({
      isLoading: false, error: permissionError, permissions, organizationRole: role,
      workspaceRole: 'member',
    }) },
    '@/store/workspace-store': {
      useCurrentWorkspace: () => workspace,
      useWorkspaceContextStatus: () => contextStatus,
      usePermissions: () => ({ organizationRole: role }),
    },
    '@/routes/console-navigation': {
      // Navigation's own permission matrix is tested by test:console-navigation-routes.
      getZGIConsoleNavigationAccess: () => 'available',
      getZGIConsoleNavigationDisplayState: (access, failed) => failed ? 'error' : access,
    },
  };
  const module = { exports: {} };
  vm.runInNewContext(code, { module, exports: module.exports, require: name => {
    assert.ok(Object.hasOwn(mocks, name), `Unexpected dependency: ${name}`);
    return mocks[name];
  } });
  return { home: module.exports.useWorkspaceHome(), requests };
}

for (const dashboard of [false, true]) {
  for (const modelConfig of [false, true]) {
    test(`model configuration requires dashboard and model capabilities (${dashboard}/${modelConfig})`, () => {
      const { home } = runHome({ dashboard, modelConfig });
      assert.equal(home.actionAccess.modelConfig, dashboard && modelConfig ? 'available' : 'forbidden');
      assert.equal(home.canManageModelConfig, dashboard && modelConfig);
    });
  }
}
test('model configuration fails closed while capabilities load or fail', () => {
  assert.equal(runHome({ dashboard: true, modelConfig: true, loading: true }).home.actionAccess.modelConfig, 'loading');
  assert.equal(runHome({ dashboard: true, modelConfig: true, error: new Error('unavailable') }).home.actionAccess.modelConfig, 'error');
});
for (const [workspace, contextStatus] of [[{ id: 'workspace-a' }, 'ready'], [null, 'none'], [null, 'loading']]) {
  test(`home requests keep explicit scope and wait for context (${contextStatus})`, () => {
    const { requests } = runHome({ workspace, contextStatus });
    assert.equal(requests.length, 2);
    for (const request of requests) {
      assert.equal(request.params.scope, workspace ? 'workspace' : 'overview');
      assert.equal(request.params.workspace_id, workspace?.id);
      assert.equal(request.options.enabled, contextStatus !== 'loading');
    }
  });
}
test('view access does not grant asset creation; each creation action is independent', () => {
  for (const permission of ['', 'agent.create', 'knowledge_base.create', 'workflow.create']) {
    const { home } = runHome({ permissions: permission ? [permission] : [] });
    for (const [action, required] of [['agentCreate', 'agent.create'], ['knowledgeCreate', 'knowledge_base.create'], ['workflowCreate', 'workflow.create']]) {
      assert.equal(home.actionAccess[action], permission === required ? 'available' : 'forbidden');
    }
  }
});
test('permission errors prevent creation even for organization owners', () => {
  const { home } = runHome({ role: 'owner', permissionError: new Error('unavailable') });
  for (const action of ['agentCreate', 'knowledgeCreate', 'workflowCreate']) {
    assert.equal(home.actionAccess[action], 'error');
  }
});
