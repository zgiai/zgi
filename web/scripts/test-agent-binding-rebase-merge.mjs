import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const transpile = relativePath =>
  ts.transpileModule(readFileSync(path.join(process.cwd(), relativePath), 'utf8'), {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText;
const databaseBindingModuleURL = `data:text/javascript;base64,${Buffer.from(
  transpile('src/components/agents/agent-runtime/database-binding-draft.ts')
).toString('base64')}`;
const outputText = transpile('src/components/agents/agent-runtime/binding-rebase-merge.ts').replace(
  "from './database-binding-draft';",
  `from '${databaseBindingModuleURL}';`
);
const moduleURL = `data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`;
const { mergeSupersededAgentRuntimePayload } = await import(moduleURL);

const workflow = (bindingId, timeoutSeconds = 10) => ({
  binding_id: bindingId,
  label: bindingId,
  agent_id: `agent-${bindingId}`,
  workflow_id: `runtime-${bindingId}`,
  version_strategy: 'latest_published',
  timeout_seconds: timeoutSeconds,
});

const basePayload = {
  system_prompt: 'submitted prompt',
  model_provider: 'provider',
  model: 'model',
  model_parameters: {},
  enabled_skill_ids: [],
  use_memory: false,
  agent_memory_enabled: false,
  agent_memory_slots: [],
  file_upload_enabled: false,
  home_title: 'Agent',
  opening_statement: '',
  input_placeholder: '',
  theme_color: '',
  suggested_questions: [],
  knowledge_dataset_ids: [],
  knowledge_retrieval_config: {},
  database_bindings: [],
  workflow_bindings: [],
  binding_revision: 'revision-submitted',
};

{
  const submitted = {
    ...basePayload,
    enabled_skill_ids: ['skill-local'],
    knowledge_dataset_ids: ['knowledge-local'],
    database_bindings: [{ data_source_id: 'database-local', table_ids: ['table-local'] }],
    workflow_bindings: [workflow('workflow-local')],
  };
  const current = { ...submitted, system_prompt: 'edited while saving' };
  const saved = {
    ...submitted,
    enabled_skill_ids: ['skill-server'],
    knowledge_dataset_ids: ['knowledge-server'],
    database_bindings: [{ data_source_id: 'database-server', table_ids: ['table-server'] }],
    workflow_bindings: [workflow('workflow-server')],
    binding_revision: 'revision-saved',
  };

  const merged = mergeSupersededAgentRuntimePayload(submitted, current, saved);
  assert.equal(merged.system_prompt, 'edited while saving');
  assert.deepEqual(merged.enabled_skill_ids, saved.enabled_skill_ids);
  assert.deepEqual(merged.knowledge_dataset_ids, saved.knowledge_dataset_ids);
  assert.deepEqual(merged.database_bindings, [
    {
      data_source_id: 'database-server',
      table_ids: ['table-server'],
      writable_table_ids: [],
    },
  ]);
  assert.deepEqual(
    merged.workflow_bindings?.map(binding => binding.binding_id),
    ['workflow-server']
  );
  assert.equal(merged.binding_revision, 'revision-saved');
}

{
  const submitted = {
    ...basePayload,
    enabled_skill_ids: ['skill-a', 'skill-b'],
    knowledge_dataset_ids: ['knowledge-a', 'knowledge-b'],
    database_bindings: [
      {
        data_source_id: 'database-1',
        table_ids: ['table-1', 'table-2'],
        writable_table_ids: ['table-2'],
      },
      { data_source_id: 'database-remove', table_ids: ['table-remove'] },
    ],
    workflow_bindings: [workflow('workflow-1'), workflow('workflow-remove')],
  };
  const current = {
    ...submitted,
    enabled_skill_ids: ['skill-b', 'skill-new'],
    knowledge_dataset_ids: ['knowledge-b', 'knowledge-new'],
    database_bindings: [
      {
        data_source_id: 'database-1',
        table_ids: ['table-1', 'table-3'],
        writable_table_ids: ['table-3'],
      },
      { data_source_id: 'database-new', table_ids: ['table-new'] },
    ],
    workflow_bindings: [workflow('workflow-1', 20), workflow('workflow-new')],
  };
  const saved = {
    ...submitted,
    enabled_skill_ids: ['skill-a', 'skill-b', 'skill-server'],
    knowledge_dataset_ids: ['knowledge-a', 'knowledge-b', 'knowledge-server'],
    database_bindings: [
      {
        data_source_id: 'database-1',
        table_ids: ['table-1', 'table-2', 'table-4'],
        writable_table_ids: ['table-2'],
      },
      {
        data_source_id: 'database-remove',
        table_ids: ['table-remove', 'table-server-added'],
      },
      { data_source_id: 'database-server', table_ids: ['table-server'] },
    ],
    workflow_bindings: [
      workflow('workflow-1'),
      workflow('workflow-remove'),
      workflow('workflow-server'),
    ],
    binding_revision: 'revision-saved',
  };

  const merged = mergeSupersededAgentRuntimePayload(submitted, current, saved);
  assert.deepEqual(merged.enabled_skill_ids, ['skill-b', 'skill-server', 'skill-new']);
  assert.deepEqual(merged.knowledge_dataset_ids, [
    'knowledge-b',
    'knowledge-server',
    'knowledge-new',
  ]);
  assert.deepEqual(merged.database_bindings, [
    {
      data_source_id: 'database-1',
      table_ids: ['table-1', 'table-3', 'table-4'],
      writable_table_ids: ['table-3'],
    },
    {
      data_source_id: 'database-new',
      table_ids: ['table-new'],
      writable_table_ids: [],
    },
    {
      data_source_id: 'database-server',
      table_ids: ['table-server'],
      writable_table_ids: [],
    },
  ]);
  assert.deepEqual(
    merged.workflow_bindings?.map(binding => [binding.binding_id, binding.timeout_seconds]),
    [
      ['workflow-1', 20],
      ['workflow-new', 10],
      ['workflow-server', 10],
    ]
  );
  assert.equal(merged.binding_revision, 'revision-saved');
}

console.log('Agent binding rebase merge checks passed.');
