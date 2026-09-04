import assert from 'node:assert/strict';
import { load, jsx, ui, harness, nodes, flush } from './connector-fixture.mjs';
const metadata = load('src/components/connectors/connector-metadata.ts');
const setup = load('src/components/connectors/setup/app-setup-state.ts');
const model = load('src/components/connectors/catalog/catalog-model.ts', {
  '@/components/connectors/connector-metadata': metadata,
  '../setup/app-setup-state': setup,
});
const picker = load('src/components/connectors/picker/connector-picker-model.ts', {
  '../connector-metadata': metadata,
  '../setup/app-setup-state': setup,
});
const feishu = {
  id: 'feishu',
  name: '飞书 / Lark',
  description: '协作资源',
  classification: 'connector',
  key_fields: [],
  extension_schema_version: '1.1.0',
  setup: { handler: 'feishu-app-registration-v1', methods: ['manual', 'qr'] },
  credential_fields: [{ name: 'app_id' }, { name: 'app_secret' }, { name: 'region' }],
  capabilities: [
    { id: 'feishu.send', usable: false, unavailable_reason: 'connector_not_verified' },
  ],
  connection_summary: { count: 0 },
};
const common = {
  '@/hooks/connectors/use-pending-setups': { usePendingSetups: () => ({data: {data: {items: [],total_pending: 0}}}) },
  'react/jsx-runtime': jsx,
  'lucide-react': ui,
  '@/components/ui/button': ui,
  '@/components/ui/badge': ui,
  '@/components/ui/card': ui,
  '@/components/ui/dropdown-menu': ui,
  '@/i18n': {
    useT:
      () =>
      (key, values = {}) =>
        key + JSON.stringify(values),
  },
  '@/lib/utils': { cn: (...v) => v.filter(Boolean).join(' ') },
};
assert.equal(model.getConnectorCatalogRowState(feishu), 'not_connected');
assert.equal(
  setup.supportsConnectorSetup({ ...feishu, extension_schema_version: 'future' }),
  false
);
const stopped = {
  ...feishu,
  capabilities: [{ ...feishu.capabilities[0], unavailable_reason: 'plugin_unavailable' }],
};
assert.equal(setup.supportsConnectorSetup(stopped), false);
assert.equal(model.getConnectorCatalogRowState(stopped), 'unavailable');
for (const count of [0, 1, 3]) {
  const item = { ...feishu, connection_summary: { count, state: count ? 'ready' : undefined } };
  const connections = Array.from({ length: count }, (_, i) => ({
    id: 'account-' + i,
    connector_id: 'feishu',
    name: 'Account ' + i,
    state: 'ready',
  }));
  const rows = picker.createConnectorPickerRows([item], connections, []);
  const create = rows.filter(row => !row.connection);
  assert.equal(create.length, 1);
  assert.equal(create[0].state, 'not_connected');
  for (const target of [
    undefined,
    { type: 'chat', id: 'current-chat' },
    { type: 'agent', id: 'current-agent' },
  ]) {
    let received;
    await picker.requestConnectorPickerAction(
      create[0],
      {
        onRequestConnectAndAdd: request => {
          received = request;
        },
      },
      target
    );
    assert.equal(received.connector.id, 'feishu');
    assert.deepEqual(received.target, target);
    assert.equal(received.connection, undefined, 'adding is independent, not key replacement');
  }
  const h = harness(react => {
    const { ConnectorCatalogRow } = load(
      'src/components/connectors/catalog/connector-catalog-row.tsx',
      {
        ...common,
        react,
        'next/link': { default: 'Link' },
        '../setup/connector-create-dialog': ui,
        '../setup/app-setup-state': setup,
        '@/components/connectors/connector-icon': ui,
        './catalog-model': model,
      }
    );
    return () => ConnectorCatalogRow({ item });
  });
  h.render();
  const action = nodes(h.value).find(
    node => (node.type === 'Button' && typeof node.props.onClick === 'function') ||
      (node.type === 'DropdownMenuItem' && typeof node.props.onSelect === 'function')
  );
  (action.props.onClick ?? action.props.onSelect)();
  h.render();
  const dialog = nodes(h.value).find(node => node.type === 'ConnectorCreateDialog');
  assert.ok(dialog);
  assert.equal(dialog.props.connector, item);
  assert.equal(dialog.props.target, undefined, 'catalog only creates a saved connection');
  dialog.props.onOpenChange(false);
  h.render();
  assert.equal(
    nodes(h.value).some(node => node.type === 'ConnectorCreateDialog'),
    false
  );
  h.unmount();
}

const permissions = load('src/components/connectors/detail/permission-evidence-model.ts');
const template = {
  profile_version: '2.0.0',
  addons: { scopes: { tenant: ['read', 'write', 'read'], user: ['offline', 'read'] } },
};
let rows = permissions.connectorPermissionRows(template, { scope_status: 'verified' });
assert.ok(
  rows.every(row => row.status === 'unverified'),
  'missing evidence is not an empty verified list'
);
rows = permissions.connectorPermissionRows(template, {
  scope_status: 'verified',
  granted_scopes: ['read', 'legacy'],
});
assert.equal(rows.filter(row => row.scope === 'read' && row.identity === 'tenant').length, 1);
assert.equal(rows.find(row => row.scope === 'write').status, 'not_granted');
assert.equal(
  rows.find(row => row.scope === 'legacy').status,
  'verified',
  'older extra grants stay honest'
);
assert.ok(rows.filter(row => row.identity === 'user').every(row => row.status === 'unverified'));
assert.ok(
  permissions
    .connectorPermissionRows(template, { scope_status: 'verified', granted_scopes: [] })
    .filter(row => row.identity === 'tenant')
    .every(row => row.status === 'not_granted')
);
const { ConnectorAppPermissions } = load(
  'src/components/connectors/detail/connector-app-permissions.tsx',
  {
    ...common,
    'next-intl': { useLocale: () => 'zh-Hans' },
    '@/components/connectors/connector-metadata': metadata,
    './permission-evidence-model': permissions,
  }
);
for (const evidence of [
  undefined,
  { scope_status: 'verified' },
  { scope_status: 'verified', granted_scopes: ['read'] },
]) {
  const tree = ConnectorAppPermissions({
    connector: { ...feishu, permission_template: template },
    evidence,
  });
  assert.equal(nodes(tree).filter(node => node.type === 'details').length, 1);
  assert.equal(tree.props.open, undefined);
  assert.doesNotMatch(JSON.stringify(tree), /capabilitiesTitle/);
}
const h = harness(react => {
  const { ConnectorCapabilityList } = load(
    'src/components/connectors/detail/connector-capability-list.tsx',
    {
      ...common,
      react,
      'next-intl': { useLocale: () => 'zh-Hans' },
      '../connector-metadata': metadata,
    }
  );
  return () =>
    ConnectorCapabilityList({
      capabilities: Array.from({ length: 8 }, (_, i) => ({
        id: 'op' + i,
        name: 'English ' + i,
        name_i18n: { 'zh-Hans': '功能 ' + i },
        description: 'Description',
        description_i18n: { 'zh-Hans': '说明' },
        usable: true,
      })),
    });
});
h.render();
assert.equal(nodes(h.value).filter(node => node.type === 'li').length, 5);
assert.match(JSON.stringify(h.value), /功能/);
assert.doesNotMatch(JSON.stringify(h.value), /English|"children":"Description"/);
nodes(h.value)
  .find(node => node.type === 'Button')
  .props.onClick();
h.render();
assert.equal(nodes(h.value).filter(node => node.type === 'li').length, 8);
h.unmount();

let selected = false;
const writes = [];
const { ConnectionChatDefaultButton } = load(
  'src/components/connectors/connections/connection-chat-default-card.tsx',
  {
    ...common,
    sonner: { toast: { success() {}, error() {} } },
    '@/hooks/connectors/use-chat-connector-preferences': {
      useChatConnectorPreferences: () => ({
        data: {
          data: {
            revision: 7,
            defaults: selected ? { feishu: { mode: 'selected', connection_id: 'a' } } : {},
          },
        },
      }),
      useSetChatConnectorPreference: () => ({
        isPending: false,
        mutateAsync: async command => {
          writes.push(command);
        },
      }),
    },
  }
);
let star = ConnectionChatDefaultButton({
  connection: { id: 'a', connector_id: 'feishu', state: 'ready' },
});
assert.equal(star.props.variant, 'ghost');
assert.equal(star.props['aria-pressed'], false);
star.props.onClick();
await flush();
assert.deepEqual(writes[0].input, { mode: 'selected', connection_id: 'a', expected_revision: 7 });
selected = true;
star = ConnectionChatDefaultButton({
  connection: { id: 'a', connector_id: 'feishu', state: 'ready' },
});
assert.equal(star.props['aria-pressed'], true);
star.props.onClick();
await flush();
assert.deepEqual(writes[1].input, { mode: 'none', expected_revision: 7 });
console.log(
  'Feishu 0/1/many direct setup, current-target handoff, compact permissions/capabilities and management stars passed'
);
