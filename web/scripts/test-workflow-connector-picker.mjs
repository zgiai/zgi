import { rewriteConnectorPureImports } from './connector-pure-fixture.mjs';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const read = relativePath => readFileSync(path.join(process.cwd(), relativePath), 'utf8');

const manager = read('src/components/workflow/nodes/tool/manager/index.tsx');
const toolConfig = read('src/components/workflow/nodes/tool/manager/tool-config-dialog.tsx');
const picker = read('src/components/connectors/picker/connector-picker-dialog.tsx');
const toolTypes = read('src/components/workflow/nodes/tool/config.ts');
const workflowEditor = read('src/components/workflow/hooks/use-workflow-editor.tsx');
const workflowRoot = read('src/components/workflow/index.tsx');

assert.match(
  manager,
  /workflowConnectorSelection\(/,
  'the node selection must retain missing native UUIDs instead of assuming they are legacy'
);
const transpile = relativePath =>
  ts.transpileModule(read(relativePath), {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText;
const sourceModuleURL = source =>
  `data:text/javascript;base64,${Buffer.from(rewriteConnectorPureImports(source)).toString('base64')}`;
const { workflowConnectorSelection } = await import(
  sourceModuleURL(
    transpile('src/components/workflow/nodes/tool/manager/workflow-connector-selection.ts')
  )
);
const setupURL = sourceModuleURL(transpile('src/components/connectors/setup/app-setup-state.ts'));
const metadataURL = sourceModuleURL(transpile('src/components/connectors/connector-metadata.ts'));
const { createConnectorPickerRows, requestConnectorPickerAction, isConnectorPickerSwitchCurrent } =
  await import(
    sourceModuleURL(
      transpile('src/components/connectors/picker/connector-picker-model.ts')
        .replace("'../setup/app-setup-state'", JSON.stringify(setupURL))
        .replace("from '../connector-metadata';", `from '${metadataURL}';`)
    )
  );

const connector = {
  id: 'web-search',
  plugin_id: 'web-search',
  name: 'Exa Web Search',
  description: 'Search the web',
  classification: 'connector',
  key_fields: [{ name: 'api_key', label: 'API Key', required: true, secret: true }],
  capabilities: [{ id: 'search_web', name: 'Search', description: 'Search', usable: true }],
  connection_summary: { count: 2, state: 'ready' },
};
const currentConnectionID = 'a1a1a1a1-1111-4111-8111-a1a1a1a1a1a1';
const replacement = {
  id: 'b2b2b2b2-2222-4222-8222-b2b2b2b2b2b2',
  connector_id: connector.id,
  name: 'Research Key',
  external_display_name: 'same-account',
  state: 'ready',
  credential_configured: true,
  revision: 1,
};
const target = { type: 'workflow', id: 'workflow-1', binding_instance_id: 'search-node-1' };
const selectionInput = {
  workflowID: target.id,
  nodeID: target.binding_instance_id,
  connectorID: connector.id,
  currentConnectionID,
  unavailableLabel: '连接不可用',
};

for (const state of ['missing', 'unavailable', 'reconnect_required', 'revoked']) {
  const personalConnection =
    state === 'missing' ? undefined : { ...replacement, id: currentConnectionID, state };
  const result = workflowConnectorSelection({ ...selectionInput, personalConnection });
  assert.equal(result.pickerBindings.length, 1, `${state}: retain the occupied native slot`);

  assert.equal(result.pickerBindings[0].status, 'needs_replacement');
  assert.equal(result.pickerBindings[0].connection_id, currentConnectionID);
  assert.ok(result.pickerBindings[0].id.includes(target.binding_instance_id));
  assert.notEqual(result.selectedConnectionLabel, currentConnectionID, 'never display the UUID');
  if (state === 'missing') assert.equal(result.selectedConnectionLabel, '连接不可用');

  const connections = personalConnection ? [personalConnection, replacement] : [replacement];
  const row = createConnectorPickerRows([connector], connections, result.pickerBindings).find(
    entry => entry.connection?.id === replacement.id
  );
  assert.ok(row, `${state}: the replacement remains selectable`);
  assert.equal(row.replacedBinding?.connection_id, currentConnectionID);
  const calls = [];
  await requestConnectorPickerAction(
    row,
    {
      onRequestAdd: request => calls.push({ action: 'add', request }),
      onRequestSwitch: request => calls.push({ action: 'switch', request }),
    },
    target
  );
  assert.equal(calls.length, 1);
  assert.equal(calls[0].action, 'switch', `${state}: replacement cannot bypass confirmation`);
  assert.equal(calls[0].request.replacedBinding.connection_id, currentConnectionID);
  assert.equal(calls[0].request.target.binding_instance_id, target.binding_instance_id);
  assert.equal(
    isConnectorPickerSwitchCurrent(calls[0].request, connections, result.pickerBindings, target),
    true
  );
  const otherNode = workflowConnectorSelection({ ...selectionInput, nodeID: 'search-node-2' });
  assert.notEqual(otherNode.pickerBindings[0].id, result.pickerBindings[0].id);
  assert.equal(
    isConnectorPickerSwitchCurrent(calls[0].request, connections, otherNode.pickerBindings, {
      ...target,
      binding_instance_id: 'search-node-2',
    }),
    false,
    'a confirmation for one node cannot replace a different node slot'
  );
}

const personalConnection = { ...replacement, id: currentConnectionID };
const ready = workflowConnectorSelection({
  ...selectionInput,
  personalConnection,
});
assert.equal(ready.pickerBindings[0].status, 'active');
assert.equal(ready.selectedConnectionLabel, personalConnection.name);
const empty = workflowConnectorSelection({ ...selectionInput, currentConnectionID: '' });
assert.deepEqual(empty.pickerBindings, []);
assert.equal(empty.selectedConnectionLabel, '');

assert.match(manager, /ConnectorPickerDialog/);
assert.match(manager, /ConnectorCreateContent/);
assert.match(manager, /useWorkflowEditor/);
assert.doesNotMatch(manager, /enable_connectors|connectorsEnabled/);
assert.match(manager, /Boolean\(isConnector && connectorID && workflowId\)/);
assert.match(manager, /disabled=\{readOnly \|\| !connectorID \|\| !workflowId\}/);
assert.match(manager, /type:\s*'workflow'/);
assert.match(manager, /id:\s*workflowId/);
assert.match(manager, /binding_instance_id:\s*nodeId/);
assert.match(manager, /connectorId=\{connectorID\}/);
assert.match(manager, /usePersonalConnections/);
assert.match(manager, /picker\.workflowConnectionPlaceholder/);
assert.match(manager, /picker\.workflowConnectionHint/);
assert.match(manager, /\[nodeId, workflowId, readOnly\]/);
assert.match(manager, /unavailableLabel:\s*connectorT\('status\.unavailable'\)/);
assert.match(manager, /onRequestSwitch/);
assert.match(manager, /setPendingConnectorSelection/);
assert.match(manager, /picker\.authorization\.confirm/);
assert.match(manager, /useConnectorUnavailableReferences/);
assert.doesNotMatch(
  manager,
  /connectorService|attachTargetBinding|createTargetBinding/,
  'Workflow selection must stay in the graph draft/save transaction'
);

const addHandler = manager.match(/onRequestAdd=\{request =>([\s\S]*?)onRequestConnectAndAdd=/)?.[1];
assert.ok(addHandler, 'Workflow must host the ready-connection add action');
assert.doesNotMatch(
  addHandler,
  /provider_id|tool_name/,
  'choosing a Connection must not rewrite the provider/tool portion of the graph tuple'
);

assert.match(picker, /connectorId\?: string/);
assert.match(picker, /item\.id === connectorId/);
assert.match(picker, /target_type:\s*target\.type/);
assert.match(picker, /target_id:\s*target\.id/);
assert.match(toolTypes, /connector\?: \{[\s\S]*connection_id: string[\s\S]*operation_id: string/);
assert.match(workflowEditor, /workflowId: string/);
assert.match(workflowRoot, /workflowId:\s*workflowData\?\.id\s*\?\?\s*''/);
assert.match(
  manager,
  /binding_instance_id:\s*nodeId[\s\S]*setConnectorConnection\(/,
  'each workflow node keeps its own explicit connection UUID'
);

assert.doesNotMatch(
  manager,
  /allowed_action_ids|allowedActions|grant|permission matrix/i,
  'Workflow Connector setup must remain policy-free'
);
assert.doesNotMatch(
  `${manager}\n${toolConfig}`,
  /工具配置|该项为必填项|已启用|已禁用|请选择\.\.\.|保存配置/,
  'workflow tool settings must use locale messages rather than hardcoded Chinese copy'
);
assert.match(toolConfig, /common\.form\.required/);
assert.match(toolConfig, /common\.form\.selectPlaceholder/);
assert.match(toolConfig, /common\.enabled/);
assert.match(toolConfig, /common\.disabled/);
assert.match(toolConfig, /common\.cancel/);
assert.match(toolConfig, /common\.save/);

console.log('Workflow Connector picker graph-draft and native-reference checks passed.');
