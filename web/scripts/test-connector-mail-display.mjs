import assert from 'node:assert/strict';
import { load, harness, nodes, jsx, ui } from './connector-fixture.mjs';

const model = load('src/components/connectors/mail-display-model.ts');
const names = load('src/components/connectors/mail-connection-name.ts', {
  './mail-display-model': model,
});
const providers = ['gmail', 'qqmail', 'netease163'].map(id => ({
  id,
  name: id,
  connection_summary: { count: 2, needs_attention_count: 1 },
}));
const group = model.mailDisplayGroup(providers);
assert.equal(group.kind, 'mail');
assert.equal(group.count, 6);
assert.equal(group.attentionCount, 3);
assert.equal(Object.hasOwn(group, 'connector'), false);
assert.equal(Object.hasOwn(group, 'currentBinding'), false);
assert.equal(model.selectMailProvider(providers)?.id, 'qqmail');
assert.equal(model.selectMailProvider(providers, 'gmail')?.id, 'gmail');
assert.equal(model.selectMailProvider(providers.slice(0, 1))?.id, 'gmail');
assert.equal(model.mailDisplayGroup([{ id: 'exa' }]), undefined);
assert.equal(model.mailDisplayKey('qqmail'), 'mail');
assert.equal(model.mailDisplayKey('exa'), 'exa');
const account = { connector_id: 'qqmail', name: '123@qq.com', external_account_id: '123@qq.com' };
assert.equal(names.mailConnectionName(account), 'QQ邮箱-123的QQ邮箱');
assert.equal(names.mailConnectionName({ ...account, name: '工作邮箱' }), 'QQ邮箱-工作邮箱');
assert.equal(names.mailConnectionName({ ...account, name: 'QQ邮箱-工作邮箱' }), 'QQ邮箱-工作邮箱');
assert.equal(
  names.mailConnectionName({ ...account, name: '工作邮箱' }, 'en-US'),
  'QQ Mail-工作邮箱'
);
assert.equal(
  names.mailConnectionName({
    ...account,
    connector_id: 'gmail',
    name: 'Name+tag@work.example',
    external_account_id: 'Name+tag@work.example',
  }),
  'Gmail-Name+tag的Gmail'
);
assert.equal(names.mailConnectionAlias({ ...account, name: 'QQ邮箱-工作邮箱' }), '工作邮箱');
assert.equal(account.name, '123@qq.com', 'presentation never mutates saved names');

const catalogRow = harness(react => {
  const component = load('src/components/connectors/catalog/mail-catalog-row.tsx', {
    react,
    'react/jsx-runtime': jsx,
    'next/link': { default: 'Link' },
    'lucide-react': ui,
    '@/components/ui/button': ui,
    '@/components/ui/card': ui,
    '@/components/ui/dropdown-menu': ui,
    '@/i18n': {
      useT:
        () =>
        (key, values = {}) =>
          values.count === undefined ? key : `${key}:${values.count}`,
    },
    '@/lib/utils': { cn: (...values) => values.filter(Boolean).join(' ') },
    '../connector-icon': { ConnectorIcon: 'ConnectorIcon' },
    '../mail-display-model': model,
    '../setup/mail-setup-state': { mailSetupEnabled: () => true },
    '../setup/connector-create-dialog': { ConnectorCreateDialog: 'ConnectorCreateDialog' },
    '../connector-taxonomy-badges': { ConnectorTaxonomyBadges: 'ConnectorTaxonomyBadges' },
  });
  return currentGroup => component.MailCatalogRow({ group: currentGroup });
});

let catalogTree = nodes(catalogRow.render({ ...group, count: 0, attentionCount: 0 }));
assert.ok(
  catalogTree.some(node => node.type === 'KeyRound'),
  'empty mail uses not-connected icon'
);
assert.ok(
  catalogTree.some(
    node => node.type === 'p' && node.props.children === 'picker.states.notConnected'
  ),
  'empty mail uses the shared not-connected label instead of a zero count'
);
assert.match(
  catalogTree.find(node => node.type === 'Button').props.className,
  /w-full/,
  'empty mail connect action fills the same fixed action rail as other catalog cards'
);

catalogTree = nodes(catalogRow.render({ ...group, attentionCount: 0 }));
assert.ok(
  catalogTree.some(node => node.type === 'CircleCheck'),
  'connected mail uses success icon'
);
assert.ok(
  catalogTree.some(node => node.type === 'p' && node.props.children === 'catalog.connectedCount:6'),
  'connected mail uses the shared connected summary'
);
assert.ok(
  catalogTree.some(node => node.type === 'DropdownMenuItem'),
  'connected mail keeps add-connection in the shared secondary action menu'
);

catalogTree = nodes(catalogRow.render(group));
assert.ok(
  catalogTree.some(node => node.type === 'CircleAlert'),
  'mail attention uses warning icon'
);
assert.ok(
  catalogTree.some(
    node => node.type === 'p' && node.props.children === 'catalog.needsAttentionCount:3'
  ),
  'mail attention count remains visible'
);
console.log('Mail display grouping, catalog states, actions and localized names passed.');

const metadata = load('src/components/connectors/connector-metadata.ts');
const setup = load('src/components/connectors/setup/app-setup-state.ts');
const picker = load('src/components/connectors/picker/connector-picker-model.ts', {
  '../connector-metadata': metadata,
  '../setup/app-setup-state': setup,
});
const menu = load('src/components/connectors/picker/connector-picker-menu-model.ts', {
  './connector-picker-model': picker,
});
const family = load('src/components/connectors/picker/mail-picker-groups.ts');
const catalog = providers.map(item => ({
  ...item,
  plugin_id: item.id,
  key_fields: [{ name: 'token', required: true }],
  capabilities: [{ id: `${item.id}.message.list`, usable: true }],
}));
const accounts = catalog.flatMap(item =>
  [0, 1].map(i => ({
    id: `${item.id}-${i}`,
    connector_id: item.id,
    name: `Account ${i}`,
    state: 'ready',
    revision: 1,
  }))
);
const bindings = catalog.map(item => ({
  id: `binding-${item.id}`,
  connector_id: item.id,
  connection_id: `${item.id}-0`,
  status: 'active',
  revision: 1,
}));
const realGroups = menu.createConnectorPickerMenuGroups(catalog, accounts, bindings);
const [mailGroup] = family.groupMailPickerProviders(realGroups, '邮箱');
assert.equal(mailGroup.count, 6);
assert.equal(
  family.groupMailPickerProviders([realGroups[0]], '邮箱', realGroups)[0].count,
  6,
  'search preserves complete family counts and bindings'
);
assert.equal(mailGroup.providers.length, 3);
assert.equal(Object.hasOwn(mailGroup, 'currentBinding'), false);
assert.equal(Object.hasOwn(mailGroup, 'canToggle'), false);
assert.equal(Object.hasOwn(mailGroup, 'connector'), false);
const selectedIDs = mailGroup.providers.map(group => group.currentBinding.connection_id);
assert.deepEqual(new Set(selectedIDs), new Set(bindings.map(binding => binding.connection_id)));
for (const target of [
  { type: 'chat', id: 'chat-1' },
  { type: 'agent', id: 'agent-1' },
  { type: 'workflow', id: 'workflow-1' },
]) {
  for (const group of mailGroup.providers) {
    let request;
    await picker.requestConnectorPickerAction(
      group.createRow,
      {
        onRequestConnectAndAdd: value => {
          request = value;
        },
      },
      target
    );
    assert.equal(request.connector.id, group.id);
    assert.equal(request.target, target);
    let switched;
    await menu.requestConnectorPickerMenuAccount(
      group.accounts[1],
      {
        onRequestSwitch: value => {
          switched = value;
        },
      },
      target
    );
    assert.equal(switched.replacedBinding.connector_id, group.id);
    assert.equal(switched.connection.connector_id, group.id);
  }
}
const requests = [];
const rowHarness = harness(react => {
  const component = load('src/components/connectors/picker/mail-picker-group-row.tsx', {
    react,
    'react/jsx-runtime': jsx,
    'lucide-react': ui,
    '@/components/ui/button': ui,
    '@/components/ui/dropdown-menu': ui,
    '@/i18n': { useT: () => key => key },
    './connector-picker-group-row': { ConnectorPickerGroupRow: 'ProviderGroup' },
  });
  return () =>
    component.MailPickerGroupRow({
      group: mailGroup,
      disabled: false,
      targetLabel: 'Chat',
      onSelect: row => requests.push(row),
      onDetach() {},
    });
});
const rowTree = nodes(rowHarness.render());
assert.equal(rowTree.filter(node => node.type === 'Button').length, 1, 'one family add action');
assert.equal(
  rowTree.filter(node => node.type === 'ProviderGroup' && node.props.hideCreate).length,
  3
);
rowTree.filter(node => node.type === 'DropdownMenuItem')[1].props.onSelect();
assert.equal(requests[0].connector.id, 'netease163', 'selected provider survives handoff');

const selector = harness(react => {
  const component = load(
    'src/components/connectors/setup/connector-mail-provider-panel.tsx',
    {
      react,
      'react/jsx-runtime': jsx,
      'next-intl': { useLocale: () => 'zh-Hans' },
      '@/components/ui/label': ui,
      '@/components/ui/select': ui,
      '@/i18n': { useT: () => key => key },
      '@/store/auth-store': {},
      '@/store/organization-store': {},
      './connector-mail-setup-panel': { ConnectorMailSetupPanel: 'MailForm' },
    },
    '\nexports.Selection = MailProviderSelection;'
  );
  return () => component.Selection({ providers: catalog });
});
let selectionTree = nodes(selector.render());
assert.equal(selectionTree.find(node => node.type === 'Select').props.value, 'qqmail');
const firstKey = selectionTree.find(node => node.type === 'MailForm').key;
selectionTree.find(node => node.type === 'Select').props.onValueChange('gmail');
selectionTree = nodes(selector.render());
assert.equal(selectionTree.find(node => node.type === 'MailForm').props.connector.id, 'gmail');
assert.notEqual(
  selectionTree.find(node => node.type === 'MailForm').key,
  firstKey,
  'switch remounts form and clears secret/state'
);
selectionTree.find(node => node.type === 'MailForm').props.onBusyChange(true);
selectionTree = nodes(selector.render());
assert.equal(selectionTree.find(node => node.type === 'Select').props.disabled, true);
selectionTree.find(node => node.type === 'Select').props.onValueChange('qqmail');
assert.equal(
  nodes(selector.render()).find(node => node.type === 'MailForm').props.connector.id,
  'gmail',
  'cannot switch during verification'
);
console.log(
  'Independent mail bindings, real-provider handoff and locked/remounted provider selection passed.'
);
