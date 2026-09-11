import assert from 'node:assert/strict';
import { load, harness, flush, nodes, jsx, ui } from './connector-fixture.mjs';

const setup = load('src/components/connectors/setup/app-setup-state.ts');
const messaging = load('src/components/connectors/setup/messaging-setup-state.ts', {
  './app-setup-state': setup,
});
const errors = load('src/hooks/connectors/connector-request-error.ts');
const connector = {
  id: 'weixin',
  name: 'WeChat',
  key_fields: [],
  extension_schema_version: '1.2.0',
  setup: {
    handler: 'weixin-registration-v1',
    protocol_version: 1,
    methods: ['qr_connect', 'qr_reauthorize'],
  },
  setup_availability: { enabled: true, methods: ['qr_connect', 'qr_reauthorize'] },
  credential_fields: [],
  capabilities: [
    { id: 'weixin.messages.pull', usable: false, unavailable_reason: 'connector_not_verified' },
  ],
  connection_summary: { count: 0 },
};
assert.equal(setup.supportsConnectorSetup(connector), true);
assert.equal(setup.supportsConnectorSetup({ ...connector, setup_availability: undefined }), true);
assert.equal(
  setup.messagingMethod({ ...connector, setup_availability: undefined }, 'qr_connect'),
  true
);
assert.equal(setup.messagingMethod(connector, 'manual'), false);
const picker = load('src/components/connectors/picker/connector-picker-model.ts', {
  '../setup/app-setup-state': setup,
  '../connector-metadata': load('src/components/connectors/connector-metadata.ts'),
});
const menu = load('src/components/connectors/picker/connector-picker-menu-model.ts', {
  './connector-picker-model': picker,
});
for (const provider of ['dingtalk', 'wecom', 'weixin']) {
  for (const count of [0, 1, 3]) {
    const selected = {
      ...connector,
      id: provider,
      setup: { ...connector.setup, handler: `${provider}-registration-v1` },
      capabilities: [{ id: `${provider}.messages.send`, usable: count > 0 }],
    };
    const accounts = Array.from({ length: count }, (_, index) => ({
      id: `${provider}-${index}`,
      connector_id: provider,
      name: `Account ${index}`,
      state: 'ready',
      revision: 1,
    }));
    const [group] = menu.createConnectorPickerMenuGroups([selected], accounts, []);
    assert.equal(group.count, count);
    assert.equal(
      group.createRow.state,
      'not_connected',
      'independent create survives account count'
    );
    assert.equal(group.canToggle, count === 1, 'multiple accounts require explicit choice');
    const actions = [];
    const handlers = {
      onRequestAdd: request => actions.push(request),
      onRequestConnectAndAdd: request => actions.push(request),
    };
    const target = { type: 'agent', id: 'unsaved-agent-draft' };
    await menu.requestConnectorPickerMenuToggle(group, handlers, target);
    assert.equal(actions.length, count === 1 ? 1 : 0);
    await picker.requestConnectorPickerAction(group.createRow, handlers, target);
    assert.equal(actions.at(-1).target.id, target.id, 'host owns attachment to its current draft');
    const unhealthy = accounts.map(account => ({ ...account, state: 'reconnect_required' }));
    const [repair] = menu.createConnectorPickerMenuGroups([selected], unhealthy, []);
    assert.equal(
      repair.createRow.state,
      'not_connected',
      'unhealthy accounts never suppress create'
    );
  }
}
assert.equal(
  setup.supportsMessagingContract({
    ...connector,
    setup: { ...connector.setup, handler: 'arbitrary-handler' },
  }),
  false
);
let server = {
  id: globalThis.crypto.randomUUID(),
  connector_id: 'weixin',
  protocol_version: 1,
  intent: 'login_session',
  state: 'awaiting_authorization',
  step: 'poll',
  revision: 3,
  retry_after: 300,
  expires_at: new Date(Date.now() + 300000).toISOString(),
  qr_payload: 'https://liteapp.weixin.qq.com/fixture-private-challenge',
  allowed_actions: ['cancel'],
};
assert.equal(messaging.messagingQR(connector, server), server.qr_payload);
for (const raw of [
  'https://liteapp.weixin.qq.com.evil.invalid/x',
  'https://user@liteapp.weixin.qq.com/x',
  'http://liteapp.weixin.qq.com/x',
  'https://127.0.0.1/x',
])
  assert.equal(messaging.messagingQR(connector, { ...server, qr_payload: raw }), undefined);
assert.equal(messaging.messagingQR(connector, { ...server, expires_at: '2000-01-01' }), undefined);
assert.equal(messaging.messagingQR(connector, { ...server, protocol_version: 99 }), undefined);

let actor = 'owner-1',
  org = 'org-1';
const auth = { use: { user: () => ({ id: actor }) }, getState: () => ({ user: { id: actor } }) };
const organizations = {
  use: { currentOrganization: () => ({ id: org }) },
  getState: () => ({ currentOrganization: { id: org } }),
};
const storage = new Map();
globalThis.sessionStorage = {
  getItem: k => storage.get(k) ?? null,
  setItem: (k, v) => storage.set(k, v),
  removeItem: k => storage.delete(k),
};
const posts = [],
  actions = [],
  gets = [],
  cancels = [];
let hold;
const service = {
  async startOnboarding(id, request, key, connection) {
    posts.push({ id, request, key, connection });
    return { data: { ...server } };
  },
  async getOnboarding(id) {
    gets.push(id);
    return { data: { ...server } };
  },
  async getConnection(id) {
    return { data: { id, connector_id: 'weixin', name: 'Saved', state: 'ready', revision: 1 } };
  },
  async submitOnboardingCode(flow, code) {
    actions.push({ flow, code });
    if (hold) return hold;
    server = { ...server, step: 'poll', revision: server.revision + 1 };
    return { data: { ...server } };
  },
  async cancelOnboarding(id, revision) {
    cancels.push({ id, revision });
    return { data: { ...server, state: 'cancelled' } };
  },
};
const queryClient = {};
const mount = (update, selected = connector) =>
  harness(react => {
    const hook = load('src/hooks/connectors/use-connector-onboarding.ts', {
      react,
      '@tanstack/react-query': { useQueryClient: () => queryClient },
      '@/store/auth-store': { useAuthStore: auth },
      '@/store/organization-store': { useOrganizationStore: organizations },
      '@/services/connector.service': { connectorService: service },
      '@/components/connectors/setup/app-setup-state': setup,
      './connector-request-error': errors,
      './connection-detail-query': { invalidatePersonalConnectionQueries() {} },
    });
    return () => hook.useConnectorOnboarding(selected, update);
  });
async function settle(h) {
  for (let i = 0; i < 4; i++) {
    await flush();
    h.render();
  }
}
let h = mount();
h.render();
await settle(h);
assert.equal(posts.length, 0, 'opening a panel must not register');
await h.value.start();
await settle(h);
assert.deepEqual(posts[0].request, { intent: 'login_session', protocol_version: 1 });
assert.doesNotMatch(
  [...storage.values()].join(''),
  /qr_payload|fixture-private|verify_code|token|https:/
);
const id = h.value.flow.id;
h.unmount();
h = mount();
h.render();
await settle(h);
assert.equal(h.value.flow.id, id);
assert.equal(posts.length, 1, 'restore must use GET');
server = {
  ...server,
  step: 'awaiting_code',
  challenge_revision: 2,
  revision: 8,
  allowed_actions: ['cancel', 'submit_code'],
};
await h.value.retryStatus();
await settle(h);
await h.value.submitCode('12345');
await settle(h);
assert.equal(actions[0].flow.revision, 8);
assert.equal(actions[0].flow.challenge_revision, 2);
assert.doesNotMatch([...storage.values()].join(''), /12345|challenge_revision/);
server = {
  ...server,
  state: 'recovery_required',
  step: 'poll',
  revision: 10,
  qr_payload: undefined,
};
await h.value.retryStatus();
await settle(h);
h.value.clearTerminal();
await settle(h);
assert.equal(h.value.flow, undefined);
assert.equal(posts.length, 1, 'clearing a terminal receipt must not begin a new registration');
h.unmount();

// Reauthorization freezes the existing connection; old connection readiness is
// independent from a new registration's waiting state.
server = {
  ...server,
  state: 'awaiting_authorization',
  step: 'awaiting_code',
  challenge_revision: 4,
  revision: 12,
  allowed_actions: ['submit_code', 'cancel'],
};
const existing = {
  id: globalThis.crypto.randomUUID(),
  connector_id: 'weixin',
  state: 'ready',
  revision: 5,
};
h = mount(existing);
h.render();
await settle(h);
await h.value.start();
await settle(h);
assert.deepEqual(posts.at(-1).request, {
  intent: 'reauthorize',
  protocol_version: 1,
  expected_revision: 5,
});
assert.equal(posts.at(-1).connection, existing.id);
assert.equal(h.value.connection, undefined);
let resolve;
hold = new Promise(r => {
  resolve = r;
});
const inFlight = h.value.submitCode('67890');
h.unmount();
actor = 'owner-2';
resolve({
  data: { ...server, state: 'ready', result_connection_id: globalThis.crypto.randomUUID() },
});
await inFlight;
hold = undefined;
const fresh = mount(existing);
fresh.render();
await settle(fresh);
assert.equal(fresh.value.flow, undefined);
assert.equal(fresh.value.connection, undefined);
fresh.unmount();

// Inspect the real form with the existing fixture renderer: secrets start
// empty, QR is rendered locally, and no iframe or provider URL is fetched.
const metadata = load('src/components/connectors/connector-metadata.ts');
const formHarness = harness(react => {
  const panel = load(
    'src/components/connectors/setup/connector-messaging-setup-panel.tsx',
    {
      react,
      'react/jsx-runtime': jsx,
      'next-intl': { useLocale: () => 'en-US' },
      'next/link': { default: 'Link' },
      'react-qr-code': { default: 'QRCode' },
      '@tanstack/react-query': { useQueryClient: () => queryClient },
      '@/components/ui/button': ui,
      './connector-key-form': { ConnectorFieldHelp: 'ConnectorFieldHelp' },
      '@/components/ui/input': ui,
      '@/components/ui/label': ui,
      '@/components/ui/alert': ui,
      '@/i18n': { useT: () => key => key },
      '@/store/auth-store': { useAuthStore: auth },
      '@/store/organization-store': { useOrganizationStore: organizations },
      '@/services/connector.service': { connectorService: service },
      '@/hooks/connectors/use-connector-onboarding': { useConnectorOnboarding: () => ({}) },
      '@/hooks/connectors/connection-detail-query': { invalidatePersonalConnectionQueries() {} },
      '@/hooks/connectors/connector-request-error': errors,
      '../connector-metadata': metadata,
      './use-creation-default': { useCreationDefault: () => ({ option: () => null, begin() {} }) },
      './app-setup-state': setup,
      './messaging-setup-state': messaging,
    },
    '\nexports.fixturePanel = MessagingSetup;'
  );
  return () =>
    panel.fixturePanel({
      connector: {
        ...connector,
        id: 'dingtalk',
        setup: { handler: 'dingtalk-registration-v1', protocol_version: 1, methods: ['manual'] },
        setup_availability: { enabled: true, methods: ['manual'] },
        credential_fields: [
          { name: 'client_id', label: 'Client ID', max_length: 128, required: true },
          {
            name: 'client_secret',
            label: 'Secret',
            max_length: 16384,
            required: true,
            secret: true,
          },
        ],
      },
      initialMode: 'manual',
    });
});
const tree = formHarness.render();
const inputs = nodes(tree).filter(n => n.type === 'Input');
assert.equal(inputs.length, 2);
assert.ok(inputs.every(n => n.props.value === ''));
assert.equal(inputs.find(n => n.props.name === 'client_secret').props.type, 'password');
assert.equal(
  nodes(tree).some(n => n.type === 'iframe' || n.type === 'img'),
  false
);
let manualSubmissions = 0;
service.startCredentialSetup = async (_provider, request) => {
  manualSubmissions++;
  assert.deepEqual(request.values, {
    client_id: 'fixture-client',
    client_secret: 'PRIVATE_FORM_SECRET',
  });
  throw new Error('response lost after submission');
};
inputs[0].props.onChange({ target: { value: 'fixture-client' } });
inputs[1].props.onChange({ target: { value: 'PRIVATE_FORM_SECRET' } });
formHarness.render();
nodes(formHarness.value)
  .find(n => n.type === 'form')
  .props.onSubmit({ preventDefault() {} });
await settle(formHarness);
assert.equal(manualSubmissions, 1);
assert.ok(
  nodes(formHarness.value)
    .filter(n => n.type === 'Input')
    .every(n => n.props.value === ''),
  'submission clears the private form'
);
assert.ok(
  nodes(formHarness.value).some(n => n.props?.href === '/console/connectors/connections'),
  'unknown outcomes offer inspection of saved connections'
);
assert.ok(
  nodes(formHarness.value).find(n => n.props?.type === 'submit').props.disabled,
  'unknown results do not permit resubmission'
);
formHarness.unmount();

// A received credential outlives its QR. Restore/retry the same flow without registering again.
actor = 'verification-owner';
server = {
  ...server,
  state: 'verification_waiting',
  step: 'verify',
  revision: 20,
  qr_expires_at: '2000-01-01T00:00:00Z',
  expires_at: '2000-01-01T00:00:00Z',
  candidate_expires_at: '2099-01-01T00:00:00Z',
  verification_deadline: '2099-01-01T00:00:00Z',
  verification_phase: 'identity',
  allowed_actions: ['cancel', 'retry_verification'],
};
assert.equal(setup.activeOnboarding(server), true);
assert.equal(messaging.messagingQR(connector, server), undefined);
h = mount();
h.render();
await settle(h);
await h.value.start();
await settle(h);
const beforeRetryStarts = posts.length;
h.unmount();
h = mount();
h.render();
await settle(h);
assert.equal(h.value.flow.state, 'verification_waiting');
await h.value.start();
await settle(h);
assert.equal(posts.length, beforeRetryStarts, 'waiting must not create another QR flow');
const retries = [];
let failRetry = true;
service.retryOnboardingVerification = async (flow, key) => {
  retries.push({ flow, key });
  if (failRetry) throw new Error('lost retry response');
  server = {
    ...server,
    revision: flow.revision + 1,
    state: 'verifying',
    allowed_actions: ['cancel'],
  };
  return { data: server };
};
server = { ...server, revision: 23 };
await h.value.retryVerification();
await settle(h);
assert.equal(retries[0].flow.revision, 23, 'retry uses the latest server revision');
failRetry = false;
await h.value.retryVerification();
await settle(h);
assert.equal(
  retries[0].key,
  retries[1].key,
  'uncertain retry preserves idempotency for the same revision'
);
assert.equal(h.value.flow.state, 'verifying');
assert.equal(posts.length, beforeRetryStarts);
assert.doesNotMatch(
  [...storage.values()].join(''),
  /token|qr_payload|candidate_expires_at|verification_phase/
);
h.unmount();

const requests = [];
const realService = load('src/services/connector.service.ts', {
  '@/lib/http/services': {
    BaseService: class {
      request(...args) {
        requests.push(args);
        return Promise.resolve({});
      }
    },
  },
}).connectorService;
await realService.retryOnboardingVerification(
  { id: 'original-flow', revision: 23 },
  'original-command'
);
assert.deepEqual(requests[0].slice(0, 3), [
  'post',
  '/connectors/onboarding-flows/original-flow/actions',
  { action: 'retry_verification', expected_revision: 23 },
]);
assert.equal(requests[0][3].headers['Idempotency-Key'], 'original-command');
assert.equal(requests[0][3].retryAttemptsOverride, 0);
console.log(
  'Messaging default setup, QR expiry, restore, challenge CAS, account isolation, reauthorization and private form checks passed.'
);
