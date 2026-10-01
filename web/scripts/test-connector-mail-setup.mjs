import assert from 'node:assert/strict';
import { load, harness, flush, nodes, jsx, ui, read } from './connector-fixture.mjs';

const app = load('src/components/connectors/setup/app-setup-state.ts');
const mail = load('src/components/connectors/setup/mail-setup-state.ts', {
  './app-setup-state': app,
});
const state = load('src/components/connectors/setup/key-setup-state.ts');
assert.equal(mail.connectionMailUsage(), 'read_send');
assert.equal(
  mail.connectionMailUsage({
    connector_id: 'qqmail',
    usable_operation_ids: ['qqmail.message.send', 'qqmail.message.list'],
  }),
  'read_send'
);
assert.equal(
  mail.connectionMailUsage({
    connector_id: 'qqmail',
    usable_operation_ids: ['qqmail.message.send'],
  }),
  'send'
);
const errors = load('src/hooks/connectors/connector-request-error.ts');
const partial = { usage: 'read_send', imap_status: 'verified', smtp_status: 'failed' };
assert.deepEqual(
  mail.mailFailureVerification({
    response: { data: { mail_verification: { ...partial, app_password: 'MUST_NOT_SURVIVE' } } },
  }),
  partial
);
assert.equal(
  mail.mailFailureVerification({
    response: {
      data: { mail_verification: { ...partial, smtp_status: 'untrusted raw response' } },
    },
  }),
  undefined
);
assert.equal(mail.mailFailureMessage('connector_mail_policy_restricted'), 'mail.policyRestricted');
const connector = {
  id: 'gmail',
  name: 'Gmail',
  extension_schema_version: '1.4.0',
  key_fields: [],
  setup: { handler: 'gmail-app-password-v1', methods: ['manual'] },
  credential_fields: [
    { name: 'email', required: true, secret: false },
    { name: 'app_password', required: true, secret: true },
  ],
  capabilities: [{ id: 'gmail.message.list', usable: false }],
  connection_summary: { count: 0 },
};
assert.equal(app.supportsConnectorSetup(connector), true);
assert.equal(
  app.supportsConnectorSetup({ ...connector, extension_schema_version: '1.3.0' }),
  false
);
assert.equal(
  app.supportsConnectorSetup({ ...connector, setup_availability: { enabled: false, methods: [] } }),
  false
);
assert.equal(
  app.supportsMailSetup({ ...connector, credential_fields: [{ name: 'token', secret: true }] }),
  false
);
for (const email of ['person+tag@gmail.com', 'first.last@company.example', 'Name@googlemail.com']) {
  assert.equal(mail.normalizeMailEmail(` ${email} `), email, 'preserve full authenticated login');
}
for (const email of [
  'person',
  'a b@gmail.com',
  'Name <a@gmail.com>',
  'a@gmail.com,b@gmail.com',
  'a\u200b@gmail.com',
  'a@b\nc.com',
]) {
  assert.equal(mail.normalizeMailEmail(email), undefined);
}
assert.equal(mail.normalizeMailAppPassword('Abcd efgh\tijkl\r\nmnop\u00a0'), 'Abcdefghijklmnop');
for (const password of [
  'abcd\u200befghijklmnop',
  'abcdefghijklmnop\u202e',
  'abcd\vefghijklmnop',
]) {
  assert.equal(mail.normalizeMailAppPassword(password), undefined);
}
assert.equal(mail.normalizeMailAppPassword('my-normal-password'), 'my-normal-password');
assert.equal(
  mail.normalizeMailAppPassword('abcd1234efgh5678'),
  'abcd1234efgh5678',
  'do not invent an alphabet-only password contract'
);
assert.equal(mail.normalizeMailEmail('Name+tag@COMPANY.example'), 'Name+tag@company.example');
const secret = 'Abcdefghijklmnop';
let pending;
const calls = [];
const api = {
  startKeySetup: (id, input, key) => {
    calls.push({ id, input, key, snapshot: globalThis.structuredClone(input) });
    return pending;
  },
  reconnectConnection: (id, input, key) => {
    calls.push({ id, input, key, snapshot: globalThis.structuredClone(input) });
    return pending;
  },
};
let queryOptions;
let invalidations = 0;
function createHook(connection, selectedConnector = connector) {
  return harness(react => {
    const module = load('src/hooks/connectors/use-connector-mail-setup.ts', {
      react,
      '@tanstack/react-query': {
        useQuery: options => {
          queryOptions = options;
          return {};
        },
        useQueryClient: () => ({}),
      },
      '@/hooks/query-keys': { CONNECTOR_KEYS: { setup: id => ['setup', id] } },
      '@/services/connector.service': { connectorService: api },
      '@/components/connectors/setup/key-setup-state': state,
      '@/components/connectors/setup/mail-setup-state': mail,
      './connector-request-error': errors,
      './connection-detail-query': { invalidatePersonalConnectionQueries: () => invalidations++ },
    });
    return () => module.useConnectorMailSetup(selectedConnector, connection);
  });
}
for (const usage of ['read', 'send', 'read_send']) {
  let resolve;
  pending = new Promise(done => {
    resolve = done;
  });
  const h = createHook();
  let hook = h.render();
  assert.equal(hook.setUsage, undefined);
  hook.setEmail('person+tag@company.example');
  hook.setPassword(secret);
  hook = h.render();
  assert.equal(hook.canSubmit, true);
  const submission = hook.submit();
  hook.submit();
  hook = h.render();
  assert.equal(hook.password, '', 'clear password as soon as submit starts');
  assert.equal(hook.state.status, 'verifying');
  assert.equal(calls.at(-1).snapshot.mail_usage, 'read_send');
  assert.equal(Object.hasOwn(queryOptions, 'mutationFn'), false);
  assert.equal(JSON.stringify(queryOptions.queryKey).includes(secret), false);
  resolve({
    data: {
      state: 'failed',
      attempt_id: usage,
      error_code: 'connector_credentials_invalid',
      retryable: true,
    },
  });
  await submission;
  hook = h.render();
  assert.deepEqual(calls.at(-1).input.values, {}, 'wipe request object after completion');
  assert.equal(hook.state.status, 'failed');
  assert.equal(hook.setUsage, undefined, 'failure does not offer single-direction downgrade');
  hook.reset();
  hook = h.render();
  assert.equal(hook.email, 'person+tag@company.example');
  assert.equal(hook.password, '');
  assert.equal(hook.canSubmit, false, 'retry requires password again');
  h.unmount();
}
assert.equal(calls.length, 3, 'double submit is blocked');
let rejectPartial;
pending = new Promise((_resolve, reject) => {
  rejectPartial = reject;
});
const partialHook = createHook();
let partialView = partialHook.render();
partialView.setEmail('person@company.example');
partialView.setPassword(secret);
partialView = partialHook.render();
const partialOperation = partialView.submit();
rejectPartial({
  response: {
    status: 422,
    data: {
      code: 'connector.mail.policy_restricted',
      mail_verification: { ...partial, password: secret },
    },
  },
});
await partialOperation;
partialView = partialHook.render();
assert.equal(partialView.state.status, 'failed');
assert.equal(
  partialView.state.connection,
  undefined,
  'partial verification cannot activate a connection'
);
assert.deepEqual(partialView.verification, partial);
assert.equal(partialView.password, '');
assert.equal(JSON.stringify(partialView.verification).includes(secret), false);
partialView.reset();
partialView = partialHook.render();
partialView = partialHook.render();
assert.equal(partialView.setUsage, undefined);
assert.equal(partialView.canSubmit, false, 'an explicit retry still requires the password');
partialHook.unmount();
const connection = {
  id: 'original',
  external_account_id: 'Original+tag@company.example',
  provider_evidence: { mail: { usage: 'send' } },
};
let resolve;
pending = new Promise(done => {
  resolve = done;
});
const reconnect = createHook(connection);
let hook = reconnect.render();
assert.equal(hook.setUsage, undefined);
hook.setEmail('different@gmail.com');
hook.setPassword(secret);
hook = reconnect.render();
assert.equal(hook.email, connection.external_account_id, 'reconnect locks email');
const operation = hook.submit();
assert.equal(calls.at(-1).id, 'original');
assert.equal(
  calls.at(-1).snapshot.mail_usage,
  'read_send',
  'explicit reconnect upgrades historic one-direction use'
);
assert.equal(calls.at(-1).snapshot.values.email, connection.external_account_id);
reconnect.unmount();
assert.deepEqual(calls.at(-1).input.values, {}, 'unmount clears in-flight request values');
resolve({ data: { state: 'ready', attempt_id: 'late', connection: { id: 'original' } } });
await operation;
assert.equal(invalidations, 0, 'late response cannot mutate the next account scope');

// The shared picker preserves independent creation at every account count and host surface.
const picker = load('src/components/connectors/picker/connector-picker-model.ts', {
  '../setup/app-setup-state': app,
  '../connector-metadata': load('src/components/connectors/connector-metadata.ts'),
});
const menu = load('src/components/connectors/picker/connector-picker-menu-model.ts', {
  './connector-picker-model': picker,
});
for (const type of ['chat', 'agent', 'workflow'])
  for (const count of [0, 1, 3]) {
    const accounts = Array.from({ length: count }, (_, index) => ({
      id: `mail-${index}`,
      connector_id: 'gmail',
      name: `Mailbox ${index}`,
      state: 'ready',
      revision: 1,
    }));
    const availableConnector = {
      ...connector,
      capabilities: connector.capabilities.map(capability => ({
        ...capability,
        usable: count > 0,
      })),
    };
    const [group] = menu.createConnectorPickerMenuGroups([availableConnector], accounts, []);
    assert.equal(group.createRow.state, 'not_connected');
    assert.equal(group.count, count);
    assert.equal(
      group.canToggle,
      count === 1,
      'one account can toggle; multiple accounts require a selection'
    );
    const requests = [];
    const added = [];
    await menu.requestConnectorPickerMenuToggle(
      group,
      { onRequestAdd: request => added.push(request) },
      { type, id: 'draft' }
    );
    assert.equal(added.length, count === 1 ? 1 : 0);
    await picker.requestConnectorPickerAction(
      group.createRow,
      { onRequestConnectAndAdd: request => requests.push(request) },
      { type, id: 'draft' }
    );
    assert.equal(requests.at(-1).target.type, type);
  }

// Render the real form with an isolated hook: exactly two credentials, fixed read and send with no usage control.
const display = {
  state: { status: 'idle' },
  email: connection.external_account_id,
  password: '',
  usage: 'send',
  setEmail() {},
  setPassword() {},
  setUsage() {},
};
const rendered = harness(react => {
  const panel = load(
    'src/components/connectors/setup/connector-mail-setup-panel.tsx',
    {
      react,
      'react/jsx-runtime': jsx,
      'next/link': ui,
      '@/components/ui/alert': ui,
      '@/components/ui/button': ui,
      '@/components/ui/input': ui,
      '@/components/ui/label': ui,
      '@/components/ui/collapsible': ui,
      'lucide-react': ui,
      '@/i18n': { useT: () => key => key },
      '@/store/auth-store': {},
      '@/store/organization-store': {},
      '@/hooks/connectors/use-connector-mail-setup': { useConnectorMailSetup: () => display },
      './connector-setup-status': { ConnectorSetupStatus: 'Status' },
      '../detail/connector-mail-information': { ConnectorMailInformation: 'Information' },
      './use-creation-default': { useCreationDefault: () => ({ option() {} }) },
      './mail-setup-state': mail,
    },
    '\nexports.MailSetup = MailSetup;'
  );
  return () => panel.MailSetup({ connector, updateConnection: connection });
});
const tree = nodes(rendered.render());
assert.equal(tree.filter(node => node.type === 'Input').length, 2);
assert.equal(tree.find(node => node.props.id === 'gmail-email').props.readOnly, true);
assert.equal(tree.find(node => node.props.id === 'gmail-app-password').props.type, 'password');
assert.equal(tree.filter(node => node.props.name === 'mail_usage').length, 0);
display.state = { status: 'failed', errorCode: 'connector_mail_policy_restricted' };
display.verification = partial;
const failedTree = JSON.stringify(rendered.render());
assert.match(failedTree, /mail.protocol.verified/);
assert.match(failedTree, /mail.protocol.failed/);
assert.match(failedTree, /mail.sendFailed/);
assert.doesNotMatch(failedTree, /mail.verifiedHint/);
for (const file of [
  'src/hooks/connectors/use-connector-mail-setup.ts',
  'src/components/connectors/setup/connector-mail-setup-panel.tsx',
]) {
  assert.doesNotMatch(read(file), /localStorage|sessionStorage|useMutation|console\./);
}
await flush();
console.log(
  'Gmail fields, usage, normalization, locked reconnect, request cleanup, account isolation and 0/1/multi-account host entry tests passed.'
);

for (const provider of ['qqmail', 'netease163']) {
  const selected = {
    ...connector,
    id: provider,
    extension_schema_version: '1.5.0',
    setup: { handler: `${provider}-authorization-code-v1`, methods: ['manual'] },
    credential_fields: [
      { name: 'email', required: true, secret: false },
      { name: 'authorization_code', required: true, secret: true },
    ],
  };
  assert.equal(app.supportsMailSetup(selected), true);
  assert.equal(app.supportsMailSetup({ ...selected, setup: connector.setup }), false);
  const email = provider === 'qqmail' ? 'User+tag@qq.com' : 'User+tag@163.com';
  assert.equal(mail.normalizeMailProfileEmail(provider, email), email);
  assert.equal(mail.normalizeMailProfileEmail(provider, 'user@gmail.com'), undefined);
  assert.equal(mail.normalizeMailSecret(provider, ' Abcdefghijklmnop '), 'Abcdefghijklmnop');
  assert.equal(
    mail.normalizeMailSecret(provider, 'Abcd efghijklmnop'),
    provider === 'netease163' ? 'Abcdefghijklmnop' : 'Abcd efghijklmnop'
  );
  assert.equal(
    mail.normalizeMailSecret(provider, 'ABCD\u00a0EFGH\nIJKL\tMNOP'),
    provider === 'netease163' ? 'ABCDEFGHIJKLMNOP' : undefined
  );
  assert.equal(
    mail.normalizeMailSecret(provider, 'abcd1234efgh5678'),
    'abcd1234efgh5678'
  );
  for (const usage of ['read', 'send', 'read_send']) {
    let resolve;
    pending = new Promise(done => {
      resolve = done;
    });
    const h = createHook(undefined, selected);
    let hook = h.render();
    hook.setEmail(email);
    hook.setPassword(secret);
    hook = h.render();
    assert.equal(hook.canSubmit, true);
    const submission = hook.submit();
    assert.equal(calls.at(-1).id, provider);
    assert.deepEqual(calls.at(-1).snapshot, {
      values: { email, authorization_code: secret },
      mail_usage: 'read_send',
    });
    resolve({
      data: {
        state: 'failed',
        attempt_id: provider + usage,
        error_code: 'connector_mail_authentication_rejected',
      },
    });
    await submission;
    assert.deepEqual(calls.at(-1).input.values, {});
    assert.equal(h.render().password, '');
    h.unmount();
  }
}
console.log('QQ/163 formats, provider routing, selected usage and credential cleanup passed.');
