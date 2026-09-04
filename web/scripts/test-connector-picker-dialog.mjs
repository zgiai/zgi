import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const read = file => readFileSync(path.join(process.cwd(), file), 'utf8');
const element = (type, props) => ({ type, props: props ?? {} });
const nodes = tree =>
  Array.isArray(tree)
    ? tree.flatMap(nodes)
    : tree && typeof tree === 'object'
      ? [tree, ...nodes(tree.props?.children)]
      : [];
const text = tree =>
  Array.isArray(tree)
    ? tree.map(text).join(' ')
    : tree && typeof tree === 'object'
      ? text(tree.props?.children)
      : String(tree ?? '');
const fakeEvent = () => ({
  prevented: false,
  preventDefault() {
    this.prevented = true;
  },
});
const flush = async () => {
  for (let i = 0; i < 8; i++) await Promise.resolve();
};
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
};
function load(file, imports = {}) {
  const code = ts.transpileModule(read(file), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText;
  const module = { exports: {} };
  new Function('require', 'module', 'exports', code)(
    name => {
      if (name === '../setup/app-setup-state') return load('src/components/connectors/setup/app-setup-state.ts');
      assert.ok(imports[name], `${file}: unexpected import ${name}`);
      return imports[name];
    },
    module,
    module.exports
  );
  return module.exports;
}
const metadata = load('src/components/connectors/connector-metadata.ts');
const pickerModel = load('src/components/connectors/picker/connector-picker-model.ts', {
  '../connector-metadata': metadata,
});
const groupModel = load('src/components/connectors/picker/connector-picker-menu-model.ts', {
  './connector-picker-model': pickerModel,
});
const catalogModel = load('src/components/connectors/catalog/catalog-model.ts', {
  '@/components/connectors/connector-metadata': metadata,
});
const translate = (key, values = {}) =>
  `${key}${Object.keys(values).length ? `:${JSON.stringify(values)}` : ''}`;
const ui = new Proxy({}, { get: (_, name) => name });
let pendingRecords = [];
const common = {
  '@/hooks/connectors/use-pending-setups': { usePendingSetups: () => ({data: {data: {items: pendingRecords,total_pending: pendingRecords.length}}}) },
  '@/components/connectors/setup/connector-key-setup-panel': {ConnectorKeySetupPanel: 'ConnectorKeySetupPanel'},
  'react/jsx-runtime': { jsx: element, jsxs: element, Fragment: 'Fragment' },
  'lucide-react': ui,
  '@/i18n': { useT: () => translate },
  '@/lib/utils': { cn: (...values) => values.filter(Boolean).join(' ') },
  '@/components/ui/button': ui,
  '@/components/ui/dialog': ui,
  '@/components/ui/input': ui,
  '@/components/ui/skeleton': ui,
  '@/components/ui/confirm-dialog': ui,
  '@/components/ui/checkbox': ui,
  '@/components/ui/dropdown-menu': ui,
  '@/components/connectors/connector-icon': ui,
  '../connector-visibility': load('src/components/connectors/connector-visibility.ts'),
  './connector-picker-menu-model': groupModel,
  './connector-first-use-dialog': ui,
  sonner: { toast: { success() {}, error() {} } },
};

const github = {
  id: 'github',
  plugin_id: 'zgi.github',
  name: 'GitHub',
  description: 'Read repositories',
  classification: 'connector',
  key_fields: [{ name: 'token', required: true }],
  capabilities: [{ id: 'repo.list', name: 'Repositories', usable: true }],
  connection_summary: { count: 3, state: 'ready', needs_attention_count: 1 },
};
const exa = { ...github, id: 'exa', name: 'Exa Web Search' };
const feishu = { ...github, id: 'feishu', name: 'Feishu', key_fields: [],
  extension_schema_version: '1.1.0',
  setup: { handler: 'feishu-app-registration-v1', methods: ['manual', 'qr'] },
  credential_fields: [{name: 'app_id'}, {name: 'app_secret'}, {name: 'region'}],
  capabilities: [{id: 'feishu.messages.send', usable: false, unavailable_reason: 'connector_not_verified'}],
};
const connection = (id, state = 'ready', connector_id = 'github') => ({
  id,
  name: `Account ${id}`,
  external_display_name: 'same-user',
  connector_id,
  state,
  revision: 1,
});
const connections = [
  connection('a'),
  connection('b'),
  connection('old', 'reconnect_required'),
  connection('exa', 'ready', 'exa'),
];
const binding = {
  id: 'binding-a',
  connector_id: 'github',
  connection_id: 'a',
  status: 'active',
  revision: 3,
};
const target = { type: 'chat', id: 'conversation-a' };
const catalog = [github, exa, feishu];
const groups = groupModel.createConnectorPickerMenuGroups(catalog, connections, [binding]);
assert.equal(groups.length, 3);
assert.equal(
  groups[0].count,
  3,
  'one service card counts saved connections, not the create action'
);
assert.equal(groups[2].count, 0);
for (const filters of [
  { query: 'Account old', status: 'all' },
  { query: '', status: 'needs_attention' },
]) {
  const filtered = groupModel.filterConnectorPickerGroups(groups, filters);
  assert.equal(filtered.length, 1);
  assert.equal(filtered[0].count, 3, 'search/filter must not under-report saved connections');
  assert.equal(
    filtered[0].accounts.length,
    3,
    'repair and alternate connections remain accessible'
  );
  assert.ok(filtered[0].createRow);
}
assert.deepEqual(
  groupModel.filterConnectorPickerGroups(groups, { query: 'missing', status: 'all' }),
  []
);

// Execute actual component callbacks, without starting a server/browser. The UI
// primitives retain responsibility for DOM focus trapping and keyboard navigation.
function mount(file, component, initialProps, imports = {}) {
  const hooks = [];
  let cursor = 0,
    effects = [],
    tree,
    props = initialProps;
  const react = {
    useRef(initial) {
      const index = cursor++;
      return (hooks[index] ??= { current: initial });
    },
    useState(initial) {
      const index = cursor++;
      if (!(index in hooks)) hooks[index] = typeof initial === 'function' ? initial() : initial;
      return [
        hooks[index],
        value => {
          hooks[index] = typeof value === 'function' ? value(hooks[index]) : value;
        },
      ];
    },
    useMemo(factory) {
      cursor++;
      return factory();
    },
    useEffect(callback, dependencies) {
      const index = cursor++,
        previous = hooks[index];
      if (!previous || dependencies.some((value, i) => value !== previous.dependencies[i])) {
        hooks[index] = { dependencies, cleanup: previous?.cleanup };
        effects.push(() => {
          hooks[index].cleanup?.();
          hooks[index].cleanup = callback();
        });
      }
    },
  };
  const instance = load(file, { ...common, ...imports, react })[component];
  const render = (patch = {}) => {
    props = { ...props, ...patch };
    cursor = 0;
    tree = instance(props);
    const pending = effects;
    effects = [];
    pending.forEach(effect => effect());
    return tree;
  };
  render();
  render();
  return {
    render,
    all: type => nodes(tree).filter(node => node.type === type),
    find: type => nodes(tree).find(node => node.type === type),
    unmount: () => hooks.forEach(hook => hook?.cleanup?.()),
  };
}

let selected = [],
  detached = 0,
  managed = 0;
const groupRow = mount(
  'src/components/connectors/picker/connector-picker-group-row.tsx',
  'ConnectorPickerGroupRow',
  {
    group: groups[0],
    targetLabel: 'Chat',
    disabled: false,
    onSelect: row => selected.push(row),
    onDetach: () => detached++,
    onManage: () => managed++,
  }
);
assert.equal(groupRow.all('article').length, 1);
assert.match(text(groupRow.find('DropdownMenuTrigger')), /Account a/);
assert.match(text(groupRow.find('DropdownMenuTrigger')), /status.ready/);
groupRow.find('DropdownMenu').props.onOpenChange(true);
assert.equal(selected.length, 0, 'opening the count menu never changes the current connection');
assert.equal(groupRow.find('DropdownMenuRadioGroup').props.value, 'a');
assert.equal(
  groupRow.all('DropdownMenuRadioItem').length,
  2,
  'invalid credentials are repair actions, not radio choices'
);
groupRow.all('DropdownMenuRadioItem')[1].props.onSelect();
assert.equal(selected[0].connection.id, 'b');
const handoff = fakeEvent();
groupRow.find('DropdownMenuContent').props.onCloseAutoFocus(handoff);
assert.equal(handoff.prevented, true, 'closing dropdown must not steal confirmation-dialog focus');
groupRow
  .all('DropdownMenuItem')
  .find(node => node.props['aria-label']?.includes('Account old'))
  .props.onSelect();
assert.equal(selected.at(-1).state, 'reconnect');
assert.equal(groupRow.all('DropdownMenuSub').length, 0);
assert.equal(groupRow.all('DropdownMenuItem').some(node => text(node).includes('catalog.manageConnections')), false);
groupRow
  .all('DropdownMenuItem')
  .find(node => text(node).includes('picker.group.removeFromTarget'))
  .props.onSelect();
assert.equal(detached, 1);
groupRow.render({ disabled: true });
assert.ok(groupRow.all('DropdownMenuRadioItem').every(node => node.props.disabled));
assert.ok(groupRow.all('DropdownMenuItem').every(node => node.props.disabled));
assert.equal(groupRow.find('Button').props.disabled, true);
groupRow.render({ group: groups[2], disabled: false });
assert.equal(groupRow.find('DropdownMenu'), undefined);
groupRow.find('Button').props.onClick();
assert.equal(selected.at(-1).connector.id, 'feishu');
assert.equal(selected.at(-1).state, 'not_connected');

// Schema 1.3 adds host surfaces without changing how personal credentials are created.
for (const service of [github, { ...exa, id: 'web-search' }, feishu]) {
  for (const schema of [service.extension_schema_version, '1.3.0']) {
    const descriptor = {
      ...service, extension_schema_version: schema,
      capabilities: service.capabilities.map(capability => ({
        ...capability, usable: true, unavailable_reason: undefined,
      })),
    };
    const saved = connection('saved', 'ready', service.id);
    const current = { ...binding, connector_id: service.id, connection_id: saved.id };
    const [group] = groupModel.createConnectorPickerMenuGroups([descriptor], [saved], [current]);
    assert.equal(group.count, 1);
    assert.equal(group.createRow?.state, 'not_connected', `${service.id}/${schema}: existing accounts retain creation`);
    for (const status of ['all', 'connected']) {
      assert.equal(
        groupModel.filterConnectorPickerGroups([group], { query: saved.name, status })[0]?.createRow,
        group.createRow,
        'searching existing accounts must retain the new-connection action'
      );
    }
    for (const type of ['chat', 'agent']) {
      const requests = [];
      let selection;
      const row = mount(
        'src/components/connectors/picker/connector-picker-group-row.tsx',
        'ConnectorPickerGroupRow',
        {
          group, targetLabel: type, disabled: false,
          onSelect: candidate => {
            selection = groupModel.requestConnectorPickerMenuAccount(candidate, {
              onRequestConnectAndAdd: request => requests.push(request),
              onRequestSwitch: () => assert.fail('creation must not switch the current account'),
              onRequestUpdateKey: () => assert.fail('creation must not repair the current account'),
            }, { type, id: 'target' });
          },
          onDetach: () => assert.fail('creation must not remove the current account'),
        }
      );
      const add = row.all('DropdownMenuItem').filter(node => text(node).includes('picker.actions.addConnection'));
      assert.equal(add.length, 1, `${type}/${service.id}: one new-connection menu item`);
      add[0].props.onSelect();
      await selection;
      assert.equal(requests.length, 1);
      assert.equal(requests[0].connector, descriptor);
      assert.equal(requests[0].connection, undefined);
      assert.equal(requests[0].binding, undefined);
      assert.equal(requests[0].replacedBinding, current);
      assert.equal(requests[0].target.type, type);
      row.unmount();
    }
  }
}

const query = items => ({
  data: { data: { items } },
  isLoading: false,
  isError: false,
  refetch: async () => {},
});
let loading = false,
  failed = false;
const connectionQueries = [];
let preferences = { revision: 1, defaults: { github: { mode: 'none' }, exa: { mode: 'none' } } };
let preferenceFailure = false;
const preferenceWrites = [];
const mountPicker = (overrides = {}) =>
  mount(
    'src/components/connectors/picker/connector-picker-dialog.tsx',
    'ConnectorPickerDialog',
    {
      open: true,
      groupByConnector: true,
      target,
      targetName: 'Chat',
      bindings: [binding],
      onOpenChange: () => {},
      onRequestAdd: () => {},
      onRequestSwitch: () => {},
      onRequestConnectAndAdd: () => {},
      onRequestUpdateKey: () => {},
      onRequestViewReason: () => {},
      onRequestDetach: () => {},
      ...overrides,
    },
    {
      '@/components/connectors/connector-metadata': {
        localizeConnectorCatalogItem: value => value,
      },
      '@/hooks/connectors/use-connectors': {
        useConnectorCatalog: () => ({ ...query(catalog), isLoading: loading, isError: failed }),
        usePersonalConnections: (filters, enabled) => {
          connectionQueries.push({ filters, enabled });
          return query(connections);
        },
        connectorCatalogItems: value => value?.items ?? [],
        personalConnectionItems: value => value?.items ?? [],
      },
      '@/hooks/use-locale': { useLocale: () => ({ locale: 'en-US' }) },
      '@/hooks/connectors/use-chat-connector-preferences': {
        useChatConnectorPreferences: () => ({
          data: { data: preferences },
          scope: 'owner/org',
          isError: false,
          refetch: async () => {},
        }),
        useSetChatConnectorPreference: () => ({
          isPending: false,
          mutateAsync: async input => {
            preferenceWrites.push(input);
            if (preferenceFailure) throw new Error('preference conflict');
            return { data: preferences };
          },
        }),
      },
      './connector-picker-model': pickerModel,
      './connector-picker-menu-model': groupModel,
      './connector-picker-group-row': ui,
      './connector-picker-row': ui,
      '../unavailable-references': ui,
    }
  );
const draftPicker = mountPicker({
  draft: true,
  target: { type: 'chat', id: 'draft-connectors-1' },
  bindings: [],
});
assert.deepEqual(
  connectionQueries.at(-1),
  { filters: { connector_id: undefined }, enabled: true },
  'new-chat picker loads personal connections without sending a fake target'
);
assert.equal(draftPicker.find('ConnectorUnavailableReferences').props.enabled, false);
draftPicker.render({ open: false });
assert.equal(connectionQueries.at(-1).enabled, false);
mountPicker();
assert.deepEqual(
  connectionQueries.at(-1).filters,
  { connector_id: undefined, target_type: 'chat', target_id: target.id },
  'existing chat retains its target-scoped queries'
);
const groupView = harness => harness.find('ConnectorPickerGroupRow');
const chooseB = harness =>
  groupView(harness).props.onSelect(groupView(harness).props.group.accounts[1]);
const confirm = harness => harness.find('ConfirmDialog');
let switches = 0,
  removes = 0;
const picker = mountPicker({ onRequestSwitch: () => switches++, onRequestDetach: () => removes++ });
assert.equal(picker.all('ConnectorPickerGroupRow').length, 3);
assert.equal(picker.all('ConnectorPickerRow').length, 0);
assert.ok(picker.find('SearchInput'));
assert.match(text(picker.find('DialogFooter')), /picker.group.footer/);
groupView(picker).props.onSelect(groupView(picker).props.group.accounts[0]);
await flush();
picker.render();
assert.equal(removes, 0, 'clicking the current account cannot detach it');
chooseB(picker);
await flush();
picker.render();
assert.equal(switches, 0, 'choosing a different account stages confirmation');
assert.equal(confirm(picker).props.open, true);
confirm(picker).props.onCancel();
picker.render();
assert.equal(confirm(picker).props.open, false);
assert.equal(switches, 0);
chooseB(picker);
await flush();
picker.render({ bindings: [{ ...binding, revision: 4 }] });
confirm(picker).props.onConfirm();
await flush();
picker.render();
assert.equal(switches, 0, 'stale revisions must not submit');
assert.equal(confirm(picker).props.open, false);

const save = deferred();
const once = mountPicker({
  onRequestSwitch: () => {
    switches++;
    return save.promise;
  },
});
chooseB(once);
await flush();
once.render();
const submit = confirm(once).props.onConfirm;
submit();
submit();
assert.equal(switches, 1, 'synchronous double clicks only submit once');
save.resolve();
await flush();
once.render();
assert.equal(confirm(once).props.open, false);

const first = deferred(),
  second = deferred();
let adds = 0;
const race = mountPicker({
  bindings: [],
  onRequestAdd: () => (++adds === 1 ? first.promise : second.promise),
});
chooseB(race);
race.render({ open: false });
race.render({ open: true });
race.render();
chooseB(race);
assert.equal(adds, 2);
first.resolve();
await flush();
race.render();
chooseB(race);
assert.equal(adds, 2, 'a late previous-session completion cannot unlock the new request');
assert.equal(groupView(race).props.disabled, true);
second.resolve();
await flush();
race.render();
assert.equal(groupView(race).props.disabled, false);

let focus = 0;
const anchor = { isConnected: true, focus: () => focus++ };
const focusPicker = mountPicker({ anchorElement: anchor });
focusPicker.find('DialogContent').props.onCloseAutoFocus(fakeEvent());
assert.equal(focus, 1);
groupView(focusPicker).props.onSelect(groupView(focusPicker).props.group.createRow);
focusPicker.find('DialogContent').props.onCloseAutoFocus(fakeEvent());
assert.equal(focus, 1, 'setup handoff does not restore the old trigger');
await flush();
const confirmFocus = mountPicker();
groupView(confirmFocus).props.onSelect(groupView(confirmFocus).props.group.accounts[1], anchor);
await flush();
confirmFocus.render();
confirm(confirmFocus).props.onCancel();
confirmFocus.render();
confirm(confirmFocus).props.onCloseAutoFocus(fakeEvent());
assert.equal(
  focus,
  2,
  'cancel/confirm returns to the count button rather than a removed account row'
);

const workflow = mountPicker({
  groupByConnector: false,
  target: { type: 'workflow', id: 'wf', binding_instance_id: 'node' },
});
assert.equal(
  workflow.all('ConnectorPickerGroupRow').length,
  0,
  'Workflow node selection remains unchanged'
);
assert.ok(workflow.all('ConnectorPickerRow').length > 0);
loading = true;
const loadState = mountPicker();
assert.ok(loadState.all('Skeleton').length > 0);
assert.equal(loadState.all('ConnectorPickerGroupRow').length, 0);
loading = false;
failed = true;
loadState.render();
assert.equal(loadState.all('ConnectorPickerGroupRow').length, 0);
assert.match(text(loadState.find('DialogBody')), /picker.loadFailedTitle/);
failed = false;

// Current identity and default identity remain independent, even with broken credentials.
const invalidCurrent = groupModel.createConnectorPickerMenuGroups(catalog, connections, [
  { ...binding, connection_id: 'old' },
])[0];
const summary = groupModel.connectorPickerGroupSummary(invalidCurrent, {
  mode: 'selected',
  connection_id: 'a',
  available: true,
});
assert.equal(summary.name, 'Account old');
assert.equal(summary.availability, 'reconnect_required');
assert.equal(summary.isDefault, false);
assert.equal(
  groupModel.connectorPickerGroupSummary({
    ...invalidCurrent,
    currentBinding: { ...binding, connection_id: 'hidden' },
  }).availability,
  'unavailable'
);
groupRow.render({
  group: invalidCurrent,
  preference: { mode: 'selected', connection_id: 'a', available: true },
  preferencesReady: true,
  onDefaultChange: (connection, selected) => preferenceWrites.push({ connection, selected }),
});
assert.match(text(groupRow.find('DropdownMenuTrigger')), /Account old/);
assert.match(text(groupRow.find('DropdownMenuTrigger')), /status.reconnectRequired/);
assert.equal(groupRow.find('DropdownMenuRadioGroup').props.value, 'old');
const selectedBeforeDefault = selected.length;
assert.equal(groupRow.all('DropdownMenuSub').length, 0);
assert.equal(groupRow.all('Star').length, 1, 'default is a read-only account icon');
assert.equal(selected.length, selectedBeforeDefault);
assert.deepEqual(preferenceWrites, []);

const disabledGroup = groupModel.createConnectorPickerMenuGroups(catalog, connections, [
  { ...binding, status: 'disabled' },
])[0];
groupRow.render({ group: disabledGroup });
assert.match(text(groupRow.find('DropdownMenuTrigger')), /Account a/);
assert.match(text(groupRow.find('DropdownMenuTrigger')), /usage.status.disabled/);
assert.doesNotMatch(text(groupRow.find('DropdownMenuTrigger')), /picker.group.usedInTarget/);
assert.equal(groupRow.find('DropdownMenuRadioGroup').props.value, '');
const missingGroup = groupModel.createConnectorPickerMenuGroups(
  [],
  [],
  [{ ...binding, connector_id: 'missing-private-id', connection_id: 'hidden-key-id' }]
)[0];
assert.equal(
  groupModel.filterConnectorPickerGroups([missingGroup], { query: '', status: 'all' }).length,
  1
);
groupRow.render({ group: missingGroup });
assert.match(text(groupRow.find('DropdownMenuTrigger')), /picker.group.currentMissing/);
assert.doesNotMatch(text(groupRow.tree), /missing-private-id|hidden-key-id/);
const detachedBeforeOrphan = detached;
groupRow
  .all('DropdownMenuItem')
  .find(node => text(node).includes('picker.group.removeFromTarget'))
  .props.onSelect();
assert.equal(detached, detachedBeforeOrphan + 1, 'missing connector remains visible and removable');
let orphanDetached;
await groupModel.requestConnectorPickerMenuToggle(
  missingGroup,
  {
    onRequestDetach: request => {
      orphanDetached = request;
    },
  },
  target
);
assert.equal(orphanDetached.binding.connection_id, 'hidden-key-id');

preferences = { revision: 0, defaults: {} };
let firstUseAdds = 0;
const firstUse = mountPicker({ bindings: [], onRequestAdd: () => firstUseAdds++ });
groupView(firstUse).props.onSelect(groupView(firstUse).props.group.accounts[0]);
await flush();
firstUse.render();
assert.equal(firstUseAdds, 1, 'first selection is immediate, with no default confirmation');
assert.equal(firstUse.find('ConnectorFirstUseDialog'), undefined);
assert.deepEqual(preferenceWrites, [], 'picker never writes a default');
preferenceFailure = true;
const failedBinding = mountPicker({ bindings: [], onRequestAdd: () => { throw new Error('binding conflict'); } });
groupView(failedBinding).props.onSelect(groupView(failedBinding).props.group.accounts[0]);
await flush();
failedBinding.render();
assert.match(text(failedBinding.find('DialogBody')), /picker.actionFailed/);
assert.deepEqual(preferenceWrites, []);
preferenceFailure = false;

const catalogCreateRequests = [];
const catalogRow = load('src/components/connectors/catalog/connector-catalog-row.tsx', {
  ...common,
  'next/link': { default: 'Link' },
  './catalog-model': catalogModel,
  react: { useState: initial => [initial, value => catalogCreateRequests.push(value)] },
  '../setup/connector-create-dialog': ui,
}).ConnectorCatalogRow;
const cards = [
  github,
  { ...feishu, description: '', connection_summary: { count: 0 } },
  {
    ...github,
    name: 'A very long connector name '.repeat(8),
    description: 'Long description '.repeat(100),
    connection_summary: { count: 2, state: 'reconnect_required' },
  },
  { ...exa, connection_summary: { count: 1, state: 'unavailable' } },
].map(item => catalogRow({ item }));
const layoutClasses = tree =>
  nodes(tree)
    .filter(node => ['article', 'h3'].includes(node.type) ||
      (node.type === 'div' && node.props.className === 'flex h-9 w-36 shrink-0'))
    .map(node => node.props.className);
for (const card of cards) {
  assert.deepEqual(
    layoutClasses(card),
    layoutClasses(cards[0]),
    'connected/unconnected/error states keep identical card/title/button geometry'
  );
  assert.ok(
    nodes(card).some(node => node.type === 'p' && /h-10 line-clamp-2/.test(node.props.className)),
    'every description reserves exactly two lines'
  );
  assert.ok(
    nodes(card).some(node => node.type === 'div' && /mt-auto flex h-9/.test(node.props.className)),
    'status and action remain in the same reserved footer'
  );
}
for (const count of [1, 2]) {
  const card = catalogRow({ item: { ...github, connection_summary: { count, state: 'ready' } } });
  const cardNodes = nodes(card);
  const mainAction = cardNodes.find(node => node.type === 'Button');
  assert.equal(text(mainAction), 'catalog.manageConnections');
  assert.equal(nodes(mainAction).find(node => node.type === 'Link').props.href, '/console/connectors/github');
  assert.equal(cardNodes.filter(node => node.type === 'DropdownMenuTrigger').length, 1);
  const menuItems = cardNodes.filter(node => node.type === 'DropdownMenuItem');
  assert.equal(menuItems.length, 1, 'the dropdown only offers add; manage remains the primary action');
  const beforeAdd = catalogCreateRequests.length;
  menuItems.find(node => text(node).includes('connections.addConnection')).props.onSelect();
  assert.equal(catalogCreateRequests.length, beforeAdd + 1);
  assert.equal(catalogCreateRequests.at(-1), true, 'add opens the existing independent create dialog');
}
const newCard = catalogRow({ item: { ...github, connection_summary: { count: 0 } } });
assert.equal(nodes(newCard).filter(node => node.type === 'DropdownMenuTrigger').length, 0);
assert.match(text(nodes(newCard).find(node => node.type === 'Button')), /actions.connect/);
assert.match(
  read('src/components/connectors/catalog/connector-catalog-section.tsx'),
  /grid auto-rows-fr gap-4 lg:grid-cols-2/
);
for (const locale of ['en-US', 'zh-Hans']) {
  const messages = load(`src/i18n/modules/connectors/${locale}.ts`).default;
  assert.ok(messages.catalog.connectedCount.includes('{count}'));
  assert.ok(messages.picker.group.chooseForTarget.includes('{target}'));
  assert.ok(messages.picker.group.connectionsLabel.includes('{connector}'));
}
console.log(
  'Connector dialog grouping, account actions, switch races/focus, and uniform catalog-card layout checks passed.'
);

pendingRecords=[{kind:'pending_setup',id:'pending-original',connector_id:'feishu',app_id:'cli_original',region:'feishu',state:'awaiting_application',revision:2,allowed_actions:['resume_manual','cancel']}];
let resumedPending;
groupRow.render({group:groups[2],disabled:false,pendingSetups:pendingRecords,onResume:id=>{resumedPending=id;}});
assert.equal(groupRow.all('DropdownMenuRadioItem').length,0,'pending is never a selectable connection');
assert.match(text(groupRow.find('DropdownMenuTrigger')),/pending.continue/);
groupRow.all('DropdownMenuItem').find(node=>text(node).includes('cli_original')).props.onSelect();
assert.equal(resumedPending,'pending-original');
assert.ok(groupRow.all('DropdownMenuItem').some(node=>text(node).includes('picker.actions.addConnection')),'new connection remains available');
const pendingCard=catalogRow({item:{...feishu,connection_summary:{count:0}}});
assert.deepEqual(layoutClasses(pendingCard),layoutClasses(cards[0]));
assert.match(text(nodes(pendingCard).find(node=>node.type==='Button')),/pending.continue/);
let latePendingAdds=0;
const pendingPicker=mountPicker({onRequestAdd:()=>latePendingAdds++,onRequestSwitch:()=>latePendingAdds++});
pendingPicker.all('ConnectorPickerGroupRow').find(node=>node.props.group.id==='feishu').props.onResume('pending-original');
pendingPicker.render();
assert.equal(pendingPicker.all('Dialog').length,1,'resume replaces content of original dialog');
const resumedPanel=pendingPicker.find('ConnectorKeySetupPanel');
assert.equal(resumedPanel.props.pendingSetupID,'pending-original');
assert.equal(resumedPanel.props.target,undefined,'pending does not bind a target during credential creation');
pendingPicker.render({open:false});
resumedPanel.props.onReady(connection('new','ready','feishu'));
await flush();assert.equal(latePendingAdds,0,'closed or stale picker cannot bind a late result');
pendingPicker.unmount();pendingRecords=[];
console.log('Pending menu actions, same-dialog recovery, card geometry and stale-host fencing checks passed.');
