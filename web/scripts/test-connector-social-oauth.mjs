import assert from 'node:assert/strict';
import { harness, jsx, load, nodes, ui, flush } from './connector-fixture.mjs';
const state = load('src/components/connectors/setup/social-oauth-state.ts');
for (const id of ['x-social', 'facebook-social', 'linkedin-social', 'github'])
  assert.equal(state.supportsSocialOAuth({ id }), true);
assert.equal(state.supportsSocialOAuth({ id: 'x' }), false);
assert.equal(state.supportsSocialOAuth({ id: 'github-mcp' }), false);
for (const status of ['pending', 'processing', 'select_page', 'install_app'])
  assert.equal(state.isOAuthFlowActive({ status }), true);
for (const status of ['ready', 'failed']) assert.equal(state.isOAuthFlowActive({ status }), false);
for (const url of [
  'javascript:alert(1)',
  'http://example.com',
  'https://user:password@example.com',
])
  assert.throws(() => state.safeOAuthAuthorizeURL(url));
assert.equal(
  state.safeOAuthAuthorizeURL('https://example.com/authorize'),
  'https://example.com/authorize'
);
let flow,
  blocked = false,
  startCalls = 0,
  completed = 0,
  notified = 0,
  opens = 0,
  closes = 0;
let resolveStart;
const popup = {
  opener: 'unsafe',
  closed: false,
  close() {
    closes++;
  },
  location: {
    replace(url) {
      this.url = url;
    },
  },
};
globalThis.window = {
  open() {
    opens++;
    return blocked ? null : popup;
  },
  setInterval() {
    return 1;
  },
  clearInterval() {},
};
const controller = harness(react => {
  const { ConnectorSocialOAuthPanel } = load(
    'src/components/connectors/setup/connector-social-oauth-panel.tsx',
    {
      react,
      'react/jsx-runtime': jsx,
      '@tanstack/react-query': {
        useQuery: () => ({ data: { data: { available: true } }, refetch() {} }),
      },
      '@/components/ui/button': ui,
      '@/components/ui/card': ui,
      '@/hooks/query-keys': { CONNECTOR_KEYS: { all: ['connectors'] } },
      '@/hooks/connectors/use-connector-oauth': {
        useConnectorOAuthFlow: () => ({ flow, query: { refetch() {} } }),
      },
      '@/i18n': { useT: () => key => key },
      '@/services/connector.service': {
        connectorService: {
          startOAuth: () => {
            startCalls++;
            return new Promise(resolve => {
              resolveStart = resolve;
            });
          },
          getConnection: async () => ({ data: { id: 'connection', connector_id: 'x-social' } }),
        },
      },
      './connector-oauth-flow-status': { ConnectorOAuthFlowStatus: 'FlowStatus' },
      './social-oauth-state': state,
      './use-creation-default': {
        useCreationDefault: () => ({
          begin() {},
          option() {},
          complete: async () => {
            completed++;
            return true;
          },
        }),
      },
    }
  );
  return () =>
    ConnectorSocialOAuthPanel({
      connector: { id: 'x-social' },
      onReady() {
        notified++;
      },
      manualFallback: 'manual',
    });
});
const button = label =>
  nodes(controller.render()).find(n => n.type === 'Button' && n.props.children === label);
blocked = true;
button('oauth.connect').props.onClick();
assert.equal(startCalls, 0, 'blocked popup must not create an orphan server flow');
assert(nodes(controller.render()).some(n => n.props.children === 'oauth.blocked'));
blocked = false;
button('oauth.connect').props.onClick();
assert.equal(opens, 2);
assert.equal(startCalls, 1);
assert.equal(popup.opener, null);
assert.equal(popup.location.url, undefined);
resolveStart({
  data: {
    id: 'flow',
    connector_id: 'x-social',
    authorize_url: 'https://provider.example/authorize',
  },
});
await flush();
assert.equal(popup.location.url, 'https://provider.example/authorize');
for (const status of ['pending', 'processing']) {
  flow = { id: 'flow', connector_id: 'x-social', status };
  const activeTree = controller.render();
  await flush();
  assert.equal(notified, 0, 'processing must not notify before credentials are saved');
  assert(
    nodes(activeTree).find(n => n.type === 'Button' && n.props.children === 'oauth.connect').props
      .disabled
  );
}
flow = { id: 'flow', connector_id: 'x-social', status: 'ready', connection_id: 'connection' };
controller.render();
await flush();
controller.render();
await flush();
assert.equal(completed, 1, 'apply frozen creation preference once');
assert.equal(notified, 1, 'notify embedded host once without creating a target binding');
controller.unmount();
assert(closes > 0);
console.log('Social OAuth UI lifecycle checks passed');

// API status responses carry only flow state. Starting authorization needs the
// browser-binding cookie, while no global credential policy is changed.
const requests = [];
const { connectorService } = load('src/services/connector.service.ts', {
  '@/lib/http/services': {
    BaseService: class {
      request(...args) {
        requests.push(args);
        return Promise.resolve({ data: {} });
      }
    },
  },
});
await connectorService.startOAuth('x-social', 'previous');
assert.deepEqual(requests.at(-1).slice(0, 3), [
  'post',
  '/connectors/x-social/oauth',
  { connection_id: 'previous' },
]);
assert.equal(requests.at(-1)[3].withCredentials, true);
assert.equal(requests.at(-1)[3].retryAttemptsOverride, 0);
await connectorService.selectOAuthPage('flow', 'page');
assert.deepEqual(requests.at(-1).slice(0, 3), [
  'post',
  '/connectors/oauth-flows/flow/page',
  { page_id: 'page' },
]);
assert.equal(requests.at(-1)[3].withCredentials, undefined);
await connectorService.completeOAuthInstallation('flow/encoded');
assert.deepEqual(requests.at(-1).slice(0, 3), [
  'post',
  '/connectors/oauth-flows/flow%2Fencoded/installation',
  {},
]);
assert.equal(requests.at(-1)[3].retryAttemptsOverride, 0);
console.log('Social OAuth service contract checks passed');

let installationCompletions = 0,
  installationError = false;
let selection,
  selectionError = false,
  statusFlow = { status: 'select_page', pages: [{ id: 'p1', name: 'My Page' }] };
const { ConnectorOAuthFlowStatus } = load(
  'src/components/connectors/setup/connector-oauth-flow-status.tsx',
  {
    'react/jsx-runtime': jsx,
    'next/link': { default: 'Link' },
    './social-oauth-state': state,
    '@/components/ui/button': ui,
    '@/i18n': { useT: () => key => key },
    '@/hooks/connectors/use-connector-oauth': {
      useConnectorOAuthFlow: () => ({
        flow: statusFlow,
        query: {},
        completeInstallation: {
          isError: installationError,
          isPending: false,
          mutate() {
            installationCompletions++;
          },
        },
        selectPage: {
          isError: selectionError,
          mutate(id) {
            selection = id;
          },
        },
      }),
    },
  }
);
let tree = ConnectorOAuthFlowStatus({ flowId: 'flow' });
const pageButton = nodes(tree).find(n => n.type === 'Button' && n.props.children === 'My Page');
pageButton.props.onClick();
assert.equal(selection, 'p1');
statusFlow = {
  status: 'install_app',
  install_url: 'https://github.com/apps/zgi-connect/installations/new',
};
tree = ConnectorOAuthFlowStatus({ flowId: 'flow' });
const installLink = nodes(tree).find(n => n.type === 'a');
assert.equal(installLink.props.href, statusFlow.install_url);
assert.equal(installLink.props.target, '_blank');
assert.equal(installLink.props.rel, 'noreferrer');
const resume = nodes(tree).find(
  n => n.type === 'Button' && n.props.children === 'oauth.completeInstallation'
);
resume.props.onClick();
assert.equal(installationCompletions, 1);
installationError = true;
assert(
  nodes(ConnectorOAuthFlowStatus({ flowId: 'flow' })).some(
    n => n.props.children === 'oauth.installationFailed'
  )
);
for (const install_url of [
  'javascript:alert(1)',
  'https://evil.example/apps/zgi-connect/installations/new',
  'https://github.com/other',
]) {
  statusFlow = { status: 'install_app', install_url };
  tree = ConnectorOAuthFlowStatus({ flowId: 'flow' });
  assert(!nodes(tree).some(n => n.type === 'a'), 'untrusted installation URL must not render');
}
statusFlow = { status: 'ready', connection_id: 'different-account' };
tree = ConnectorOAuthFlowStatus({ flowId: 'flow', reconnectId: 'original-account' });
assert(nodes(tree).some(n => n.props.children === 'oauth.changedIdentity'));
statusFlow = { status: 'ready', connection_id: 'original-account' };
tree = ConnectorOAuthFlowStatus({ flowId: 'flow', reconnectId: 'original-account' });
assert(nodes(tree).some(n => n.props.children === 'oauth.ready'));
console.log('Social OAuth Page selection and reconnect identity checks passed');

// Both the callback and opener use this hook, so processing must keep polling
// even while provider exchange and native credential verification are running.
let queryOptions,
  mutationOptions,
  invalidations = 0,
  statusReads = 0;
let remoteFlow = { id: 'shared-flow', status: 'pending' };
const queryResult = {
  data: { data: remoteFlow },
  async refetch() {
    statusReads++;
    queryResult.data = { data: remoteFlow };
    return { data: queryResult.data };
  },
};
const hook = harness(react => {
  const { useConnectorOAuthFlow } = load('src/hooks/connectors/use-connector-oauth.ts', {
    react,
    '@tanstack/react-query': {
      useQuery(options) {
        queryOptions = options;
        return queryResult;
      },
      useMutation(options) {
        mutationOptions = options;
        return { options };
      },
      useQueryClient: () => ({
        setQueryData(_key, data) {
          queryResult.data = data;
        },
      }),
    },
    '@/hooks/query-keys': { CONNECTOR_KEYS: { all: ['connectors'] } },
    '@/services/connector.service': {
      connectorService: {
        completeOAuthInstallation: async id => ({ data: { id, status: 'ready' } }),
      },
    },
    '@/components/connectors/setup/social-oauth-state': state,
    './connection-detail-query': {
      invalidatePersonalConnectionQueries() {
        invalidations++;
      },
    },
  });
  return () => useConnectorOAuthFlow('shared-flow');
});
for (const status of ['pending', 'processing', 'install_app', 'ready']) {
  queryResult.data = { data: { id: 'shared-flow', status } };
  hook.render();
  assert.equal(
    queryOptions.refetchInterval({ state: { data: queryResult.data } }),
    status === 'ready' ? false : 1500
  );
}
assert(invalidations > 0);
const installationMutation = hook.render().completeInstallation.options;
assert.deepEqual(await installationMutation.mutationFn(), {
  data: { id: 'shared-flow', status: 'ready' },
});
installationMutation.onSuccess({ data: { id: 'shared-flow', status: 'install_app' } });
assert.equal(queryResult.data.data.status, 'install_app');
// Another window won the Page selection race; the losing mutation must read
// the saved status and render success, without leaving a stale Page failure.
queryResult.data = { data: { id: 'shared-flow', status: 'select_page' } };
hook.render();
remoteFlow = { id: 'shared-flow', status: 'ready', connection_id: 'original-account' };
await mutationOptions.onError(new Error('selection already completed'));
assert.equal(statusReads, 1);
assert.equal(hook.render().flow.status, 'ready');
statusFlow = queryResult.data.data;
selectionError = true;
tree = ConnectorOAuthFlowStatus({ flowId: 'shared-flow', reconnectId: 'original-account' });
assert(nodes(tree).some(n => n.props.children === 'oauth.ready'));
assert(!nodes(tree).some(n => n.props.children === 'oauth.pageFailed'));
for (const status of ['pending', 'processing']) {
  statusFlow = { status };
  tree = ConnectorOAuthFlowStatus({ flowId: 'shared-flow' });
  assert(nodes(tree).some(n => n.props.children === 'oauth.waiting'));
}
hook.unmount();
console.log('Social OAuth processing polling and concurrent Page selection checks passed');
