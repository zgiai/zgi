import assert from 'node:assert/strict';
import ts from 'typescript';
import { load, read, jsx, ui, nodes, harness, flush } from './connector-fixture.mjs';

const { parseConnectorConfirmationPresentation: parse } = load(
  'src/components/connectors/runtime/connector-confirmation-presentation.ts'
);
const envelope = {
  version: 1,
  preview: {
    account_name: 'Mailbox',
    account_identity: 'sender@example.com',
    kind: 'mail',
    complete: true,
    fields: [
      {
        label: 'Recipients',
        label_i18n: { 'zh-Hans': '收件人' },
        value: 'to@example.com',
        truncated: false,
      },
    ],
  },
};
assert.equal(parse(envelope).complete, true);
for (const mutate of [
  x => {
    x.version = 2;
  },
  x => {
    x.preview.token = 'secret';
  },
  x => {
    x.preview.account_identity = 'sender\u202e@example.com';
  },
  x => {
    x.preview.fields[0].value = 'a'.repeat(8193);
  },
  x => {
    x.preview.fields[0].value = 'a\u0000b';
  },
  x => {
    x.preview.fields[0].label_i18n['unknown'] = 'Unknown';
  },
]) {
  const changed = globalThis.structuredClone(envelope);
  mutate(changed);
  assert.equal(parse(changed), null);
}
for (const mutate of [
  x => {
    x.preview.fields = [];
  },
  x => {
    x.preview.account_identity = '';
  },
]) {
  const changed = globalThis.structuredClone(envelope);
  mutate(changed);
  assert.equal(parse(changed).complete, false);
}
const longRecipients = globalThis.structuredClone(envelope);
longRecipients.preview.fields[0].value = 'a'.repeat(6000);
assert.equal(parse(longRecipients).fields[0].value.length, 6000);

const row = harness(
  react =>
    load('src/components/chat/variants/aichat/tool-invocation-row.tsx', {
      react,
      'react/jsx-runtime': jsx,
      'lucide-react': ui,
      '@/lib/utils': { cn: (...x) => x.join(' ') },
    }).ToolInvocationRow
);
const props = { tone: 'success', title: 'Action', children: 'safe detail' };
let tree = row.render(props);
let button = nodes(tree).find(x => x.type === 'button');
assert.equal(button.props['aria-expanded'], false);
assert.ok(!JSON.stringify(tree).includes('safe detail'));
button.props.onClick();
tree = row.render(props);
assert.equal(nodes(tree).find(x => x.type === 'button').props['aria-expanded'], true);
assert.ok(JSON.stringify(tree).includes('safe detail'));
assert.ok(!nodes(row.render({ ...props, children: undefined })).some(x => x.type === 'button'));

// Execute the active row function, including its dispatch and safe-detail guards.
const file = ts.createSourceFile(
  'row.tsx',
  read('src/components/chat/variants/aichat/agentic-timeline.tsx'),
  ts.ScriptTarget.Latest,
  true,
  ts.ScriptKind.TSX
);
const fn = file.statements.find(
  x => ts.isFunctionDeclaration(x) && x.name?.text === 'SkillTimelineRow'
);
const js = ts.transpileModule(fn.getText(file), {
  compilerOptions: { jsx: ts.JsxEmit.ReactJSX, module: ts.ModuleKind.CommonJS },
}).outputText;
let genericReads = 0;
const dependencies = {
  useT: () => key => key,
  useLocale: () => ({ locale: 'en-US' }),
  isConnectorOperationInvocation: invocation => invocation.kind === 'connector_operation',
  parseConnectorOperationPresentation: () => ({
    title: 'Safe action',
    display_status: 'completed',
  }),
  getDurationText: () => '1s',
  getStatusIcon: () => null,
  sanitizeDisplayString: value => {
    genericReads++;
    return value;
  },
  sanitizeTimelineResultForDisplay: value => {
    genericReads++;
    return value;
  },
  timelineDebugRows: () => {
    genericReads++;
    return [];
  },
  ToolInvocationRow: 'ToolInvocationRow',
  AIChatSkillIcon: 'SkillIcon',
  AIChatSkillResultSummary: 'ResultSummary',
};
const render = new Function(
  'require',
  'exports',
  ...Object.keys(dependencies),
  js + ';return SkillTimelineRow;'
)(() => jsx, {}, ...Object.values(dependencies));
const event = {
  tone: 'success',
  title: 'Generic title',
  detail: 'SECRET DETAIL',
  skill: {},
  item: {
    invocation: {
      kind: 'connector_operation',
      arguments: { token: 'SECRET ARGUMENT' },
      result: { password: 'SECRET RESULT' },
    },
  },
};
tree = render({ event, showDetails: true });
assert.equal(tree.type, 'ToolInvocationRow');
assert.equal(genericReads, 0, 'Connector must never enter generic debug/result rendering');
assert.equal(tree.props.title, 'Safe action · runtime.presentation.status.completed');
assert.equal(nodes(tree).find(x => x.type === 'ResultSummary').props.result, undefined);
tree = render({
  event: { ...event, item: { invocation: { kind: 'tool', result: {} } } },
  showDetails: true,
});
assert.equal(tree.type, 'ToolInvocationRow');
assert.ok(genericReads > 0, 'ordinary Skill retains its details');

let catalogEnabled;
const ready = [];
const create = load('src/components/connectors/setup/connector-create-dialog.tsx', {
  'react/jsx-runtime': jsx,
  'lucide-react': ui,
  '@/components/ui/button': ui,
  '@/components/ui/dialog': ui,
  '@/i18n': { useT: () => key => key },
  './connector-key-setup-panel': ui,
  './connector-mail-provider-panel': ui,
  './mail-setup-state': { supportsMailSetup: () => true },
  '@/hooks/connectors/use-connectors': {
    connectorCatalogItems: x => x,
    useConnectorCatalog: (_, enabled) => {
      catalogEnabled = enabled;
      return { data: { data: ['mail'] } };
    },
  },
  '../mail-display-model': {
    mailDisplayGroup: () => ({ providers: [{ id: 'gmail' }, { id: 'qqmail' }] }),
  },
}).ConnectorCreateContent;
const setup = {
  connector: { id: 'gmail', name: 'Gmail' },
  onReady: x => ready.push(x.connector_id),
};
tree = create(setup);
assert.equal(catalogEnabled, false);
let panel = nodes(tree).find(x => x.type === 'ConnectorKeySetupPanel');
panel.props.onReady({ connector_id: 'qqmail' });
assert.deepEqual(ready, []);
panel.props.onReady({ connector_id: 'gmail' });
assert.deepEqual(ready, ['gmail']);
tree = create({ ...setup, allowProviderSelection: true });
assert.equal(catalogEnabled, true);
panel = nodes(tree).find(x => x.type === 'ConnectorMailProviderPanel');
panel.props.onReady({ connector_id: 'qqmail' });
panel.props.onReady({ connector_id: 'unrelated' });
assert.deepEqual(ready, ['gmail', 'qqmail']);

console.log(
  'Connector preview validation, shared invocation/details and provider restriction passed'
);

// Ordinary approvals retain their original input-area panel and protocol.
for (const action of ['approve', 'reject', 'error']) {
  const calls = [];
  const approval = {
    id: `approval-${action}`,
    actionSentence: 'Use tool',
    assets: [],
    canSubmit: true,
    onSubmitDecision: async (...args) => {
      calls.push(args);
      if (action === 'error') throw new Error('Unavailable');
    },
  };
  const panel = harness(
    react =>
      load('src/components/chat/variants/aichat/tool-governance-decision-card.tsx', {
        react: {
          ...react,
          createContext: value => ({ value }),
          useContext: context => context.value,
        },
        'react/jsx-runtime': jsx,
        'lucide-react': ui,
        '@/components/ui/button': ui,
        '@/components/ui/checkbox': ui,
        '@/components/aichat/operation-cards': ui,
        '@/i18n/translations': { useT: () => key => key },
        '@/lib/utils': { cn: (...args) => args.join(' ') },
        '@/components/chat/variants/aichat/error-utils': {
          isAIChatContinuationLikelyStarted: () => false,
        },
      }).ToolGovernanceApprovalPanel
  );
  panel.render({ approval });
  let card = panel.render({ approval });
  assert.equal(card.type, 'div');
  nodes(card)
    .find(x => x.type === 'Checkbox')
    .props.onCheckedChange(true);
  card = panel.render({ approval });
  const buttons = nodes(card).filter(x => x.type === 'Button');
  assert.equal(buttons.length, 2);
  (action === 'reject' ? buttons[0] : buttons[1]).props.onClick();
  assert.equal(
    nodes(panel.render({ approval }))
      .filter(x => x.type === 'Button')
      .every(button => button.props.disabled === true),
    true
  );
  await flush();
  card = panel.render({ approval });
  assert.deepEqual(calls, [[action === 'reject' ? 'reject' : 'approve', action !== 'reject']]);
  if (action === 'error') {
    assert.match(JSON.stringify(card), /Unavailable/);
    assert.equal(
      nodes(card)
        .filter(x => x.type === 'Button')
        .every(button => button.props.disabled === false),
      true
    );
  } else {
    assert.equal(
      nodes(card)
        .filter(x => x.type === 'Button')
        .every(button => button.props.disabled === true),
      true
    );
  }
  panel.unmount();
}
console.log(
  'Ordinary tool approval remembers only approval, preserves rejection and recovers failed submission'
);
