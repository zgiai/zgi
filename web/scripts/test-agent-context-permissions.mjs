import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import vm from 'node:vm';
import test from 'node:test';
import ts from 'typescript';

const require = createRequire(import.meta.url);
const filename = new URL('../src/components/agents/aichat-context.tsx', import.meta.url);
const code = ts.transpileModule(
  readFileSync(filename, 'utf8') + '\nexport { buildAgentsAIChatContextItems };',
  { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS }, fileName: filename.pathname }
).outputText;
const mocks = {
  '@/components/aichat/page-context': { sanitizeAIChatContextText: value => value },
  '@/hooks/query-keys': { AGENT_KEYS: { all: ['agents'], lists: () => ['agents', 'list'] } },
  '@/services/types/agent': {
    AgentType: { AGENT: 'AGENT', WORKFLOW: 'WORKFLOW', CONVERSATIONAL_AGENT: 'CONVERSATIONAL_WORKFLOW' },
  },
  '@/utils/agent-detail-routes': { getAgentDetailEditHref: id => `/console/agents/${id}` },
};
const module = { exports: {} };
vm.runInNewContext(code, {
  module, exports: module.exports,
  require: name => Object.hasOwn(mocks, name) ? mocks[name] : require(name),
});
const build = module.exports.buildAgentsAIChatContextItems;

test('Agent list wires independent permission decisions into its context', () => {
  const source = readFileSync(new URL('../src/components/agents/agent-asset-list-page.tsx', import.meta.url), 'utf8');
  for (const action of ['Create', 'Update', 'Delete']) {
    assert.match(source, new RegExp(`const can${action}Agent = hasAnyPermission\\(AGENT_PERMISSION_ACTIONS\\.${action.toLowerCase()}\\)`));
    assert.match(source, new RegExp(`can${action}=\\{isWorkflowList \\? false : can${action}Agent\\}`));
  }
  assert.doesNotMatch(source, /AGENT_MANAGE_PERMISSION_CODES/);
});

for (let mask = 0; mask < 8; mask += 1) {
  for (const editable of [false, true]) {
    test(`Agent context uses exact actions (mask=${mask}, editable=${editable})`, () => {
      const canCreate = Boolean(mask & 1);
      const canUpdate = Boolean(mask & 2);
      const canDelete = Boolean(mask & 4);
      const [page, item] = build({
        agents: [{ id: 'fixture-agent', name: 'Fixture', description: '', agent_type: 'AGENT', can_edit: editable }],
        pageSize: 20, searchKeyword: '', canView: true,
        canCreate, canUpdate, canDelete,
        isLoading: false, isFetching: false, hasNextPage: false,
      });
      const check = (context, id, permission, allowed) => {
        const capability = context.capabilities.find(value => value.id === id);
        assert.ok(capability);
        assert.equal(capability.status, allowed ? 'available' : 'disabled');
        assert.equal(capability.permissions.join(','), permission);
        assert.equal(capability.requiresConfirmation, true);
      };
      check(page, 'agent.create_from_page', 'agent.create', canCreate);
      check(page, 'agent.update_identity', 'agent.update', canUpdate);
      check(page, 'agent.delete_visible', 'agent.delete', canDelete);
      check(item, 'agent.update_identity', 'agent.update', editable && canUpdate);
      check(item, 'agent.delete', 'agent.delete', editable && canDelete);
      assert.equal(JSON.stringify([page, item]).includes('agent.manage'), false);
    });
  }
}
