import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';
import ts from 'typescript';

const filename = new URL('../src/components/agents/templates/use-create-from-template.ts', import.meta.url);
const code = ts.transpileModule(readFileSync(filename, 'utf8'), {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  fileName: filename.pathname,
}).outputText;

function setup(importError) {
  const navigations = [];
  const imports = [];
  const mocks = {
    react: { useCallback: fn => fn },
    'next/navigation': { useRouter: () => ({ push: href => navigations.push(href) }) },
    'next-intl': { useLocale: () => 'zh-Hans' },
    sonner: { toast: { error() {} } },
    yaml: { parse: () => ({ app: {}, workflow: { graph: { nodes: [] } } }), stringify: JSON.stringify },
    '@/hooks/workflow/use-workflow-import-export': { useImportWorkflow: () => ({
      isImporting: false,
      importWorkflow: async input => {
        imports.push(input);
        if (importError) throw importError;
        return { data: { agent_id: 'created-resource' } };
      },
    }) },
    '@/i18n': { useT: () => key => key },
    '@/lib/config': { withBasePath: path => path },
    '@/utils/agent-detail-routes': {
      getAgentDetailBaseHref: (id, kind) => `/console/${kind === 'agent' ? 'agents' : 'workflows'}/${id}`,
      getAgentDetailEditHref: (id, kind) => `/console/${kind === 'agent' ? 'agents' : 'workflows'}/${id}/editor`,
    },
    '@/components/common/icon-input/avatar-preset-upload': { uploadAppAvatarPreset: async () => ({ imageId: 'fixture-image' }) },
    '@/components/common/icon-input/avatar-presets': { getRandomAppAvatar: () => 'fixture-avatar' },
  };
  const module = { exports: {} };
  vm.runInNewContext(code, {
    module, exports: module.exports, File,
    fetch: async () => ({ ok: true, text: async () => 'fixture-yaml' }),
    require: name => {
      assert.ok(Object.hasOwn(mocks, name), `Unexpected dependency: ${name}`);
      return mocks[name];
    },
  });
  return { create: module.exports.useCreateAgentFromTemplate().createFromTemplate, navigations, imports };
}

for (const kind of ['agent', 'workflow']) {
  test(`created ${kind} enters the permission-aware resource root`, async () => {
    const { create, navigations, imports } = setup();
    const result = await create({ id: 'fixture', kind, yamlPath: '/fixture.yml' }, 'workspace-a');
    assert.equal(result.agent_id, 'created-resource');
    assert.deepEqual(navigations, [`/console/${kind === 'agent' ? 'agents' : 'workflows'}/created-resource`]);
    assert.equal(imports.length, 1);
    assert.equal(imports[0].workspaceId, 'workspace-a');
    assert.equal(imports[0].file.name, 'fixture.yml');
  });
}
test('failed template import never navigates to a resource', async () => {
  const error = new Error('import denied');
  const { create, navigations } = setup(error);
  await assert.rejects(create({ id: 'fixture', kind: 'workflow', yamlPath: '/fixture.yml' }, 'workspace-a'), err => err === error);
  assert.deepEqual(navigations, []);
});
