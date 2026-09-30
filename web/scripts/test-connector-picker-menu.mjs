import { rewriteConnectorPureImports, connectorPureImport } from './connector-pure-fixture.mjs';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const read = relative => readFileSync(path.join(process.cwd(), relative), 'utf8');
const transpile = relative =>
  ts.transpileModule(read(relative), {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText;
const url = source =>
  `data:text/javascript;base64,${Buffer.from(rewriteConnectorPureImports(source)).toString('base64')}`;
const metadataURL = url(transpile('src/components/connectors/connector-metadata.ts'));
const setupURL = url(transpile('src/components/connectors/setup/app-setup-state.ts'));
const pickerURL = url(
  transpile('src/components/connectors/picker/connector-picker-model.ts')
    .replace("'../setup/app-setup-state'", JSON.stringify(setupURL))
    .replace("from '../connector-metadata';", `from '${metadataURL}';`)
);
const pickerModel = await import(pickerURL);
const visibility = await import(
  url(transpile('src/components/connectors/connector-visibility.ts'))
);
assert.deepEqual(
  ['dingtalk', 'web-search', 'feishu', 'github', 'weixin', 'wecom'].filter(
    visibility.isConnectorListed
  ),
  ['dingtalk', 'web-search', 'feishu', 'github', 'weixin', 'wecom'],
  'discovery lists every connector returned by the catalog'
);
const { isConnectorPickerSwitchCurrent } = pickerModel;
const {
  createConnectorPickerMenuGroups,
  filterConnectorPickerGroups,
  requestConnectorPickerMenuAccount,
  requestConnectorPickerMenuToggle,
} = await import(
  url(
    transpile('src/components/connectors/picker/connector-picker-menu-model.ts').replace(
      "from './connector-picker-model';",
      `from '${pickerURL}';`
    )
  )
);
const menuModel = {
  createConnectorPickerMenuGroups,
  requestConnectorPickerMenuAccount,
  requestConnectorPickerMenuToggle,
};

const github = {
  id: 'github',
  plugin_id: 'zgi.github',
  name: 'GitHub',
  description: 'Repositories',
  classification: 'connector',
  key_fields: [{ name: 'token', label: 'Key', required: true, secret: true }],
  capabilities: [{ id: 'repo.list', name: 'Repositories', description: '', usable: true }],
  connection_summary: { count: 2, state: 'ready' },
};
const exa = { ...github, id: 'web-search', plugin_id: 'zgi.exa-search', name: 'Exa Web Search' };
const connection = (id, state = 'ready', connectorID = github.id) => ({
  id,
  connector_id: connectorID,
  name: `Name ${id}`,
  external_display_name: 'same-account',
  state,
  credential_configured: true,
  revision: 1,
});
const current = {
  id: 'binding-a',
  connection_id: 'a',
  connector_id: github.id,
  status: 'active',
  compatibility: 'native',
  revision: 7,
};
const target = { type: 'chat', id: 'conversation-a' };
const connections = [connection('a'), connection('b'), connection('exa-a', 'ready', exa.id)];
const groups = createConnectorPickerMenuGroups([github, exa], connections, [current], []);
const pendingOnlyGroups = createConnectorPickerMenuGroups([exa], [], []);
assert.equal(
  filterConnectorPickerGroups(
    pendingOnlyGroups,
    { query: '', status: 'needs_attention' },
    new Set([exa.id])
  ).length,
  1
);
assert.equal(
  filterConnectorPickerGroups(
    pendingOnlyGroups,
    { query: '', status: 'connected' },
    new Set([exa.id])
  ).length,
  0,
  'pending is not connected'
);
assert.equal(
  filterConnectorPickerGroups(
    pendingOnlyGroups,
    { query: 'unrelated-name', status: 'needs_attention' },
    new Set([exa.id])
  ).length,
  0,
  'pending preserves search filtering'
);
const hiddenConnector = { ...github, id: 'dingtalk' };
const hiddenConnection = connection('hidden', 'ready', hiddenConnector.id);
const hiddenBinding = {
  ...current,
  connector_id: hiddenConnector.id,
  connection_id: hiddenConnection.id,
};
const sourceGroups = createConnectorPickerMenuGroups(
  [hiddenConnector, exa],
  [hiddenConnection],
  [hiddenBinding]
);
const listedGroups = visibility.listedConnectorGroups(sourceGroups);
assert.deepEqual(
  listedGroups.map(group => group.id),
  sourceGroups.map(group => group.id)
);
assert.ok(
  listedGroups.find(group => group.id === hiddenConnector.id).createRow,
  'previously hidden connectors expose the create-connection action'
);
assert.equal(
  listedGroups.find(group => group.id === hiddenConnector.id).currentBinding,
  hiddenBinding
);
assert.ok(
  sourceGroups.find(group => group.id === hiddenConnector.id).createRow,
  'visibility does not mutate source metadata'
);
assert.deepEqual(
  visibility
    .listedConnectorGroups(
      createConnectorPickerMenuGroups([hiddenConnector, exa], [hiddenConnection], [])
    )
    .map(group => group.id),
  [hiddenConnector.id, exa.id],
  'previously hidden providers appear as new choices without an existing binding'
);
assert.ok(
  visibility
    .listedConnectorRows(
      pickerModel.createConnectorPickerRows(
        [hiddenConnector, exa],
        [hiddenConnection],
        [hiddenBinding]
      )
    )
    .some(row => row.connector.id === hiddenConnector.id && !row.binding),
  'flat pickers expose new choices for previously hidden providers'
);
assert.equal(groups.length, 2, 'one top-level row per service, not per account');
assert.equal(groups[0].accounts.length, 2);
assert.equal(groups[0].count, 2, 'new account action is not counted as an account');
assert.equal(groups[0].currentBinding.connection_id, 'a');
assert.ok(groups[0].createRow, 'always expose independent create beneath existing accounts');
assert.equal(groups[0].accounts[1].connection.name, 'Name b');
assert.equal(groups[0].accounts[1].replacedBinding.revision, 7);

const calls = [];
const handlers = {
  onRequestAdd: request => calls.push({ action: 'add', request }),
  onRequestSwitch: request => calls.push({ action: 'pending-switch', request }),
  onRequestDetach: request => calls.push({ action: 'detach', request }),
  onRequestUpdateKey: request => calls.push({ action: 'update-key', request }),
  onRequestConnectAndAdd: request => calls.push({ action: 'create', request }),
};
await requestConnectorPickerMenuAccount(groups[0].accounts[0], handlers, target);
assert.equal(calls.length, 0, 'clicking the checked account must not detach it');
await requestConnectorPickerMenuAccount(groups[0].accounts[1], handlers, target);
assert.equal(
  calls[0].action,
  'pending-switch',
  'another account only stages explicit confirmation'
);
assert.equal(
  isConnectorPickerSwitchCurrent(calls[0].request, connections, [current], target),
  true
);
assert.equal(
  isConnectorPickerSwitchCurrent(
    calls[0].request,
    connections,
    [{ ...current, revision: 8 }],
    target
  ),
  false
);
assert.equal(
  isConnectorPickerSwitchCurrent(calls[0].request, connections, [current], {
    ...target,
    id: 'other',
  }),
  false
);
assert.equal(
  isConnectorPickerSwitchCurrent(
    calls[0].request,
    [connections[0], connection('b', 'unavailable')],
    [current],
    target
  ),
  false
);

calls.length = 0;
await requestConnectorPickerMenuToggle(groups[0], handlers, target);
assert.equal(calls[0].action, 'detach', 'turning off only detaches the target binding');
assert.equal(calls[0].request.binding.id, current.id);
const unboundGroups = createConnectorPickerMenuGroups([github, exa], connections, [], []);
calls.length = 0;
assert.equal(
  unboundGroups[0].canToggle,
  false,
  'multiple ready accounts require an explicit account choice'
);
await requestConnectorPickerMenuToggle(unboundGroups[0], handlers, target);
assert.equal(calls.length, 0, 'enable never chooses the first of multiple accounts');
assert.equal(unboundGroups[1].canToggle, true);
await requestConnectorPickerMenuToggle(unboundGroups[1], handlers, target);
assert.equal(calls[0].action, 'add');
assert.equal(calls[0].request.connection.id, 'exa-a');

const invalidCatalog = {
  ...github,
  capabilities: github.capabilities.map(item => ({ ...item, usable: false })),
};
const invalid = createConnectorPickerMenuGroups(
  [invalidCatalog],
  [connection('a', 'reconnect_required')],
  [current],
  []
)[0];
calls.length = 0;
await requestConnectorPickerMenuAccount(invalid.accounts[0], handlers, target);
assert.equal(calls[0].action, 'update-key', 'an invalid account can only repair, never select');
assert.equal(
  invalid.createRow.state,
  'not_connected',
  'old invalid Keys must not disable a new Key'
);
const missing = createConnectorPickerMenuGroups([github], [connection('b')], [current], [])[0];
assert.equal(
  missing.currentBinding.connection_id,
  'a',
  'missing current connection still occupies the slot'
);
assert.equal(missing.accounts[0].replacedBinding.connection_id, 'a');

const menu = read('src/components/connectors/picker/connector-picker-menu.tsx');
const exports = read('src/components/connectors/picker/index.ts');
assert.match(menu, /from 'radix-ui\/internal'/);
assert.match(menu, /<Menu\.Anchor virtualRef=\{anchorRef\}/);
assert.match(menu, /anchorElement\?\.isConnected/);
assert.doesNotMatch(
  menu,
  /document\.activeElement|getElementById|<Dialog|SearchInput|FILTER_VALUES/
);
assert.match(menu, /<Menu\.Sub/);
assert.match(menu, /<Menu\.SubTrigger/);
assert.match(menu, /<Menu\.SubContent/);
assert.match(menu, /group\.classification \?\? 'connector'/);
assert.match(menu, /<Menu\.RadioItem/);
assert.match(menu, /<Menu\.CheckboxItem/);
assert.match(menu, /isConnectorPickerSwitchCurrent/);
assert.match(menu, /onCloseAutoFocus/);
assert.match(menu, /anchorElement\.focus/);
assert.match(menu, /anchorElement\?\.contains/);
assert.doesNotMatch(menu, /onRequestManageConnector/, 'consumer menus cannot manage other targets');
assert.match(menu, /menu\.back/);
assert.match(menu, /sessionRef\.current !== session/);
assert.match(exports, /export \{ ConnectorPickerMenu \}/);
assert.match(exports, /export type \{ ConnectorPickerMenuProps \}/);
for (const locale of ['en-US', 'zh-Hans']) {
  const messages = read(`src/i18n/modules/connectors/${locale}.ts`);
  for (const key of [
    'connectAnotherAccount',
    'manageConnector',
    'chooseAccountFirst',
    'accountCount',
    'pendingSwitch',
    'useInTarget',
  ]) {
    assert.match(messages, new RegExp(`${key}:`), `${locale}: ${key}`);
  }
}

// Execute the real component callbacks with a small hook/element harness. Radix
// remains responsible for DOM menu semantics; these tests cover our state,
// handoff and deferred-action behavior without a browser or running services.
const element = (type, props) => ({ type, props: props ?? {} });
const nodes = tree =>
  !tree || typeof tree !== 'object'
    ? []
    : Array.isArray(tree)
      ? tree.flatMap(nodes)
      : [tree, ...nodes(tree.props?.children)];
const deferred = () => {
  let resolve;
  let reject;
  const promise = new Promise((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
};
const flush = async () => {
  for (let index = 0; index < 8; index += 1) await Promise.resolve();
};
const event = (target, extras = {}) => ({
  target,
  defaultPrevented: false,
  preventDefault() {
    this.defaultPrevented = true;
  },
  ...extras,
});
class FakeNode {}
globalThis.Node = FakeNode;
globalThis.MutationObserver = class {
  observe() {}
  disconnect() {}
};
function mountMenu(overrides = {}, mobile = false) {
  const hooks = [];
  let cursor = 0;
  let effects = [];
  let tree;
  let focusCount = 0;
  let props;
  const anchor = Object.assign(new FakeNode(), {
    isConnected: true,
    contains: candidate => candidate === anchor,
    focus: () => {
      focusCount += 1;
    },
    ownerDocument: { documentElement: {} },
  });
  const React = {
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
    useRef(initial) {
      const index = cursor++;
      if (!(index in hooks)) hooks[index] = { current: initial };
      return hooks[index];
    },
    useMemo(factory) {
      cursor++;
      return factory();
    },
    useEffect(callback, dependencies) {
      const index = cursor++;
      const previous = hooks[index];
      if (
        !previous ||
        dependencies.some((value, offset) => value !== previous.dependencies[offset])
      ) {
        hooks[index] = { dependencies, cleanup: previous?.cleanup };
        effects.push(() => {
          hooks[index].cleanup?.();
          hooks[index].cleanup = callback();
        });
      }
    },
  };
  const translated = key => key;
  const query = items => ({ data: { data: { items } }, isLoading: false, isError: false });
  const imports = {
    react: React,
    'react/jsx-runtime': { jsx: element, jsxs: element, Fragment: 'Fragment' },
    'lucide-react': new Proxy({}, { get: (_, name) => name }),
    'radix-ui/internal': { Menu: new Proxy({}, { get: (_, name) => `Menu.${name}` }) },
    '@/components/connectors/connector-icon': { ConnectorIcon: 'ConnectorIcon' },
    '@/components/connectors/connector-metadata': { localizeConnectorCatalogItem: value => value },
    '@/hooks/connectors/use-connectors': {
      useConnectorCatalog: () => query([github, exa]),
      usePersonalConnections: () => query(connections),
      connectorCatalogItems: value => value?.items ?? [],
      personalConnectionItems: value => value?.items ?? [],
    },
    '@/hooks/use-locale': { useLocale: () => ({ locale: 'en-US' }) },
    '@/hooks/use-mobile': { useIsMobile: () => mobile },
    '@/i18n': { useT: () => translated },
    '@/lib/utils': { cn: (...values) => values.join(' ') },
    './connector-picker-model': pickerModel,
    './connector-picker-menu-model': menuModel,
    '../connector-visibility': visibility,
    '../unavailable-references': { ConnectorUnavailableReferences: () => null },
  };
  const compiled = ts.transpileModule(menu, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText;
  const module = { exports: {} };
  new Function('require', 'module', 'exports', compiled)(
    name => {
      if (!imports[name] && connectorPureImport(name)) return connectorPureImport(name);
      assert.ok(imports[name], `unexpected import ${name}`);
      return imports[name];
    },
    module,
    module.exports
  );
  props = {
    open: true,
    target,
    targetName: 'Chat',
    bindings: [current],
    legacyBindings: [],
    anchorElement: anchor,
    onOpenChange: () => {},
    onRequestAdd: () => {},
    onRequestSwitch: () => {},
    onRequestConnectAndAdd: () => {},
    onRequestUpdateKey: () => {},
    onRequestViewReason: () => {},
    onRequestDetach: () => {},
    onRequestDetachLegacy: () => {},
    onRequestManageLegacy: () => {},
    onRequestManageConnector: () => {},
    ...overrides,
  };
  const render = patch => {
    props = { ...props, ...patch };
    cursor = 0;
    tree = module.exports.ConnectorPickerMenu(props);
    const run = effects;
    effects = [];
    for (const effect of run) effect();
    return tree;
  };
  render();
  return {
    render,
    anchor,
    focusCount: () => focusCount,
    find: predicate => nodes(tree).find(predicate),
    all: predicate => nodes(tree).filter(predicate),
    select(predicate) {
      const item = nodes(tree).find(predicate);
      assert.ok(item, 'menu action exists');
      const selection = event();
      item.props.onSelect(selection);
      assert.equal(selection.defaultPrevented, true, 'host owns selection dismissal');
    },
  };
}
const content = harness => harness.find(item => item.type === 'Menu.Content');
const accountB = item => item.type === 'Menu.RadioItem' && item.props.value === 'b';
const textAction = key => item =>
  item.type === 'Menu.Item' &&
  nodes(item.props.children).length === 0 &&
  item.props.children === key;

// Outside dismissal must not steal the user's focus back from an input field.
const outsideFocus = mountMenu();
const outsideContent = content(outsideFocus);
outsideContent.props.onInteractOutside(event(new FakeNode()));
outsideFocus.render({ open: false });
outsideContent.props.onCloseAutoFocus(event());
assert.equal(outsideFocus.focusCount(), 0, 'outside interactions retain their new focus');
const escapeFocus = mountMenu();
const escapeContent = content(escapeFocus);
escapeFocus.render({ open: false });
escapeContent.props.onCloseAutoFocus(event());
assert.equal(escapeFocus.focusCount(), 1, 'Escape/explicit close returns to the actual trigger');
const triggerEvent = event(escapeFocus.anchor);
escapeContent.props.onInteractOutside(triggerEvent);
assert.equal(
  triggerEvent.defaultPrevented,
  true,
  'trigger pointerdown does not fight click toggle'
);
const firstEntry = mountMenu();
let firstItemFocused = 0;
content(firstEntry).props.ref.current = {
  querySelector: () => ({
    focus: () => {
      firstItemFocused++;
    },
  }),
};
const entryEvent = event();
content(firstEntry).props.onEntryFocus(entryEvent);
assert.equal(firstItemFocused, 1, 'first keyboard opening focuses an actionable service');
assert.equal(entryEvent.defaultPrevented, true);

const handedOff = mountMenu();
const handoffContent = content(handedOff);
handedOff.select(
  item =>
    item.type === 'Menu.Item' &&
    Array.isArray(item.props.children) &&
    item.props.children.includes('picker.menu.connectAnotherAccount')
);
handedOff.render({ open: false });
handoffContent.props.onCloseAutoFocus(event());
assert.equal(
  handedOff.focusCount(),
  0,
  'Key dialog handoff keeps its focus after closing the menu'
);
await flush();

// Confirm is explicit and can be cancelled; selecting or reopening a service
// does not persist anything, and a repeated open event must preserve pending.
let switched = 0;
const selection = mountMenu({
  onRequestSwitch: () => {
    switched++;
  },
});
selection.find(item => item.type === 'Menu.Sub').props.onOpenChange(true);
selection.render();
selection.select(accountB);
await flush();
selection.render();
assert.equal(switched, 0);
assert.ok(
  selection.find(
    item => item.type === 'Menu.Group' && item.props['aria-label'] === 'picker.menu.pending'
  )
);
selection.find(item => item.type === 'Menu.Sub').props.onOpenChange(true);
selection.render();
assert.ok(
  selection.find(
    item => item.type === 'Menu.Group' && item.props['aria-label'] === 'picker.menu.pending'
  ),
  'same submenu open is not a cancellation'
);
selection.select(textAction('picker.switchCancel'));
selection.render();
assert.equal(switched, 0, 'cancel leaves the binding untouched');
selection.select(accountB);
await flush();
selection.render();
selection.render({ bindings: [{ ...current, revision: 8 }] });
const confirmAction = item =>
  item.type === 'Menu.Item' &&
  Array.isArray(item.props.children) &&
  item.props.children.includes('picker.switchConfirm');
selection.select(confirmAction);
selection.render();
assert.equal(switched, 0, 'confirm rechecks the latest slot revision');
assert.equal(
  selection.find(
    item => item.type === 'Menu.Group' && item.props['aria-label'] === 'picker.menu.pending'
  ),
  undefined
);

const save = deferred();
const confirmOnce = mountMenu({
  onRequestSwitch: () => {
    switched++;
    return save.promise;
  },
});
confirmOnce.select(accountB);
await flush();
confirmOnce.render();
confirmOnce.select(confirmAction);
confirmOnce.select(confirmAction);
assert.equal(switched, 1, 'repeated Enter/click cannot submit a confirmed switch twice');
save.resolve();
await flush();
confirmOnce.render();
assert.equal(
  confirmOnce.find(
    item => item.type === 'Menu.Group' && item.props['aria-label'] === 'picker.menu.pending'
  ),
  undefined
);

const mobileMenu = mountMenu({}, true);
mobileMenu.select(item => item.type === 'Menu.Item' && item.props.textValue === 'GitHub');
mobileMenu.render();
assert.equal(mobileMenu.all(item => item.type === 'Menu.Sub').length, 0, 'mobile uses one panel');
assert.ok(mobileMenu.find(accountB));
mobileMenu.select(
  item =>
    item.type === 'Menu.Item' &&
    nodes(item).some(child => child.props?.children === 'picker.menu.back')
);
mobileMenu.render();
assert.equal(
  mobileMenu.find(accountB),
  undefined,
  'mobile back restores the service list without mutation'
);

let forcedClosed = 0;
const withoutAnchor = mountMenu({
  anchorElement: null,
  onOpenChange: value => {
    if (!value) forcedClosed++;
  },
});
assert.equal(withoutAnchor.render(), null);
assert.equal(forcedClosed, 1, 'a missing real trigger closes rather than rendering at the origin');

// A close/open round trip is a new session even when the target is unchanged.
// Resolving an old action must not clear busy state for a later in-flight one.
const first = deferred();
const second = deferred();
let adds = 0;
const race = mountMenu({
  bindings: [],
  onRequestAdd: () => (++adds === 1 ? first.promise : second.promise),
});
race.select(accountB);
race.render({ open: false });
race.render({ open: true });
race.render();
race.select(accountB);
assert.equal(adds, 2);
first.resolve();
await flush();
race.render();
race.select(accountB);
assert.equal(adds, 2, 'late old completion cannot unlock a new session');
second.resolve();
await flush();

// An old rejection must not replace the new session's pending choice with an
// error banner or clear that choice (same target, same open=true string).
const obsolete = deferred();
const rejectionRace = mountMenu({ bindings: [], onRequestAdd: () => obsolete.promise });
rejectionRace.select(accountB);
rejectionRace.render({ open: false });
rejectionRace.render({ open: true, bindings: [current] });
rejectionRace.render();
rejectionRace.select(accountB);
await flush();
rejectionRace.render();
obsolete.reject(new Error('obsolete request failed'));
await flush();
rejectionRace.render();
assert.ok(
  rejectionRace.find(
    item => item.type === 'Menu.Group' && item.props['aria-label'] === 'picker.menu.pending'
  ),
  'old rejection preserves a new pending choice'
);
assert.equal(
  rejectionRace.find(item => item.props?.role === 'alert'),
  undefined,
  'old rejection cannot paint a new session error'
);

console.log(
  'Compact Connector menu grouping, explicit selection, toggle, and revision checks passed.'
);
