import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import vm from 'node:vm';
import test from 'node:test';
import ts from 'typescript';

const require = createRequire(import.meta.url);
const compile = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
  fileName: path,
}).outputText;
function load(code, mocks) {
  const module = { exports: {} };
  vm.runInNewContext(code, { module, exports: module.exports, require: name => {
    if (name === 'react/jsx-runtime') return require(name);
    assert.ok(Object.hasOwn(mocks, name), `Unexpected dependency: ${name}`);
    return mocks[name];
  } });
  return module.exports;
}
const routes = load(compile('../src/utils/agent-detail-routes.ts'), {
  '@/services/types/agent': { AgentType: { AGENT: 'AGENT', WORKFLOW: 'WORKFLOW', CONVERSATIONAL_AGENT: 'CONVERSATIONAL_WORKFLOW' } },
});
const guardCode = compile('../src/components/agents/api/agent-api-access-guard.tsx');

function check({ agentType = 'AGENT', granted = [], loading = false, permissionError = null, queryLoading = false } = {}) {
  const queries = [];
  let childrenCalled = 0;
  const { AgentApiAccessGuard } = load(guardCode, {
    'lucide-react': { AlertCircle: () => null, Loader2: () => null },
    '@/hooks/agent/use-agents': { useAgent: (id, enabled = true) => {
      queries.push({ id, enabled });
      // Keep cached data even when disabled, as React Query does.
      return { agent: { data: { agent_type: agentType } }, isLoading: queryLoading, error: null };
    } },
    '@/hooks/organization/use-account-permissions': { useAccountPermissions: () => ({
      hasAnyPermission: codes => codes.some(code => granted.includes(code)),
      isLoading: loading, error: permissionError,
    }) },
    '@/i18n': { useT: () => key => key },
    '@/utils/agent-detail-routes': routes,
    '@/utils/error-notifications': { getErrorMessage: () => 'safe error' },
    '@/constants/permissions': {
      AGENT_PERMISSION_ACTIONS: { runtimeAccessManage: ['agent.runtime_access.manage'] },
      WORKFLOW_PERMISSION_ACTIONS: { runtimeAccessManage: ['workflow.runtime_access.manage'] },
    },
  });
  const element = AgentApiAccessGuard({ agentId: 'fixture', children: () => { childrenCalled++; return null; } });
  return { queries, childrenCalled, element };
}

for (const agentType of ['AGENT', 'WORKFLOW']) {
  for (const granted of [[], ['agent.runtime_access.manage'], ['workflow.runtime_access.manage'], ['agent.runtime_access.manage', 'workflow.runtime_access.manage']]) {
    test(`metadata query and runtime-specific children: ${agentType}/${granted.join(',')}`, () => {
      const result = check({ agentType, granted });
      assert.equal(result.queries.length, 1);
      assert.equal(result.queries[0].enabled, granted.length > 0);
      assert.equal(result.childrenCalled, Number(granted.includes(`${agentType.toLowerCase()}.runtime_access.manage`)));
    });
  }
}
for (const state of [{ loading: true }, { permissionError: 'permission lookup failed' }]) {
  test(`unresolved permissions do not query or render cached data: ${JSON.stringify(state)}`, () => {
    const result = check({ ...state, granted: ['agent.runtime_access.manage'] });
    assert.equal(result.queries[0].enabled, false);
    assert.equal(result.childrenCalled, 0);
  });
}
test('revoked permissions deny access even if a disabled query is pending', () => {
  const result = check({ queryLoading: true });
  assert.equal(result.queries[0].enabled, false);
  assert.equal(result.childrenCalled, 0);
  assert.match(JSON.stringify(result.element), /common.accessDenied/);
});
