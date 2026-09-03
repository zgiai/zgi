import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const read = relativePath => readFileSync(path.join(process.cwd(), relativePath), 'utf8');
const transpile = relativePath =>
  ts.transpileModule(read(relativePath), {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText;
const sourceModuleURL = source =>
  `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`;
const metadataURL = sourceModuleURL(transpile('src/components/connectors/connector-metadata.ts'));
const { localizeConnectorCatalogItem } = await import(metadataURL);
const setupURL = sourceModuleURL(transpile('src/components/connectors/setup/app-setup-state.ts'));
const modelURL = sourceModuleURL(
  transpile('src/components/connectors/picker/connector-picker-model.ts').replace("'../setup/app-setup-state'", JSON.stringify(setupURL)).replace(
    "from '../connector-metadata';",
    `from '${metadataURL}';`
  )
);
const {
  createConnectorPickerRows,
  filterConnectorPickerRows,
  isConnectorPickerSwitchCurrent,
  requestConnectorPickerAction,
} = await import(modelURL);

const target = { type: 'agent', id: 'agent-customer-research' };

const pickerDialog = readFileSync(
  path.join(process.cwd(), 'src/components/connectors/picker/connector-picker-dialog.tsx'),
  'utf8'
);
assert.match(pickerDialog, /target_type:\s*target\.type/);
assert.match(pickerDialog, /target_id:\s*target\.id/);

const catalog = [
  {
    id: 'github',
    plugin_id: 'github-plugin',
    name: 'GitHub',
    description: 'Manage repositories, Issues, and Pull Requests',
    classification: 'connector',
    key_fields: [{ name: 'token', label: 'Personal access token', required: true, secret: true }],
    capabilities: [
      { id: 'issues.read', name: 'Issues', description: 'Search issues', usable: true },
    ],
    connection_summary: { count: 1, state: 'ready' },
  },
  {
    id: 'notion',
    plugin_id: 'notion-plugin',
    name: 'Notion',
    description: 'Search pages and databases',
    classification: 'connector',
    key_fields: [{ name: 'token', label: 'Integration Key', required: true, secret: true }],
    capabilities: [
      { id: 'pages.search', name: 'Pages', description: 'Search pages', usable: true },
    ],
    connection_summary: { count: 1, state: 'ready' },
  },
  {
    id: 'linear',
    plugin_id: 'linear-plugin',
    name: 'Linear',
    description: 'View and update Issues',
    classification: 'connector',
    key_fields: [{ name: 'api_key', label: 'API Key', required: true, secret: true }],
    capabilities: [{ id: 'issues.list', name: 'Issues', description: 'List issues', usable: true }],
    connection_summary: { count: 1, state: 'reconnect_required' },
  },
  {
    id: 'web-search',
    plugin_id: 'exa-plugin',
    name: 'Exa Web Search',
    name_i18n: { 'en-US': 'Exa Web Search', 'zh-Hans': 'Exa 网页搜索' },
    description: 'Search the live web',
    classification: 'connector',
    key_fields: [{ name: 'api_key', label: 'API Key', required: true, secret: true }],
    capabilities: [
      { id: 'web.search', name: 'Web search', description: 'Find current sources', usable: true },
    ],
    connection_summary: { count: 0 },
  },
  {
    id: 'reference-library',
    plugin_id: 'reference-library-plugin',
    name: 'Reference Library',
    description: 'Read archived documents',
    classification: 'data_source',
    key_fields: [{ name: 'api_key', label: 'API Key', required: true, secret: true }],
    capabilities: [
      { id: 'documents.read', name: 'Read documents', description: 'Read documents', usable: true },
    ],
    connection_summary: { count: 0 },
  },
];

const connection = (id, connectorId, state = 'ready') => ({
  id,
  connector_id: connectorId,
  name: `${connectorId} personal connection`,
  external_display_name: `@${connectorId}-owner`,
  state,
  credential_configured: true,
  revision: 1,
  created_at: '2026-09-01T08:00:00Z',
  updated_at: '2026-09-01T08:00:00Z',
});

const connections = [
  connection('connection-github', 'github'),
  {
    ...connection('connection-github-side-project', 'github'),
    name: 'Side project GitHub',
    external_display_name: '@side-project',
  },
  connection('connection-notion', 'notion'),
  connection('connection-linear', 'linear', 'reconnect_required'),
  connection('connection-linear-unavailable', 'linear', 'unavailable'),
];

const binding = (id, connectionId, connectorId, compatibility = 'native') => ({
  id,
  connection_id: connectionId,
  connector_id: connectorId,
  target,
  status: 'active',
  compatibility,
  revision: 7,
});

const initialBindings = Object.freeze([
  Object.freeze(binding('binding-github', 'connection-github', 'github')),
]);

const initialRows = createConnectorPickerRows(catalog, connections, initialBindings);
assert.equal(
  initialRows.find(row => row.connector.id === 'web-search')?.connector.classification,
  'connector',
  'the Exa picker entry must carry its real Connector catalog identity and classification'
);
const rowForConnection = connectionID =>
  initialRows.find(row => row.connection?.id === connectionID);
const newConnectionRow = connectorID =>
  initialRows.find(row => row.connector.id === connectorID && !row.connection);

assert.equal(rowForConnection('connection-github')?.state, 'added');
assert.equal(rowForConnection('connection-github-side-project')?.state, 'ready');
assert.equal(rowForConnection('connection-notion')?.state, 'ready');
assert.equal(rowForConnection('connection-linear')?.state, 'reconnect');
assert.equal(rowForConnection('connection-linear-unavailable')?.state, 'unavailable');
assert.equal(newConnectionRow('web-search')?.state, 'not_connected');
assert.equal(
  newConnectionRow('reference-library')?.connector.classification,
  'data_source',
  'the shared picker must still preserve unrelated data-source classifications'
);
for (const locale of ['en-US', 'zh-Hans']) {
  const localizedRows = createConnectorPickerRows(
    catalog.map(item => localizeConnectorCatalogItem(item, locale)),
    connections,
    initialBindings
  );
  for (const query of ['Exa', '网页搜索', 'websearch']) {
    const matches = filterConnectorPickerRows(localizedRows, { query, status: 'all' });
    assert.deepEqual(
      matches.map(row => row.connector.id),
      ['web-search'],
      `${locale}: ${query}`
    );
    assert.equal(matches[0].connector.classification, 'connector');
  }
}
const expiredOnlyRows = createConnectorPickerRows(
  [
    {
      ...catalog[0],
      capabilities: catalog[0].capabilities.map(item => ({ ...item, usable: false })),
    },
  ],
  [connection('connection-github', 'github', 'reconnect_required')],
  [initialBindings[0]]
);
const expiredOnlyCreateRow = expiredOnlyRows.find(row => !row.connection);
assert.equal(
  expiredOnlyCreateRow?.state,
  'not_connected',
  'unusable saved Keys must not prevent independently verifying a new Key'
);
let expiredOnlyCreateRequest;
await requestConnectorPickerAction(
  expiredOnlyCreateRow,
  {
    onRequestConnectAndAdd: request => {
      expiredOnlyCreateRequest = request;
    },
  },
  target
);
assert.equal(expiredOnlyCreateRequest?.connection, undefined);
assert.equal(expiredOnlyCreateRequest?.replacedBinding?.id, 'binding-github');
assert.equal(
  newConnectionRow('github')?.state,
  'not_connected',
  'a provider with saved connections must still expose an independent add-connection row'
);
assert.equal(
  rowForConnection('connection-github-side-project')?.replacedBinding?.id,
  'binding-github',
  'another saved connection carries the exact binding it would replace'
);
assert.equal(
  rowForConnection('connection-github-side-project')?.replacedConnection?.id,
  'connection-github',
  'the switch confirmation can name the currently selected connection'
);

assert.deepEqual(
  filterConnectorPickerRows(initialRows, { query: '', status: 'connected' }).map(
    row => row.connection?.id ?? row.connector.id
  ),
  ['connection-github', 'connection-github-side-project', 'connection-notion'],
  'Connected includes ready and added native connections'
);
assert.deepEqual(
  filterConnectorPickerRows(initialRows, { query: '', status: 'needs_attention' }).map(
    row => row.connection?.id ?? row.connector.id
  ),
  ['connection-linear', 'connection-linear-unavailable'],
  'Needs attention contains reconnect and unavailable saved connections'
);
assert.deepEqual(
  filterConnectorPickerRows(initialRows, { query: 'live web', status: 'all' }).map(
    row => row.connector.id
  ),
  ['web-search']
);

const notionRow = rowForConnection('connection-notion');
assert.ok(notionRow);

let agentDraft = [...initialBindings];
let addRequest;
await requestConnectorPickerAction(
  notionRow,
  {
    onRequestAdd: request => {
      addRequest = request;
      agentDraft = [
        ...agentDraft,
        binding('binding-notion', request.connection.id, request.connector.id),
      ];
    },
  },
  target
);

assert.deepEqual(addRequest?.target, target, 'the host receives the target context it controls');
assert.equal(addRequest?.connection.id, 'connection-notion');
assert.equal(initialBindings.length, 1, 'the picker must not mutate the host Agent draft');
assert.equal(
  createConnectorPickerRows(catalog, connections, initialBindings).find(
    row => row.connector.id === 'notion'
  )?.state,
  'ready',
  'a row remains ready until the host supplies its updated draft'
);
assert.equal(
  createConnectorPickerRows(catalog, connections, agentDraft).find(
    row => row.connector.id === 'notion'
  )?.state,
  'added',
  'the controlled row becomes added only after the host updates the draft'
);

let connectRequest;
const exaRow = initialRows.find(row => row.connector.id === 'web-search');
assert.ok(exaRow);
await requestConnectorPickerAction(
  exaRow,
  {
    onRequestConnectAndAdd: request => {
      connectRequest = request;
    },
  },
  target
);
assert.equal(connectRequest?.connector.id, 'web-search');
assert.deepEqual(connectRequest?.target, target);
assert.equal(
  connectRequest?.connection,
  undefined,
  'not-connected rows hand setup to the host without synthesizing a Connection'
);

let reconnectRequest;
const linearRow = initialRows.find(row => row.connector.id === 'linear');
assert.ok(linearRow);
await requestConnectorPickerAction(
  linearRow,
  {
    onRequestUpdateKey: request => {
      reconnectRequest = request;
    },
  },
  target
);
assert.equal(reconnectRequest?.connection.id, 'connection-linear');
assert.deepEqual(reconnectRequest?.target, target);

let unavailableUpdateRequest;
await requestConnectorPickerAction(
  rowForConnection('connection-linear-unavailable'),
  {
    onRequestUpdateKey: request => {
      unavailableUpdateRequest = request;
    },
    onRequestAdd: () => {
      throw new Error('an unavailable connection must not be selectable');
    },
  },
  target
);
assert.equal(unavailableUpdateRequest?.connection.id, 'connection-linear-unavailable');

let switchRequest;
let switchDispatchedAsAdd = false;
await requestConnectorPickerAction(
  rowForConnection('connection-github-side-project'),
  {
    onRequestAdd: () => {
      switchDispatchedAsAdd = true;
    },
    onRequestSwitch: request => {
      switchRequest = request;
    },
  },
  target
);
assert.equal(switchDispatchedAsAdd, false, 'switching is a distinct, confirmable action');
assert.equal(switchRequest?.connection.id, 'connection-github-side-project');
assert.equal(switchRequest?.replacedBinding.id, 'binding-github');
assert.equal(switchRequest?.replacedBinding.revision, 7, 'the stale-safe revision is preserved');
assert.equal(switchRequest?.replacedConnection?.id, 'connection-github');
assert.ok(switchRequest);
assert.equal(
  isConnectorPickerSwitchCurrent(switchRequest, connections, initialBindings, target),
  true,
  'only the unchanged slot and verified connection can be confirmed'
);
assert.equal(
  isConnectorPickerSwitchCurrent(
    switchRequest,
    connections,
    initialBindings.map(item =>
      item.id === 'binding-github' ? { ...item, revision: item.revision + 1 } : item
    ),
    target
  ),
  false,
  'a binding revision change after opening confirmation must not overwrite newer selection'
);
assert.equal(
  isConnectorPickerSwitchCurrent(switchRequest, connections, initialBindings, {
    ...target,
    id: 'another-agent',
  }),
  false,
  'a pending switch cannot leak into another host target'
);
assert.equal(
  isConnectorPickerSwitchCurrent(
    switchRequest,
    connections.map(item =>
      item.id === switchRequest.connection.id ? { ...item, state: 'reconnect_required' } : item
    ),
    initialBindings,
    target
  ),
  false,
  'a connection that loses readiness before confirmation is not selected'
);

let forbiddenAddedAction = false;
const githubRow = initialRows.find(row => row.connector.id === 'github');
assert.ok(githubRow);
await requestConnectorPickerAction(githubRow, {
  onRequestAdd: () => {
    forbiddenAddedAction = true;
  },
});
assert.equal(
  forbiddenAddedAction,
  false,
  'Added is disabled and cannot create a duplicate binding'
);

const root = process.cwd();
const pickerSource = readFileSync(
  path.join(root, 'src/components/connectors/picker/connector-picker-dialog.tsx'),
  'utf8'
);

const confirmBody = pickerSource.match(
  /const confirmSwitch = async \(\) => \{([\s\S]*?)\n {2}\};/
)?.[1];
assert.ok(confirmBody, 'exercise the actual confirmation callback used by the shared picker');
const performBody = pickerSource.match(
  /const perform = async \(key: string, action: \(\) => void \| Promise<void>, handoff = false\) => \{([\s\S]*?)\n {2}\};/
)?.[1];
assert.ok(performBody, 'execute the shared async guard with the real confirmation callback');
const confirmSwitch = new Function(
  'dependencies',
  `const {
    pendingSwitch, switchPending, connections, bindings, target,
    isConnectorPickerSwitchCurrent, setPendingSwitch, setActionFailed,
    setSwitchPending, onRequestSwitch, setPendingKey, open = true, staleSession = false,
  } = dependencies;
  const groupByConnector = false;
  const busyRef = { current: false };
  const mountedRef = { current: true };
  const restoreFocusRef = { current: true };
  const session = {};
  const sessionRef = { current: staleSession ? {} : session };
  const perform = async (key, action, handoff = false) => { ${performBody} };
  return (async () => { ${confirmBody} })();`
);
async function exerciseConfirmation(
  request,
  currentBindings = initialBindings,
  fail = false,
  overrides = {}
) {
  const calls = [];
  const states = [];
  await confirmSwitch({
    pendingSwitch: request,
    switchPending: false,
    connections,
    bindings: currentBindings,
    target,
    isConnectorPickerSwitchCurrent,
    setPendingSwitch: value => states.push(['pending', value]),
    setActionFailed: value => states.push(['failed', value]),
    setSwitchPending: value => states.push(['busy', value]),
    setPendingKey: value => states.push(['pending-key', value]),
    onRequestSwitch: value => {
      calls.push(value);
      if (fail) throw new Error('revision changed');
    },
    ...overrides,
  });
  return { calls, states };
}

assert.equal(
  (await exerciseConfirmation(undefined)).calls.length,
  0,
  'cancelling the pending choice never invokes a host mutation'
);
assert.equal(
  (await exerciseConfirmation(switchRequest)).calls.length,
  1,
  'the confirmed switch invokes the host once'
);
assert.equal(
  (await exerciseConfirmation(switchRequest, initialBindings, false, { open: false })).calls.length,
  0,
  'closed dialog cannot submit a stale confirmation'
);
assert.equal(
  (await exerciseConfirmation(switchRequest, initialBindings, false, { staleSession: true })).calls
    .length,
  0,
  'reopened or changed-scope dialog rejects a stale confirmation'
);
const staleConfirmation = await exerciseConfirmation(
  switchRequest,
  initialBindings.map(item =>
    item.id === 'binding-github' ? { ...item, revision: item.revision + 1 } : item
  )
);
assert.equal(staleConfirmation.calls.length, 0);
assert.ok(staleConfirmation.states.some(([kind, value]) => kind === 'failed' && value));
const failedConfirmation = await exerciseConfirmation(switchRequest, initialBindings, true);
assert.equal(failedConfirmation.calls.length, 1, 'a failed switch is not retried automatically');
assert.ok(failedConfirmation.states.some(([kind, value]) => kind === 'failed' && value));

const workflowTargetA = {
  type: 'workflow',
  id: 'workflow-research',
  binding_instance_id: 'node-a',
};
const workflowTargetB = { ...workflowTargetA, binding_instance_id: 'node-b' };
const nodeABinding = {
  ...binding('node-a-binding', 'connection-github', 'github'),
  target: workflowTargetA,
};
const nodeBBinding = {
  ...binding('node-b-binding', 'connection-github-side-project', 'github'),
  target: workflowTargetB,
};
assert.equal(
  createConnectorPickerRows(catalog, connections, [nodeABinding]).find(
    row => row.connection?.id === 'connection-github'
  )?.state,
  'added'
);
assert.equal(
  createConnectorPickerRows(catalog, connections, [nodeBBinding]).find(
    row => row.connection?.id === 'connection-github-side-project'
  )?.state,
  'added',
  'different workflow nodes can explicitly select different connections for the same provider'
);
assert.equal(
  isConnectorPickerSwitchCurrent(
    { ...switchRequest, target: workflowTargetA, replacedBinding: nodeABinding },
    connections,
    [nodeABinding],
    workflowTargetB
  ),
  false,
  'a node A confirmation cannot change node B'
);

assert.doesNotMatch(pickerSource, /components\/integrations/);
assert.doesNotMatch(
  pickerSource,
  /connectorService|attachTargetBinding|createTargetBinding/,
  'the shared picker must not persist behind the Agent editor'
);
assert.match(pickerSource, /onRequestAdd/);
assert.match(pickerSource, /onRequestSwitch/);
assert.match(pickerSource, /onRequestConnectAndAdd/);
assert.match(pickerSource, /onRequestUpdateKey/);
assert.match(pickerSource, /ConfirmDialog/);
assert.match(pickerSource, /pendingSwitch/);
assert.match(pickerSource, /isConnectorPickerSwitchCurrent/);
assert.doesNotMatch(
  pickerSource.toLowerCase(),
  /oauth|qr code|organization connection|permission matrix|allowed_action_ids/,
  'the first-release picker must stay Key-only, Personal-only, and policy-free'
);

console.log('Connector picker binding and host-controlled draft checks passed.');
