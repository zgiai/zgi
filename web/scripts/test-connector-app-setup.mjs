import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import ts from 'typescript';

const read = file => readFileSync(file, 'utf8');
function load(file, imports = {}, suffix = '') {
  const code = ts.transpileModule(read(file), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText;
  const module = { exports: {} };
  new Function('require', 'module', 'exports', code + suffix)(
    name => {
      assert.ok(name in imports, `${file}: unexpected ${name}`);
      return imports[name];
    },
    module,
    module.exports
  );
  return module.exports;
}
const state = load('src/components/connectors/setup/app-setup-state.ts');
const errors = load('src/hooks/connectors/connector-request-error.ts');
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((a, b) => {
    resolve = a;
    reject = b;
  });
  return { promise, resolve, reject };
};
const flush = async () => {
  for (let i = 0; i < 12; i++) await Promise.resolve();
};
function hooksHarness(factory) {
  const slots = [];
  let cursor = 0,
    effects = [],
    value;
  const same = (a, b) => a && a.length === b.length && a.every((v, i) => Object.is(v, b[i]));
  const react = {
    useRef(initial) {
      const i = cursor++;
      return (slots[i] ??= { current: initial });
    },
    useState(initial) {
      const i = cursor++;
      if (!(i in slots)) slots[i] = typeof initial === 'function' ? initial() : initial;
      return [
        slots[i],
        v => {
          slots[i] = typeof v === 'function' ? v(slots[i]) : v;
        },
      ];
    },
    useMemo(fn, deps) {
      const i = cursor++;
      if (!slots[i] || !same(slots[i].deps, deps)) slots[i] = { deps, value: fn() };
      return slots[i].value;
    },
    useCallback(fn, deps) {
      return react.useMemo(() => fn, deps);
    },
    useEffect(fn, deps) {
      const i = cursor++;
      if (!slots[i] || !same(slots[i].deps, deps)) {
        const old = slots[i];
        slots[i] = { deps, cleanup: old?.cleanup };
        effects.push(() => {
          slots[i].cleanup?.();
          slots[i].cleanup = fn();
        });
      }
    },
  };
  const component = factory(react);
  const render = () => {
    cursor = 0;
    value = component();
    const pending = effects;
    effects = [];
    pending.forEach(fn => fn());
    return value;
  };
  return {
    render,
    get value() {
      return value;
    },
    async settle() {
      for (let i = 0; i < 4; i++) {
        await flush();
        render();
      }
    },
    unmount() {
      slots.forEach(slot => slot?.cleanup?.());
    },
  };
}
let actor = 'owner',
  org = 'organization';
const auth = { use: { user: () => ({ id: actor }) }, getState: () => ({ user: { id: actor } }) };
const organizations = {
  use: { currentOrganization: () => ({ id: org }) },
  getState: () => ({ currentOrganization: { id: org } }),
};
const saved = new Map();
globalThis.sessionStorage = {
  getItem: k => saved.get(k) ?? null,
  setItem: (k, v) => saved.set(k, v),
  removeItem: k => saved.delete(k),
};
const connector = {
  id: 'feishu',
  extension_schema_version: '1.1.0',
  setup_method: 'app_credentials',
  setup: {
    handler: 'feishu-app-registration-v1',
    methods: ['manual', 'qr_create', 'qr_update'],
    template_id: 'feishu-app-template-v3',
  },
  credential_fields: [
    { name: 'region' },
    { name: 'app_id', label: 'App ID', required: true },
    { name: 'app_secret', label: 'App Secret', required: true, secret: true },
  ],
};
assert.equal(state.supportsAppSetup(connector), true);
assert.equal(state.supportsAppSetup({ id: 'github', key_fields: [] }), false);
for (const url of [
  'https://evil.invalid',
  'https://accounts.feishu.cn.evil.invalid/a',
  'http://accounts.feishu.cn',
  'https://name@accounts.feishu.cn',
  'https://accounts.feishu.cn:8443',
  'https://accounts.feishu.cn/#secret',
])
  assert.equal(state.safeVerificationURL(url), undefined);
assert.equal(
  state.safeVerificationURL('https://accounts.larksuite.com/authorize?user_code=fixture'),
  'https://accounts.larksuite.com/authorize?user_code=fixture'
);
let posts = [],
  gets = [],
  cancels = [],
  pendingStart;
let serverFlow = {
  id: crypto.randomUUID(),
  connector_id: 'feishu',
  state: 'awaiting_authorization',
  revision: 2,
  region: 'feishu',
  retry_after: 5,
  expires_at: new Date(Date.now() + 600000).toISOString(),
  verification_url: 'https://accounts.feishu.cn/authorize?user_code=fixture',
  user_code: 'fixture',
};
const readyConnection = {
  id: crypto.randomUUID(),
  connector_id: 'feishu',
  state: 'ready',
  revision: 1,
};
const service = {
  async startOnboarding(id, request, key, updateId) {
    posts.push({ id, request, key, updateId });
    if (pendingStart) return pendingStart.promise;
    return { data: { ...serverFlow } };
  },
  async getOnboarding(id) {
    gets.push(id);
    return { data: { ...serverFlow } };
  },
  async cancelOnboarding(id, revision) {
    cancels.push({ id, revision });
    serverFlow = { ...serverFlow, state: 'cancelled', revision: revision + 1 };
    return { data: { ...serverFlow } };
  },
  async getConnection() {
    return { data: readyConnection };
  },
};
const imports = {
  '@tanstack/react-query': { useQueryClient: () => queryClient },
  '@/store/auth-store': { useAuthStore: auth },
  '@/store/organization-store': { useOrganizationStore: organizations },
  '@/services/connector.service': { connectorService: service },
  '@/components/connectors/setup/app-setup-state': state,
  './connector-request-error': errors,
  './connection-detail-query': { invalidatePersonalConnectionQueries() {} },
};
const queryClient = {};
const mountHook = update =>
  hooksHarness(react => {
    const { useConnectorOnboarding } = load('src/hooks/connectors/use-connector-onboarding.ts', {
      ...imports,
      react,
    });
    return () => useConnectorOnboarding(connector, update);
  });

let h = mountHook();
h.render();
await h.settle();
assert.equal(posts.length, 0, 'opening a connector never creates an application');
await h.value.start('feishu');
await h.settle();
assert.equal(posts.length, 1);
const stored = [...saved.values()].join('');
assert.doesNotMatch(
  stored,
  /verification_url|user_code|https:|app_secret|device_code/,
  'recovery storage contains references only'
);
assert.equal(h.value.flow.id, serverFlow.id);
h.unmount();
h = mountHook();
h.render();
await h.settle();
assert.equal(posts.length, 1, 'reopen uses GET only');
assert.equal(h.value.restored, true);
serverFlow.revision = 9;
await h.value.cancel();
await h.settle();
assert.equal(cancels.at(-1).revision, 9, 'cancel uses refreshed revision');
assert.equal(h.value.flow.state, 'cancelled');
h.unmount();

saved.clear();
pendingStart = deferred();
h = mountHook();
h.render();
await h.settle();
const first = h.value.start('lark');
pendingStart.reject({});
await first;
await h.settle();
assert.equal(h.value.errorCode, 'transport_unconfirmed');
const original = posts.at(-1);
await h.value.start('feishu');
await h.settle();
assert.equal(posts.at(-1), original, 'unknown begin cannot be replaced with a new request');
pendingStart = undefined;
serverFlow = { ...serverFlow, id: crypto.randomUUID(), state: 'starting' };
await h.value.retryStatus();
await h.settle();
assert.equal(posts.at(-1).key, original.key, 'explicit recovery reuses idempotency key');
assert.equal(posts.at(-1).request.region, 'lark');
h.unmount();

serverFlow = { ...serverFlow, state: 'ready', result_connection_id: readyConnection.id };
h = mountHook();
h.render();
await h.settle();
assert.equal(h.value.connection.id, readyConnection.id);
assert.equal(h.value.restored, true, 'recovered ready flow requires explicit host use');
h.unmount();

saved.clear();
pendingStart = deferred();
h = mountHook();
h.render();
await h.settle();
const late = h.value.start('feishu');
actor = 'different-owner';
pendingStart.resolve({ data: serverFlow });
await late;
await h.settle();
assert.equal(h.value.flow, undefined, 'late result cannot cross account scope');
h.unmount();
actor = 'owner';
pendingStart = undefined;
saved.clear();

// Exercise manual form callbacks and clearing, inside the original Card/Dialog.
const el = (type, props) => ({ type, props: props ?? {} });
const ui = new Proxy({}, { get: (_, key) => key });
const nodes = tree =>
  Array.isArray(tree)
    ? tree.flatMap(nodes)
    : tree && typeof tree === 'object'
      ? [tree, ...nodes(tree.props?.children)]
      : [];
let submitted,
  manualPending = deferred(),
  attached = [];
service.startAppCredentialSetup = async (id, body) => {
  submitted = { id, values: { ...body.values } };
  return manualPending.promise;
};
const panelImports = {
  './use-creation-default': { useCreationDefault: () => ({ option: () => null, begin() {}, complete: async () => true }) },
  ...imports,
  'react/jsx-runtime': { jsx: el, jsxs: el, Fragment: 'Fragment' },
  'next/link': { default: 'Link' },
  'react-qr-code': { default: 'QRCode' },
  'lucide-react': ui,
  '@/components/ui/alert': ui,
  '@/components/ui/button': ui,
  '@/components/ui/card': ui,
  '@/components/ui/input': ui,
  '@/components/ui/label': ui,
  '@/components/connectors/detail/connector-app-permissions': {
    ConnectorPermissionTemplateDetails: 'TemplateDetails',
  },
  '@/i18n': { useT: () => key => key },
  '@/hooks/connectors/use-connector-onboarding': { useConnectorOnboarding: () => ({}) },
  '@/hooks/connectors/connector-request-error': errors,
  '@/hooks/connectors/connection-detail-query': { invalidatePersonalConnectionQueries() {} },
  './app-setup-state': state,
};
h = hooksHarness(react => {
  const module = load(
    'src/components/connectors/setup/connector-app-setup-panel.tsx',
    { ...panelImports, react },
    '\nexports.fixturePanel = ScopedConnectorAppSetupPanel;'
  );
  return () =>
    module.fixturePanel({
      connector,
      initialMode: 'manual',
      onReady: c => attached.push(c),
      embedded: true,
    });
});
h.render();
await h.settle();
const input = secret =>
  nodes(h.value).find(n => n.type === 'Input' && n.props.type === (secret ? 'password' : 'text'));
input(false).props.onChange({ target: { value: 'cli_fixture' } });
input(true).props.onChange({ target: { value: 'fixture-secret' } });
h.render();
nodes(h.value)
  .find(n => n.type === 'form')
  .props.onSubmit({ preventDefault() {} });
h.render();
assert.deepEqual(submitted.values, {
  region: 'feishu',
  app_id: 'cli_fixture',
  app_secret: 'fixture-secret',
});
assert.equal(input(true).props.value, '', 'secret clears before awaiting response');
manualPending.resolve({ data: { state: 'ready', connection: readyConnection } });
await h.settle();
assert.equal(attached.length, 1, 'new connection goes through the existing host onReady contract');
assert.doesNotMatch([...saved.values()].join(''), /fixture-secret/);
h.unmount();
console.log('App setup hook and manual form checks passed.');

// An older/incomplete evidence projection must not recreate the historical
// "reading length of undefined" failure or claim zero confirmed permissions.
const { ConnectorAppPermissions, ConnectorPermissionTemplateDetails } = load(
  'src/components/connectors/detail/connector-app-permissions.tsx',
  {
    './permission-evidence-model': load('src/components/connectors/detail/permission-evidence-model.ts'),
    'react/jsx-runtime': { jsx: el, jsxs: el, Fragment: 'Fragment' },
    'next-intl': { useLocale: () => 'zh-Hans' },
    '@/components/ui/card': ui,
    '@/components/ui/badge': ui,
    '@/i18n': { useT: () => key => key, useLocale: () => 'zh-Hans' },
    '@/components/connectors/connector-metadata': {
      resolveConnectorLocalizedText: (_localized, _locale, fallback) => fallback,
    },
  }
);
const partialPermissions = ConnectorAppPermissions({
  connector: { ...connector, permission_template: { addons: {} } },
  evidence: { app_id: 'cli_fixture', app_name: 'Fixture', scope_status: 'verified' },
});
assert.match(JSON.stringify(partialPermissions), /app.unknown/);
assert.doesNotThrow(() => ConnectorAppPermissions({ connector }));
const permissionTemplate = JSON.parse(
  read('../api/internal/modules/connectors/catalog/definitions/feishu-permissions.json')
);
const templateTree = ConnectorPermissionTemplateDetails({ template: permissionTemplate });
assert.equal(templateTree.type, 'details', 'template stays collapsed inside the existing UI');
assert.equal(Boolean(templateTree.props.open), false);
const templateText = JSON.stringify(templateTree);
assert.doesNotMatch(templateText, /cardkit:card:read/);
assert.match(templateText, /cardkit:card:write/);
assert.match(templateText, /3\.0\.0/);
for (const scope of ['tenant:tenant:readonly', 'im:message.group_msg', 'im:chat.members:read', 'directory:employee:search', 'directory:employee.base.name.name:read']) {
  assert.ok(templateText.includes(scope), `template preview omits ${scope}`);
}
assert.match(templateText, /app.templateIncrementalHint/);
assert.match(templateText, /app.userEventDependencyHint/);
for (const dependency of permissionTemplate.user_event_dependencies) {
  assert.ok(templateText.includes(dependency.event_id));
  assert.ok(templateText.includes(dependency.user_scope));
  assert.ok(templateText.includes(dependency.subscription_api));
  assert.equal(permissionTemplate.addons.scopes.user.includes(dependency.user_scope), false);
}
assert.equal(ConnectorPermissionTemplateDetails({}), null);
assert.doesNotThrow(() => ConnectorPermissionTemplateDetails({ template: { addons: {} } }));
saved.clear();
const retiredKey = crypto.randomUUID();
serverFlow = { ...serverFlow, state: 'failed', result_connection_id: undefined };
saved.set(
  `connector-onboarding:${org}:${actor}:feishu:create`,
  JSON.stringify({
    key: retiredKey,
    flowId: serverFlow.id,
    request: { intent: 'create_app', region: 'feishu', template_id: 'feishu-app-template-v2' },
  })
);
const beforeTemplateRecovery = posts.length;
h = mountHook();
h.render();
await h.settle();
assert.equal(posts.length, beforeTemplateRecovery, 'recovering a retired flow is GET-only');
assert.equal(h.value.flow.state, 'failed');
await h.value.start('feishu');
await h.settle();
assert.equal(posts.at(-1).request.template_id, 'feishu-app-template-v3');
assert.notEqual(posts.at(-1).key, retiredKey, 'explicit new flow must not rewrite an old request');
h.unmount();
saved.clear();
const existingGrant = ConnectorAppPermissions({
  connector: { ...connector, permission_template: permissionTemplate },
  evidence: { scope_status: 'verified', granted_scopes: ['cardkit:card:read'] },
});
assert.match(
  JSON.stringify(existingGrant),
  /cardkit:card:read/,
  'do not pretend an existing grant was revoked'
);
h = hooksHarness(react => {
  const module = load(
    'src/components/connectors/setup/connector-app-setup-panel.tsx',
    { ...panelImports, react },
    '\nexports.fixturePanel = ScopedConnectorAppSetupPanel;'
  );
  return () =>
    module.fixturePanel({ connector: { ...connector, permission_template: permissionTemplate } });
});
const beforePreview = posts.length;
h.render();
await h.settle();
assert.equal(
  nodes(h.value).find(node => node.type === 'TemplateDetails').props.template,
  permissionTemplate
);
assert.equal(posts.length, beforePreview, 'preview must not start registration');
h.unmount();
console.log(
  'App credentials, QR recovery/cancel, scope isolation, safe URLs, write-only form and host handoff checks passed.'
);
