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
const { getAgentConnectorBindingConflicts, mergeSupersededAgentRuntimePayload } = await import(
  moduleURL
);

const connector = (connectorId, connectionId) => ({
  connector_id: connectorId,
  connection_id: connectionId,
});

const workflow = (bindingId, timeoutSeconds = 10) => ({
  binding_id: bindingId,
  label: bindingId,
  agent_id: `agent-${bindingId}`,
  workflow_id: `runtime-${bindingId}`,
  version_strategy: 'latest_published',
  timeout_seconds: timeoutSeconds,
});

const integration = (integrationId, connectionId, actionId = 'message.send') => ({
  integration_id: integrationId,
  connection_id: connectionId,
  access_mode: 'write',
  allowed_action_ids: [actionId],
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

{
  const submitted = {
    ...basePayload,
    integration_bindings: [integration('wecom', 'connection-a')],
  };
  const current = { ...submitted, integration_bindings: [] };
  const saved = {
    ...submitted,
    integration_bindings: [integration('wecom', 'connection-a')],
    binding_revision: 'revision-saved',
  };

  const merged = mergeSupersededAgentRuntimePayload(submitted, current, saved);
  assert.deepEqual(merged.integration_bindings, []);
}

{
  const submitted = {
    ...basePayload,
    integration_bindings: [integration('wecom', 'connection-a')],
  };
  const current = { ...submitted };
  const saved = {
    ...submitted,
    integration_bindings: [integration('wecom', 'connection-b')],
    binding_revision: 'revision-saved',
  };

  const merged = mergeSupersededAgentRuntimePayload(submitted, current, saved);
  assert.deepEqual(merged.integration_bindings, [integration('wecom', 'connection-b')]);
}

{
  const submitted = {
    ...basePayload,
    integration_bindings: [integration('wecom', 'connection-a')],
  };
  const current = { ...submitted, integration_bindings: [] };
  const saved = {
    ...submitted,
    integration_bindings: [integration('wecom', 'connection-b')],
    binding_revision: 'revision-saved',
  };

  const merged = mergeSupersededAgentRuntimePayload(submitted, current, saved);
  assert.deepEqual(merged.integration_bindings, []);
}

{
  const submitted = {
    ...basePayload,
    integration_bindings: [
      integration('wecom', 'connection-a'),
      integration('mail', 'connection-mail-a', 'mail.search'),
    ],
  };
  const current = {
    ...submitted,
    integration_bindings: [integration('mail', 'connection-mail-a', 'mail.search')],
  };
  const saved = {
    ...submitted,
    integration_bindings: [
      integration('wecom', 'connection-a'),
      integration('mail', 'connection-mail-b', 'mail.search'),
    ],
    binding_revision: 'revision-saved',
  };

  const merged = mergeSupersededAgentRuntimePayload(submitted, current, saved);
  assert.deepEqual(merged.integration_bindings, [
    integration('mail', 'connection-mail-b', 'mail.search'),
  ]);
}

// Execute the production save callback without mounting React: the API stub
// deliberately rejects its first call with a stale binding revision.
const pageModelSource = readFileSync(
  path.join(
    process.cwd(),
    'src/components/agents/agent-runtime/hooks/use-agent-runtime-page-model.tsx'
  ),
  'utf8'
);
const pageModelAST = ts.createSourceFile(
  'page-model.tsx',
  pageModelSource,
  ts.ScriptTarget.ESNext,
  true,
  ts.ScriptKind.TSX
);
const findNode = predicate => {
  let found;
  const visit = node => {
    if (predicate(node)) found = node;
    if (!found) ts.forEachChild(node, visit);
  };
  visit(pageModelAST);
  return found;
};
const compilePageExpression = (expression, context) => {
  const compiled = ts.transpileModule(`const extracted = ${expression};`, {
    compilerOptions: { target: ts.ScriptTarget.ES2022 },
  }).outputText;
  return new Function(...Object.keys(context), `${compiled}\nreturn extracted;`)(
    ...Object.values(context)
  );
};
const compileSaveCallback = context => {
  const declaration = findNode(
    node =>
      ts.isVariableDeclaration(node) && node.name.getText(pageModelAST) === 'saveRuntimePayload'
  );
  assert.ok(declaration && ts.isCallExpression(declaration.initializer));
  return compilePageExpression(declaration.initializer.arguments[0].getText(pageModelAST), context);
};
const connectorRevisionErrorNode = findNode(
  node =>
    ts.isClassDeclaration(node) && node.name?.text === 'AgentConnectorBindingRevisionChangedError'
);
const ConnectorRevisionError = connectorRevisionErrorNode
  ? compilePageExpression(connectorRevisionErrorNode.getText(pageModelAST), {})
  : class extends Error {};
const baselinePayload = { ...basePayload, connector_bindings: [connector('github', 'a')] };
const pendingPayload = { ...baselinePayload, connector_bindings: [connector('github', 'b')] };

const createSaveHarness = serverPayload => {
  const requests = [];
  const warnings = [];
  const lastServerPayloadRef = { current: baselinePayload };
  const context = {
    agentId: 'agent',
    agentMemoryConfigRevision: '',
    validateAgentMemorySlots: () => [],
    agentService: {
      updateAgentConfig: async (_id, payload) => {
        requests.push(payload);
        if (requests.length === 1) throw { currentConfig: serverPayload };
        return { data: { ...payload, binding_revision: 'revision-saved', updated_at: 123 } };
      },
    },
    getAgentBindingRevisionConflict: error => error,
    getAgentConnectorBindingConflicts,
    AgentMemoryRevisionChangedError: class extends Error {},
    AgentConnectorBindingRevisionChangedError: ConnectorRevisionError,
    setBindingHealth: () => {},
    setAgentMemoryConfigRevision: () => {},
    describeBindingChanges: () => ['GitHub'],
    t: key => key,
    toast: { warning: message => warnings.push(message) },
    payloadFromRuntimeConfig: (config, fallback) => ({ ...fallback, ...config }),
    mergeSupersededAgentRuntimePayload,
    lastServerPayloadRef,
    queryClient: { setQueryData: () => {}, invalidateQueries: () => {} },
    AGENT_KEYS: { config: id => id, detail: id => id },
  };
  return { context, requests, warnings, save: compileSaveCallback(context) };
};

{
  const serverPayload = {
    ...baselinePayload,
    connector_bindings: [connector('github', 'c')],
    binding_revision: 'revision-server',
  };
  const harness = createSaveHarness(serverPayload);
  let failure;
  await assert.rejects(
    harness.save(pendingPayload),
    error => {
      failure = error;
      return error instanceof ConnectorRevisionError;
    },
    'a conflicting switch must stop before the CAS retry'
  );
  assert.equal(
    harness.requests.length,
    1,
    'must not silently submit B with the newly fetched revision'
  );

  let recoveredPayload;
  let pickerOpened = false;
  const failures = [];
  const failedCallback = findNode(
    node => ts.isPropertyAssignment(node) && node.name.getText(pageModelAST) === 'onSaveFailed'
  );
  const onSaveFailed = compilePageExpression(failedCallback.initializer.getText(pageModelAST), {
    ...harness.context,
    currentPayload: {
      ...pendingPayload,
      home_title: 'edited while the request was pending',
      connector_bindings: [connector('github', 'd'), connector('exa', 'exa-local')],
    },
    applyServerBindingPayload: payload => {
      recoveredPayload = payload;
    },
    setConnectorDialogOpen: open => {
      pickerOpened = open;
    },
    toast: { warning: message => failures.push(message), error: message => failures.push(message) },
    getAgentBindingConflict: () => undefined,
  });
  onSaveFailed(failure, { silent: true });
  assert.deepEqual(recoveredPayload.connector_bindings, [
    connector('exa', 'exa-local'),
    connector('github', 'c'),
  ]);
  assert.equal(recoveredPayload.home_title, 'edited while the request was pending');
  assert.equal(recoveredPayload.binding_revision, 'revision-server');
  assert.equal(harness.context.lastServerPayloadRef.current, failure.serverPayload);
  assert.equal(pickerOpened, true, 'the picker must show the latest C for a fresh explicit choice');
  assert.deepEqual(
    failures,
    ['toasts.connectorBindingRevisionChanged'],
    'even autosave conflicts need an actionable notice'
  );
}

{
  const harness = createSaveHarness({
    ...baselinePayload,
    connector_bindings: [connector('github', 'a'), connector('exa', 'exa-server')],
    binding_revision: 'revision-server',
  });
  const result = await harness.save(pendingPayload);
  assert.equal(
    harness.requests.length,
    2,
    'nonconflicting provider changes retain automatic CAS recovery'
  );
  assert.deepEqual(harness.requests[1].connector_bindings, [
    connector('exa', 'exa-server'),
    connector('github', 'b'),
  ]);
  assert.equal(harness.requests[1].binding_revision, 'revision-server');
  assert.equal(result.bindingPayloadRebased, true);
}

{
  // The user may confirm B after the original request began, while that
  // request is independently rebased from A onto the server's C.
  const savedPayload = {
    ...baselinePayload,
    connector_bindings: [connector('github', 'c')],
    binding_revision: 'revision-saved',
  };
  const lastServerPayloadRef = { current: baselinePayload };
  let recoveredPayload;
  let pickerOpened = false;
  const notices = [];
  const supersededCallback = findNode(
    node => ts.isPropertyAssignment(node) && node.name.getText(pageModelAST) === 'onSaveSuperseded'
  );
  const onSaveSuperseded = compilePageExpression(
    supersededCallback.initializer.getText(pageModelAST),
    {
      lastServerPayloadRef,
      mergeSupersededAgentRuntimePayload,
      getAgentConnectorBindingConflicts,
      applyServerBindingPayload: payload => {
        recoveredPayload = payload;
      },
      setConnectorDialogOpen: open => {
        pickerOpened = open;
      },
      t: key => key,
      toast: { warning: message => notices.push(message) },
    }
  );
  onSaveSuperseded({ savedPayload }, baselinePayload, {
    ...pendingPayload,
    system_prompt: 'edited while rebasing',
  });
  assert.deepEqual(recoveredPayload.connector_bindings, [connector('github', 'c')]);
  assert.equal(recoveredPayload.system_prompt, 'edited while rebasing');
  assert.equal(recoveredPayload.binding_revision, 'revision-saved');
  assert.equal(lastServerPayloadRef.current, savedPayload);
  assert.equal(pickerOpened, true);
  assert.deepEqual(notices, ['toasts.connectorBindingRevisionChanged']);
}

for (const [label, baseline, local, server, expected, conflicts] of [
  ['different switches', 'a', 'b', 'c', 'c', ['github']],
  ['local removal / remote switch', 'a', undefined, 'c', 'c', ['github']],
  ['local switch / remote removal', 'a', 'b', undefined, undefined, ['github']],
  ['concurrent additions', undefined, 'b', 'c', 'c', ['github']],
  ['same switch', 'a', 'b', 'b', 'b', []],
  ['same removal', 'a', undefined, undefined, undefined, []],
  ['local-only switch', 'a', 'b', 'a', 'b', []],
  ['remote-only switch', 'a', 'a', 'c', 'c', []],
]) {
  const bindings = connection => (connection ? [connector('github', connection)] : []);
  const submitted = { ...basePayload, connector_bindings: bindings(baseline) };
  const current = { ...submitted, connector_bindings: bindings(local) };
  const saved = {
    ...submitted,
    connector_bindings: bindings(server),
    binding_revision: 'revision-saved',
  };
  const merged = mergeSupersededAgentRuntimePayload(submitted, current, saved);
  assert.deepEqual(
    merged.connector_bindings,
    bindings(expected),
    `${label}: conflicting native slots must retain the server selection for a fresh confirmation`
  );
  assert.deepEqual(getAgentConnectorBindingConflicts(submitted, current, saved), conflicts, label);
}

{
  const submitted = {
    ...basePayload,
    connector_bindings: [connector('github', 'a'), connector('exa', 'exa-a')],
  };
  const current = {
    ...submitted,
    system_prompt: 'new local prompt',
    enabled_skill_ids: ['local-skill'],
    connector_bindings: [connector('github', 'b'), connector('exa', 'exa-a')],
  };
  const saved = {
    ...submitted,
    enabled_skill_ids: ['server-skill'],
    connector_bindings: [connector('github', 'a'), connector('exa', 'exa-c')],
    binding_revision: 'revision-saved',
  };
  const merged = mergeSupersededAgentRuntimePayload(submitted, current, saved);
  assert.deepEqual(getAgentConnectorBindingConflicts(submitted, current, saved), []);
  assert.deepEqual(merged.connector_bindings, [
    connector('exa', 'exa-c'),
    connector('github', 'b'),
  ]);
  assert.deepEqual(merged.enabled_skill_ids, ['server-skill', 'local-skill']);
  assert.equal(merged.system_prompt, 'new local prompt');
}

console.log('Agent binding rebase merge checks passed.');
