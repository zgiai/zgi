import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
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
assert.equal(
  state.supportsAppSetup({ ...connector, extension_schema_version: '1.3.0' }),
  true,
  'host-surface upgrades retain the Feishu app setup panel'
);
for (const descriptor of [
  { ...connector, extension_schema_version: '1.4.0' },
  { ...connector, extension_schema_version: '1.3.0', setup: { ...connector.setup, handler: 'unknown' } },
  { ...connector, extension_schema_version: '1.3.0', credential_fields: [] },
]) {
  assert.equal(state.supportsAppSetup(descriptor), false, 'unsupported setup contracts remain rejected');
}
for (const schema of ['1.2.0', '1.3.0']) {
  const messaging = {
    id: 'wecom', extension_schema_version: schema,
    setup: { handler: 'wecom-registration-v1', protocol_version: 1, methods: ['manual'] },
  };
  assert.equal(state.supportsMessagingContract(messaging), true);
  assert.equal(state.supportsMessagingContract({
    ...messaging, setup: { ...messaging.setup, protocol_version: 2 },
  }), false);
}
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
  id: randomUUID(),
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
  id: randomUUID(),
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
const queryClient = {invalidateQueries: async () => {}};
const mountHook = (update, pendingID) =>
  hooksHarness(react => {
    const { useConnectorOnboarding } = load('src/hooks/connectors/use-connector-onboarding.ts', {
      ...imports,
      react,
    });
    return () => useConnectorOnboarding(connector, update, pendingID);
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
serverFlow = { ...serverFlow, id: randomUUID(), state: 'starting' };
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
  '@/hooks/connectors/use-pending-setups': {pendingSetupKeys: ['connectors','pending-setups'], usePendingSetup: () => ({})},
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
const retiredKey = randomUUID();
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

// Pending is a separate receipt, rendered inside the existing setup surface.
let pendingRecord = {kind:'pending_setup',id:randomUUID(),connector_id:'feishu',app_id:'cli_original',region:'feishu',state:'awaiting_application',revision:3,resume_expires_at:new Date(Date.now()+86400000).toISOString(),allowed_actions:['resume_manual','cancel'],resume_qr_unavailable_reason:'same_app_resume_not_verified'};
let manualResumes=0, pendingReads=0;
let pendingOnboarding={};
attached=[];
service.resumePendingManual = async (id,revision,values) => {
  manualResumes++;
  assert.equal(id,pendingRecord.id);assert.equal(revision,3);
  assert.deepEqual(values,{app_id:'cli_original',region:'feishu',app_secret:'PENDING-SECRET'});
  return {data:{id:randomUUID(),state:'pending_completion',pending_setup_id:id,error_code:'connector_application_not_ready'}};
};
h=hooksHarness(react=>{
  const module=load('src/components/connectors/setup/connector-app-setup-panel.tsx', {...panelImports,react,
    '@/hooks/connectors/use-connector-onboarding':{useConnectorOnboarding:()=>pendingOnboarding},
    '@/hooks/connectors/use-pending-setups': {pendingSetupKeys:['connectors','pending-setups'],usePendingSetup:()=>({data:{data:pendingRecord},refetch:async()=>{pendingReads++;return {data:{data:pendingRecord}};}})},
  },'\nexports.fixturePanel = ScopedConnectorAppSetupPanel;');
  return ()=>module.fixturePanel({connector,pendingSetupID:pendingRecord.id,embedded:true,onReady:c=>attached.push(c)});
});
h.render();await h.settle();
assert.equal(input(false).props.value,'cli_original');assert.equal(input(false).props.disabled,true);
assert.equal(nodes(h.value).find(n=>n.type==='select').props.disabled,true);
assert.equal(nodes(h.value).filter(n=>n.type==='QRCode'||n.type==='TemplateDetails').length,0);
assert.ok(nodes(h.value).find(n=>n.type==='Button'&&n.props.children==='pending.resumeQR').props.disabled);
const previousPosts = posts.length;
await nodes(h.value).find(n=>n.type==='Button'&&n.props.children==='pending.refresh').props.onClick();
assert.equal(pendingReads,1);assert.equal(manualResumes,0);assert.equal(posts.length,previousPosts,'refresh cannot start registration or verify credentials');
input(true).props.onChange({target:{value:'PENDING-SECRET'}});h.render();
nodes(h.value).find(n=>n.type==='form').props.onSubmit({preventDefault(){}});await h.settle();
assert.equal(input(true).props.value,'');assert.equal(manualResumes,1);assert.equal(attached.length,0);
assert.equal(nodes(h.value).filter(n=>n.type==='Alert'&&n.props.variant==='destructive').length,0,'approval wait is not a fatal error');
assert.doesNotMatch([...saved.values()].join(''),/PENDING-SECRET/);
pendingOnboarding={flow:{id:randomUUID(),state:'verifying'}};
h.render();await h.settle();
pendingOnboarding={flow:{...pendingOnboarding.flow,state:'pending_completion'}};
h.render();await h.settle();
assert.ok(nodes(h.value).some(n=>n.type==='form'),'a restored unfinished verification returns to the same manual recovery form');
assert.doesNotMatch(JSON.stringify(h.value),/"app.failed"/,'pending is never presented as terminal setup failure');
pendingRecord={...pendingRecord,allowed_actions:['resume_qr','resume_manual','cancel'],resume_qr_unavailable_reason:undefined};
let qrStarts=[];
pendingOnboarding={start:(...args)=>qrStarts.push(args)};
h.render();await h.settle();
const qrTab=nodes(h.value).find(n=>n.type==='Button'&&n.props.children==='pending.resumeQR');
assert.equal(Boolean(qrTab.props.disabled),false,'server-advertised same-app scan is enabled');
qrTab.props.onClick();h.render();await h.settle();
assert.equal(qrStarts.length,0,'changing tabs does not create a QR session');
assert.ok(nodes(h.value).some(n=>n.props.children==='pending.resumeQRHint'));
assert.equal(nodes(h.value).filter(n=>n.type==='TemplateDetails').length,0,'resume does not offer the new-app permission template');
const startQR=nodes(h.value).filter(n=>n.type==='Button'&&n.props.children==='pending.resumeQR').at(-1);
startQR.props.onClick();assert.deepEqual(qrStarts, [['feishu',false,3]],'only explicit resume uses the original record revision');
assert.equal(posts.length,previousPosts,'same-app scan never calls create-app');
h.unmount();
pendingRecord={...pendingRecord,state:'expired',allowed_actions:[]};
h=hooksHarness(react=>{
  const module=load('src/components/connectors/setup/connector-app-setup-panel.tsx',{...panelImports,react,'@/hooks/connectors/use-pending-setups':{pendingSetupKeys:[],usePendingSetup:()=>({data:{data:pendingRecord}})}},'\nexports.fixturePanel = ScopedConnectorAppSetupPanel;');
  return ()=>module.fixturePanel({connector,pendingSetupID:pendingRecord.id,embedded:true});
});
h.render();await h.settle();assert.equal(nodes(h.value).filter(n=>n.type==='form').length,0);
assert.ok(nodes(h.value).some(n=>n.type==='Button'&&n.props.children==='pending.newManual'));
h.unmount();
assert.equal(state.activeOnboarding({state:'pending_completion'}),false,'pending never polls registration');
console.log('Pending app identity, manual recovery, gate, expiry, no-binding and write-only checks passed.');

saved.clear();
const resumeID=randomUUID(), qrRequests=[];
service.getPendingSetup=async()=>({data:{id:resumeID,allowed_actions:['resume_qr','resume_manual','cancel']}});
serverFlow={...serverFlow,id:randomUUID(),state:'awaiting_authorization',pending_setup_id:resumeID};
service.resumePendingSetup=async(id,revision,key)=>{qrRequests.push({id,revision,key});return {data:{...serverFlow}};};
const createCount=posts.length;
h=mountHook(undefined,resumeID);h.render();await h.settle();
assert.equal(qrRequests.length,0,'opening pending record is GET-only');
await h.value.start('lark',false,7);await h.settle();
assert.equal(qrRequests.length,1);assert.equal(qrRequests[0].id,resumeID);assert.equal(qrRequests[0].revision,7);
assert.equal(posts.length,createCount,'actual hook must use pending resume endpoint, not create-app');
assert.doesNotMatch([...saved.values()].join(''),/verification_url|device_code|app_secret|clientID|addons/);
h.unmount();h=mountHook(undefined,resumeID);h.render();await h.settle();
assert.equal(qrRequests.length,1,'reopening recovers existing QR flow without starting another');
assert.equal(h.value.flow.id,serverFlow.id);h.unmount();
console.log('Enabled same-app QR hook, explicit admission and GET-only reopening passed.');
