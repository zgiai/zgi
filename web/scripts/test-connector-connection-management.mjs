import { connectorPureImport } from './connector-pure-fixture.mjs';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { QueryClient, QueryObserver } from '@tanstack/react-query';
import ts from 'typescript';
import { CONNECTOR_KEYS } from '../src/hooks/query-keys.ts';
import {
  applyKeySetupResult,
  createKeySetupState,
} from '../src/components/connectors/setup/key-setup-state.ts';
import {
  buildRenameConnectionRequest,
  connectorBindingTargetHref,
  getConnectionManagementPageState,
  getConnectionRemovalMode,
  sortConnectionBindings,
} from '../src/components/connectors/connections/connection-management-model.ts';
const {
  groupPersonalConnections,
  personalConnectionDisplayName,
  personalConnectionSecondaryIdentity,
} = connectorPureImport('personal-connections-model');
import {
  applyConnectionReconnectResult,
  beginConnectionReconnect,
  canSubmitConnectionReconnect,
  confirmConnectionReconnectIdentity,
  createConnectionReconnectState,
  failConnectionReconnect,
  requireConnectionReconnectConfirmation,
  resetConnectionReconnect,
  updateConnectionReconnectValue,
} from '../src/components/connectors/connections/connection-reconnect-state.ts';

const root = process.cwd();
const serviceSource = readFileSync(
  path.join(root, 'src', 'services', 'connector.service.ts'),
  'utf8'
);
const reconnectHookSource = readFileSync(
  path.join(root, 'src', 'hooks', 'connectors', 'use-connector-reconnect.ts'),
  'utf8'
);

assert.match(serviceSource, /renameConnection[\s\S]*'patch'[\s\S]*\/connector-connections\//);
assert.match(
  serviceSource,
  /reconnectConnection[\s\S]*\/reconnect[\s\S]*'Idempotency-Key'/,
  'reconnect must use the dedicated idempotent candidate-Key command'
);
assert.match(serviceSource, /diagnoseConnection[\s\S]*\/diagnostics/);
assert.match(serviceSource, /getRemovalImpact[\s\S]*\/removal-impact/);
assert.match(serviceSource, /removeConnection[\s\S]*'X-Impact-Token'/);
assert.match(reconnectHookSource, /destroyValues\(requestValues\)/);
assert.match(
  reconnectHookSource,
  /finally\s*\{[\s\S]*destroyValues\(requestValues\)/,
  'candidate Key copies must be destroyed on every request outcome'
);
assert.doesNotMatch(
  reconnectHookSource,
  /localStorage|sessionStorage|persistQueryClient/,
  'Reconnect Key values must never enter persistent client storage'
);

const readyConnection = {
  id: '11111111-1111-4111-8111-111111111111',
  connector_id: 'github',
  name: 'octocat',
  external_display_name: '@octocat',
  state: 'ready',
  credential_configured: true,
  revision: 7,
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
  capabilities: [],
  bindings: [],
};

assert.equal(
  getConnectionManagementPageState({ enabled: false, featureLoading: false }),
  'disabled'
);
assert.equal(getConnectionManagementPageState({ enabled: false, featureLoading: true }), 'loading');
assert.equal(getConnectionManagementPageState({ enabled: true, isLoading: true }), 'loading');
assert.equal(
  getConnectionManagementPageState({
    enabled: true,
    error: { response: { status: 500 } },
  }),
  'error'
);
assert.equal(
  getConnectionManagementPageState({
    enabled: true,
    error: { response: { status: 404 } },
  }),
  'not_found'
);
assert.equal(getConnectionManagementPageState({ enabled: true }), 'not_found');
assert.equal(
  getConnectionManagementPageState({ enabled: true, connection: readyConnection }),
  'ready'
);
assert.equal(
  getConnectionManagementPageState({
    enabled: true,
    connection: { ...readyConnection, state: 'reconnect_required' },
  }),
  'reconnect_required'
);
assert.equal(
  getConnectionManagementPageState({
    enabled: true,
    connection: { ...readyConnection, state: 'unavailable' },
  }),
  'unavailable'
);

const bindings = [
  {
    id: 'workflow-binding',
    connection_id: readyConnection.id,
    connector_id: 'github',
    target: { type: 'workflow', id: 'workflow-1', binding_instance_id: 'node-4' },
    target_name: 'Weekly summary',
    status: 'needs_replacement',
    compatibility: 'native',
  },
  {
    id: 'chat-binding',
    connection_id: readyConnection.id,
    connector_id: 'github',
    target: { type: 'chat', id: 'conversation 1' },
    target_name: 'Product assistant',
    status: 'active',
    compatibility: 'native',
  },
  {
    id: 'agent-binding',
    connection_id: readyConnection.id,
    connector_id: 'github',
    target: { type: 'agent', id: 'agent/1' },
    target_name: 'Support agent',
    status: 'disabled',
    compatibility: 'native',
  },
];

assert.deepEqual(
  sortConnectionBindings(bindings).map(binding => binding.id),
  ['chat-binding', 'agent-binding', 'workflow-binding'],
  'bindings must put active usage first and remain deterministic'
);
assert.equal(connectorBindingTargetHref(bindings[1]), '/console/work/chat?convId=conversation%201');
assert.equal(connectorBindingTargetHref(bindings[2]), '/console/agents/agent%2F1');
assert.equal(
  connectorBindingTargetHref(bindings[0]),
  '/console/workflows/workflow-1?nodeId=node-4'
);

assert.equal(
  getConnectionRemovalMode({
    connection_id: readyConnection.id,
    can_remove: true,
    affected_targets: [],
  }),
  'normal'
);
assert.equal(
  getConnectionRemovalMode({
    connection_id: readyConnection.id,
    can_remove: false,
    impact_token: 'impact-token',
    affected_targets: bindings,
  }),
  'force'
);
assert.equal(
  getConnectionRemovalMode({
    connection_id: readyConnection.id,
    can_remove: false,
    affected_targets: bindings,
  }),
  'blocked',
  'force removal must remain disabled without a server-issued impact token'
);
assert.equal(
  getConnectionRemovalMode(
    {
      connection_id: readyConnection.id,
      can_remove: false,
      impact_token: 'expired-impact-token',
      expires_at: '2026-09-01T00:00:00Z',
      affected_targets: bindings,
    },
    Date.parse('2026-09-01T00:01:00Z')
  ),
  'blocked',
  'an expired impact token must never authorize force removal'
);

assert.deepEqual(buildRenameConnectionRequest(readyConnection, '  Work GitHub  '), {
  revision: 7,
  name: 'Work GitHub',
});
assert.equal(buildRenameConnectionRequest(readyConnection, '   '), null);
assert.equal(buildRenameConnectionRequest(readyConnection, 'x'.repeat(129)), null);

const secondConnection = {
  ...readyConnection,
  id: '55555555-5555-4555-8555-555555555555',
  name: 'Documentation research',
  state: 'reconnect_required',
  revision: 2,
};
const exaConnection = {
  ...readyConnection,
  id: '66666666-6666-4666-8666-666666666666',
  connector_id: 'web-search',
  name: 'Daily research',
  external_display_name: undefined,
};
assert.equal(personalConnectionDisplayName(readyConnection, 'Unnamed connection'), 'octocat');
assert.equal(
  personalConnectionSecondaryIdentity(readyConnection, 'Verified account'),
  '@octocat',
  'the external account remains secondary to the user-defined connection name'
);
assert.deepEqual(
  groupPersonalConnections([exaConnection, secondConnection, readyConnection]).map(group => ({
    connectorID: group.connectorID,
    connections: group.connections.map(connection => connection.id),
  })),
  [
    {
      connectorID: 'github',
      connections: [secondConnection.id, readyConnection.id],
    },
    { connectorID: 'web-search', connections: [exaConnection.id] },
  ]
);
assert.equal(secondConnection.state, 'reconnect_required');
assert.equal(readyConnection.state, 'ready');

const fields = [{ name: 'token', label: 'Personal access token', required: true, secret: true }];
const secretSentinel = 'connector-reconnect-secret-must-disappear';
let reconnectState = createConnectionReconnectState();
reconnectState = updateConnectionReconnectValue(reconnectState, 'token', secretSentinel);
assert.equal(canSubmitConnectionReconnect(reconnectState, fields), true);
reconnectState = beginConnectionReconnect(reconnectState);
assert.equal(reconnectState.status, 'verifying');
assert.deepEqual(reconnectState.values, {});
assert.equal(JSON.stringify(reconnectState).includes(secretSentinel), false);

reconnectState = requireConnectionReconnectConfirmation();
assert.equal(reconnectState.status, 'confirmation_required');
assert.deepEqual(reconnectState.values, {});
reconnectState = confirmConnectionReconnectIdentity();
assert.equal(reconnectState.confirmUnstableIdentity, true);
reconnectState = updateConnectionReconnectValue(reconnectState, 'token', secretSentinel);
reconnectState = beginConnectionReconnect(reconnectState);
assert.equal(reconnectState.confirmUnstableIdentity, true);
assert.deepEqual(reconnectState.values, {});
assert.equal(JSON.stringify(reconnectState).includes(secretSentinel), false);

reconnectState = applyConnectionReconnectResult(reconnectState, readyConnection.id, {
  attempt_id: '22222222-2222-4222-8222-222222222222',
  state: 'ready',
  connection: { ...readyConnection, credential_version: 2, revision: 8 },
});
assert.equal(reconnectState.status, 'ready');
assert.deepEqual(reconnectState.values, {});

reconnectState = resetConnectionReconnect();
reconnectState = updateConnectionReconnectValue(reconnectState, 'token', secretSentinel);
reconnectState = beginConnectionReconnect(reconnectState);
reconnectState = applyConnectionReconnectResult(reconnectState, readyConnection.id, {
  attempt_id: '33333333-3333-4333-8333-333333333333',
  state: 'ready',
  connection: {
    ...readyConnection,
    id: '44444444-4444-4444-8444-444444444444',
    external_display_name: '@different-account',
  },
});
assert.equal(
  reconnectState.status,
  'new_connection',
  'a different external identity must be represented as a new unbound Connection'
);

reconnectState = resetConnectionReconnect();
reconnectState = updateConnectionReconnectValue(reconnectState, 'token', secretSentinel);
reconnectState = beginConnectionReconnect(reconnectState);
reconnectState = failConnectionReconnect(reconnectState, 'provider_unavailable', true);
assert.equal(reconnectState.status, 'failed');
assert.equal(JSON.stringify(reconnectState).includes(secretSentinel), false);
reconnectState = resetConnectionReconnect();
assert.deepEqual(reconnectState.values, {});

reconnectState = updateConnectionReconnectValue(reconnectState, 'token', secretSentinel);
reconnectState = beginConnectionReconnect(reconnectState);
reconnectState = failConnectionReconnect(reconnectState, 'transport_unconfirmed', true, true);
assert.equal(reconnectState.status, 'unknown');
assert.equal(reconnectState.attemptId, undefined);
assert.deepEqual(reconnectState.values, {});
assert.equal(JSON.stringify(reconnectState).includes(secretSentinel), false);
assert.equal(canSubmitConnectionReconnect(reconnectState, fields), false);
reconnectState = resetConnectionReconnect();
assert.equal(reconnectState.status, 'idle');
assert.equal(canSubmitConnectionReconnect(reconnectState, fields), false);

const readSource = relativePath => readFileSync(path.join(root, relativePath), 'utf8');
const moduleURL = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`;
const transpile = source =>
  ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText;
const queryKeysURL = moduleURL(transpile(readSource('src/hooks/query-keys.ts')));
const detailHelpersSource = transpile(
  readSource('src/hooks/connectors/connection-detail-query.ts')
).replace("'@/hooks/query-keys'", JSON.stringify(queryKeysURL));
const {
  invalidatePersonalConnectionQueries,
  personalConnectionDetailQueryOptions,
  requirePersonalConnectionDetailResponse,
  requirePersonalConnectionRemovalImpactResponse,
} = await import(moduleURL(detailHelpersSource));

const setupHookSource = readSource('src/hooks/connectors/use-connector-setup.ts');
const connectorHooksSource = readSource('src/hooks/connectors/use-connectors.ts');
const finishBody = setupHookSource.match(
  /const finish = useCallback\(\s*\(result: KeySetupResult\) => \{([\s\S]*?)\n {4}\},/
)?.[1];
assert.ok(finishBody, 'the setup success callback must be exercised by this regression');
const finishSetup = new Function(
  'result',
  'mountedRef',
  'setState',
  'applyKeySetupResult',
  'queryClient',
  'CONNECTOR_KEYS',
  'invalidatePersonalConnectionQueries',
  finishBody
);
assert.match(connectorHooksSource, /useQuery\(\s*personalConnectionDetailQueryOptions\(/);
assert.doesNotMatch(setupHookSource, /setQueryData|setQueriesData/);
const renameHook = connectorHooksSource
  .split('export function useRenamePersonalConnection(')[1]
  ?.split('export function ')[0];
assert.match(renameHook, /invalidatePersonalConnectionQueries/);
assert.doesNotMatch(renameHook, /setQueryData|setQueriesData/);

const summary = { ...readyConnection };
delete summary.capabilities;
delete summary.bindings;
const detail = {
  ...readyConnection,
  capabilities: [{ id: 'search', name: 'Search', description: 'Read-only search', usable: true }],
  bindings,
};
const detailKey = CONNECTOR_KEYS.connection(readyConnection.id);
const flushQueries = () => new Promise(resolve => globalThis.setImmediate(resolve));
const makeClient = () =>
  new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } });

function completeSetup(client) {
  let setupState = createKeySetupState();
  finishSetup(
    { state: 'ready', attempt_id: 'setup-attempt', connection: summary },
    { current: true },
    update => {
      setupState = update(setupState);
    },
    applyKeySetupResult,
    client,
    CONNECTOR_KEYS,
    invalidatePersonalConnectionQueries
  );
  assert.equal(setupState.status, 'ready');
}

for (const cacheState of ['new-setup', 'existing-detail', 'polluted-summary']) {
  const client = makeClient();
  if (cacheState === 'existing-detail') client.setQueryData(detailKey, { data: detail });
  if (cacheState === 'polluted-summary') client.setQueryData(detailKey, { data: summary });
  if (cacheState !== 'polluted-summary') completeSetup(client);
  if (cacheState === 'new-setup') {
    assert.equal(
      client.getQueryData(detailKey),
      undefined,
      'setup summaries must not seed detail caches'
    );
  } else if (cacheState === 'existing-detail') {
    assert.deepEqual(client.getQueryData(detailKey).data.bindings, bindings);
    assert.deepEqual(client.getQueryData(detailKey).data.capabilities, detail.capabilities);
    assert.equal(client.getQueryState(detailKey).isInvalidated, true);
  }

  let requests = 0;
  let resolveRequest;
  const response = new Promise(resolve => {
    resolveRequest = resolve;
  });
  const observer = new QueryObserver(
    client,
    personalConnectionDetailQueryOptions(readyConnection.id, async () => {
      requests += 1;
      return response;
    })
  );
  const unsubscribe = observer.subscribe(() => {});
  try {
    await flushQueries();
    assert.equal(requests, 1, `${cacheState} must fetch the authoritative detail`);
    if (cacheState === 'polluted-summary') {
      assert.equal(observer.getCurrentResult().isError, true);
      assert.equal(
        observer.getCurrentResult().data,
        undefined,
        'fresh summaries must not reach detail consumers'
      );
    }
    resolveRequest({ data: detail });
    await flushQueries();
    const result = observer.getCurrentResult();
    assert.equal(result.isSuccess, true);
    assert.deepEqual(result.data.data.bindings, bindings);
    assert.equal(
      result.data.data.bindings.length,
      3,
      'real usages must never be replaced by fabricated empty arrays'
    );
    assert.deepEqual(result.data.data.capabilities, detail.capabilities);
  } finally {
    unsubscribe();
    client.clear();
  }
}

{
  const client = makeClient();
  client.setQueryData(detailKey, { data: detail });
  let requests = 0;
  const observer = new QueryObserver(
    client,
    personalConnectionDetailQueryOptions(readyConnection.id, async () => {
      requests += 1;
      return { data: detail };
    })
  );
  const unsubscribe = observer.subscribe(() => {});
  await flushQueries();
  assert.equal(requests, 0, 'valid fresh details retain their normal cache lifetime');
  assert.deepEqual(observer.getCurrentResult().data.data.bindings, bindings);
  unsubscribe();
  client.clear();
}

{
  const client = makeClient();
  const observer = new QueryObserver(
    client,
    personalConnectionDetailQueryOptions(readyConnection.id, async () => ({ data: summary }))
  );
  const unsubscribe = observer.subscribe(() => {});
  await flushQueries();
  assert.equal(
    observer.getCurrentResult().isError,
    true,
    'incomplete detail responses must fail closed'
  );
  assert.equal(client.getQueryData(detailKey), undefined);
  unsubscribe();
  client.clear();
}

assert.throws(() => requirePersonalConnectionDetailResponse({ data: summary }, readyConnection.id));
assert.throws(() => requirePersonalConnectionDetailResponse({ data: detail }, 'wrong-connection'));
for (const affectedTargets of [undefined, null, {}]) {
  const malformed = {
    connection_id: readyConnection.id,
    can_remove: true,
    affected_targets: affectedTargets,
  };
  assert.throws(() =>
    requirePersonalConnectionRemovalImpactResponse({ data: malformed }, readyConnection.id)
  );
  assert.equal(getConnectionRemovalMode(malformed), 'blocked');
}
const validEmptyImpact = {
  connection_id: readyConnection.id,
  can_remove: true,
  affected_targets: [],
};
assert.deepEqual(
  requirePersonalConnectionRemovalImpactResponse({ data: validEmptyImpact }, readyConnection.id)
    .data,
  validEmptyImpact
);
assert.equal(getConnectionRemovalMode(validEmptyImpact), 'normal');
const removalDialogSource = readSource(
  'src/components/connectors/connections/connection-removal-dialog.tsx'
);
assert.match(removalDialogSource, /const impact = impactQuery\.isError \? undefined/);
assert.doesNotMatch(removalDialogSource, /impact\?\.affected_targets \?\? \[\]/);

const connectorCenterSource = readSource('src/components/connectors/catalog/connector-center.tsx');
const connectorDetailSource = readSource(
  'src/components/connectors/detail/connector-detail-page.tsx'
);
const connectionsPanelSource = readSource(
  'src/components/connectors/detail/connector-connections-panel.tsx'
);
const connectionsPageSource = readSource(
  'src/components/connectors/connections/personal-connections-page.tsx'
);
const connectionsRouteSource = readSource('src/app/console/connectors/connections/page.tsx');
const setupStatusSource = readSource('src/components/connectors/setup/connector-setup-status.tsx');
assert.match(connectorCenterSource, /\/console\/connectors\/connections/);
assert.match(connectionsRouteSource, /PersonalConnectionsPage/);
assert.match(connectionsPageSource, /usePersonalConnections/);
assert.match(connectionsPageSource, /groupPersonalConnections/);
assert.match(connectionsPageSource, /ConnectorCreateDialog/);
assert.doesNotMatch(connectionsPageSource, /credential|api[_-]?key|token/i);
assert.match(connectorDetailSource, /usePersonalConnections/);
assert.match(connectorDetailSource, /ConnectorConnectionsPanel/);
assert.doesNotMatch(connectorDetailSource, /connection_summary\.count\s*>\s*0/);
assert.match(connectionsPanelSource, /connections\.map/);
assert.match(connectionsPanelSource, /ConnectorCreateDialog/);
const createDialogSource = readSource(
  'src/components/connectors/setup/connector-create-dialog.tsx'
);
assert.doesNotMatch(createDialogSource, /setup\.addDescription|mail\.notice/);
assert.match(createDialogSource, /setup\.submit/);
assert.doesNotMatch(createDialogSource, /target=|reconnectConnection/);
assert.match(setupStatusSource, /manageConnections/);
assert.doesNotMatch(setupStatusSource, /rename=1/);
assert.match(setupStatusSource, /encodeURIComponent\(state.connection.id\)/);
assert.doesNotMatch(
  readSource('src/components/connectors/setup/connector-key-form.tsx'),
  /connection[_-]?name|account[_-]?name/i,
  'a connection name must not become a precondition for submitting a Key'
);

console.log('Connector personal Connection management state checks passed.');
