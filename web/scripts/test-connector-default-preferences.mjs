import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

function load(file, imports = {}) {
  const code = ts.transpileModule(readFileSync(path.join(process.cwd(), file), 'utf8'), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText;
  const module = { exports: {} };
  new Function('require', 'module', 'exports', code)(
    name => {
      assert.ok(name in imports, `${file}: unexpected import ${name}`);
      return imports[name];
    },
    module,
    module.exports
  );
  return module.exports;
}

const requests = [];
const service = load('src/services/connector.service.ts', {
  '@/lib/http/services': {
    BaseService: class {
      request(...args) {
        requests.push(args);
        return Promise.resolve({ data: { revision: 0, defaults: {} } });
      }
    },
  },
}).connectorService;
await service.getChatPreferences();
assert.deepEqual(requests.pop(), ['get', '/connector-preferences/chat/me']);
await service.setChatPreference(
  'web-search',
  { mode: 'selected', connection_id: 'a', expected_revision: 0 },
  'once'
);
assert.deepEqual(requests.pop(), [
  'put',
  '/connector-preferences/chat/me/web-search',
  { mode: 'selected', connection_id: 'a', expected_revision: 0 },
  { headers: { 'Idempotency-Key': 'once' }, retryAttemptsOverride: 0 },
]);
await service.setChatPreference('provider/name', { mode: 'none', expected_revision: 3 }, 'clear');
assert.equal(requests.pop()[1], '/connector-preferences/chat/me/provider%2Fname');

let accountID = 'owner-a',
  organizationID = 'org-a';
let queryOptions, mutationOptions;
const cache = new Map(),
  invalidations = [],
  writes = [];
const client = {
  getQueryData(key) {
    return cache.get(JSON.stringify(key));
  },
  setQueryData(key, updater) {
    const id = JSON.stringify(key);
    cache.set(id, updater(cache.get(id)));
  },
  invalidateQueries(input) {
    invalidations.push(input);
    return Promise.resolve();
  },
};
let result = { data: { revision: 0, defaults: {} } };
let fetcher = async () => result;
let writer = async () => ({ data: { revision: 1, defaults: { 'web-search': { mode: 'none' } } } });
const hooks = load('src/hooks/connectors/use-chat-connector-preferences.ts', {
  '@tanstack/react-query': {
    useQuery: options => {
      queryOptions = options;
      return { data: undefined };
    },
    useMutation: options => {
      mutationOptions = options;
      return { mutateAsync: options.mutationFn };
    },
    useQueryClient: () => client,
  },
  '@/store/auth-store': {
    useAuthStore: {
      use: { user: () => (accountID ? { id: accountID } : null) },
      getState: () => ({ user: accountID ? { id: accountID } : null }),
    },
  },
  '@/store/organization-store': {
    useOrganizationStore: {
      use: { currentOrganization: () => (organizationID ? { id: organizationID } : null) },
      getState: () => ({ currentOrganization: organizationID ? { id: organizationID } : null }),
    },
  },
  '@/services/connector.service': {
    connectorService: {
      getChatPreferences: () => fetcher(),
      setChatPreference: (...args) => {
        writes.push(args);
        return writer();
      },
    },
  },
});
hooks.useChatConnectorPreferences(true);
assert.equal(queryOptions.enabled, true);
assert.equal(queryOptions.retry, false);
assert.deepEqual(queryOptions.queryKey, ['connectors', 'chat-preferences', 'org-a', 'owner-a']);
assert.equal(
  (await queryOptions.queryFn()).data.revision,
  0,
  'no persisted defaults is valid revision zero'
);
hooks.useChatConnectorPreferences(false);
assert.equal(queryOptions.enabled, false);
accountID = undefined;
hooks.useChatConnectorPreferences(true);
assert.equal(queryOptions.enabled, false, 'cannot request another account while auth loads');
accountID = 'owner-a';

for (const data of [
  null,
  {},
  { revision: -1, defaults: {} },
  { revision: 0, defaults: [] },
  { revision: 0, defaults: { github: { mode: 'selected', connection_id: 'x' } } },
  { revision: 1, defaults: { github: { mode: 'invalid' } } },
]) {
  assert.throws(() => hooks.requireChatConnectorPreferences({ data }), /incomplete/);
}
assert.equal(
  hooks.requireChatConnectorPreferences({
    data: {
      revision: 1,
      defaults: { github: { mode: 'selected', connection_id: 'hidden', available: false } },
    },
  }).data.defaults.github.available,
  false
);

hooks.useChatConnectorPreferences(true);
const oldRead = queryOptions.queryFn;
organizationID = 'org-b';
await assert.rejects(
  oldRead(),
  /scope changed/,
  'late read cannot cache a new org under an old query key'
);
organizationID = 'org-a';
hooks.useChatConnectorPreferences(true);
fetcher = async () => {
  accountID = 'owner-b';
  return result;
};
await assert.rejects(queryOptions.queryFn(), /scope changed/);
accountID = 'owner-a';
fetcher = async () => result;

const mutation = hooks.useSetChatConnectorPreference();
await mutation.mutateAsync({
  connectorID: 'web-search',
  input: { mode: 'none', expected_revision: 0 },
});
assert.equal(writes.length, 1);
assert.match(writes[0][2], /^[\da-f-]{36}$/i, 'each explicit attempt has an idempotency key');
assert.deepEqual(
  cache.get(JSON.stringify(hooks.CHAT_CONNECTOR_PREFERENCE_KEYS.scope('org-a', 'owner-a'))).data,
  { revision: 1, defaults: { 'web-search': { mode: 'none' } } }
);
organizationID = 'org-b';
await assert.rejects(
  mutation.mutateAsync({
    connectorID: 'web-search',
    input: { mode: 'none', expected_revision: 0 },
  }),
  /scope changed/
);
assert.equal(writes.length, 1, 'even none commands cannot be sent from stale org callbacks');
organizationID = 'org-a';

let complete;
writer = () =>
  new Promise(resolve => {
    complete = resolve;
  });
const pending = mutation.mutateAsync({
  connectorID: 'web-search',
  input: { mode: 'none', expected_revision: 1 },
});
organizationID = 'org-b';
complete({ data: { revision: 2, defaults: { 'web-search': { mode: 'none' } } } });
await pending;
assert.equal(
  cache.has(JSON.stringify(hooks.CHAT_CONNECTOR_PREFERENCE_KEYS.scope('org-b', 'owner-a'))),
  false,
  'in-flight response stays in originating cache'
);
organizationID = 'org-a';
cache.set(JSON.stringify(hooks.CHAT_CONNECTOR_PREFERENCE_KEYS.scope('org-a', 'owner-a')), {
  data: { revision: 9, defaults: {} },
});
writer = async () => ({ data: { revision: 3, defaults: {} } });
await mutation.mutateAsync({
  connectorID: 'web-search',
  input: { mode: 'none', expected_revision: 2 },
});
assert.equal(
  cache.get(JSON.stringify(hooks.CHAT_CONNECTOR_PREFERENCE_KEYS.scope('org-a', 'owner-a'))).data
    .revision,
  9,
  'late mutation cannot overwrite a newer preference revision'
);
mutationOptions.onError();
assert.deepEqual(
  invalidations.pop().queryKey,
  hooks.CHAT_CONNECTOR_PREFERENCE_KEYS.scope('org-a', 'owner-a')
);

let finishOldRead;
fetcher = () =>
  new Promise(resolve => {
    finishOldRead = resolve;
  });
hooks.useChatConnectorPreferences(true);
const pendingOldRead = queryOptions.queryFn();
const newerResponse = { data: { revision: 10, defaults: { 'web-search': { mode: 'none' } } } };
writer = async () => newerResponse;
await mutation.mutateAsync({
  connectorID: 'web-search',
  input: { mode: 'none', expected_revision: 9 },
});
finishOldRead({ data: { revision: 9, defaults: {} } });
assert.equal(
  await pendingOldRead,
  newerResponse,
  'an old GET cannot overwrite the newer PUT response'
);
assert.equal(cache.get(JSON.stringify(queryOptions.queryKey)), newerResponse);

for (const locale of ['en-US', 'zh-Hans']) {
  const messages = load(`src/i18n/modules/connectors/${locale}.ts`).default;
  assert.ok(messages.defaults.scopeHint);
  assert.ok(messages.defaults.firstUseCheckbox);
  assert.ok(messages.defaults.selectionSavedDefaultFailed);
  assert.ok(messages.defaultsDraft.bypass);
}
console.log(
  'Connector preference API, scope isolation, malformed payloads, CAS cache ordering, and bilingual labels passed.'
);
