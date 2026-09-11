import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const read = file => readFileSync(path.join(process.cwd(), file), 'utf8');
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
      assert.ok(name in imports, `${file}: unexpected import ${name}`);
      return imports[name];
    },
    module,
    module.exports
  );
  return module.exports;
}

function hooks() {
  let cursor = 0;
  const values = [];
  return {
    render(fn) {
      cursor = 0;
      return fn();
    },
    react: {
      useState(initial) {
        const i = cursor++;
        if (!(i in values)) values[i] = typeof initial === 'function' ? initial() : initial;
        return [
          values[i],
          value => {
            values[i] = typeof value === 'function' ? value(values[i]) : value;
          },
        ];
      },
      useRef(initial) {
        return (values[cursor++] ??= { current: initial });
      },
      useCallback(fn) {
        return fn;
      },
      useMemo(fn) {
        return fn();
      },
    },
  };
}
const model = load('src/components/chat/variants/aichat/connector-draft-model.ts');
const initial = model.newChatConnectorDraft('owner/org/workspace', null);
const github = { connector_id: 'github', connection_id: 'a' };
const exa = { connector_id: 'exa', connection_id: 'b' };
let draft = { ...initial, bindings: [github, exa] };
assert.deepEqual(
  model.selectChatDraftConnection(draft, {
    ...github,
    connection_id: 'c',
    secret: 'must-not-copy',
  }),
  [exa, { ...github, connection_id: 'c' }]
);
draft = model.reconcileChatConnectorDraft(draft, draft.scope, 'draft-aichat-first');
assert.equal(draft.id, initial.id);
assert.deepEqual(draft.bindings, [github, exa]);
assert.equal(model.reconcileChatConnectorDraft(draft, draft.scope, draft.conversationId), draft);
for (const scope of [
  'other-owner/org/workspace',
  'owner/other-org/workspace',
  'owner/org/other-workspace',
]) {
  assert.deepEqual(
    model.reconcileChatConnectorDraft(draft, scope, draft.conversationId).bindings,
    []
  );
}
for (const id of [null, 'persisted-chat']) {
  assert.deepEqual(model.reconcileChatConnectorDraft(draft, draft.scope, id).bindings, []);
}

const hookHarness = hooks();
const { useChatConnectorDraft } = load(
  'src/components/chat/variants/aichat/use-chat-connector-draft.ts',
  {
    react: hookHarness.react,
    './connector-draft-model': model,
  }
);
const render = (scope = 'scope', id = null) =>
  hookHarness.render(() => useChatConnectorDraft(scope, id));
let value = render();
value.update([{ ...github, credential: 'not-stored' }]);
value = render();
assert.deepEqual(value.draft.bindings, [github]);
const firstID = value.draft.id;
value = render('scope', 'draft-aichat-1');
assert.equal(value.draft.id, firstID);
assert.deepEqual(value.draft.bindings, [github]);
const late = value;
value.reset();
assert.equal(
  late.isCurrent(),
  false,
  'reset invalidates pending beforeSend and setup callbacks immediately'
);
late.update([exa]);
value = render('scope', 'draft-aichat-1');
assert.deepEqual(value.draft.bindings, []);
value.update([github]);
value = render('scope', 'draft-aichat-1');
const previousScope = value;
value = render('different-scope', 'draft-aichat-1');
assert.equal(previousScope.isCurrent(), false);
previousScope.update([exa]);
value = render('different-scope', 'draft-aichat-1');
assert.deepEqual(value.draft.bindings, []);

const ui = new Proxy({}, { get: (_, name) => name });
const element = (type, props) => ({ type, props: props ?? {} });
const nodes = tree =>
  Array.isArray(tree)
    ? tree.flatMap(nodes)
    : tree && typeof tree === 'object'
      ? [tree, ...nodes(tree.props?.children)]
      : [];
const dialogHarness = hooks();
const changes = [],
  routes = [],
  notices = [];
const setupPreferenceWrites = [];
let setupPreferenceFails = false;
const { AIChatConnectorDraftDialog } = load(
  'src/components/chat/variants/aichat/connector-draft-dialog.tsx',
  {
    react: dialogHarness.react,
    'react/jsx-runtime': { jsx: element, jsxs: element },
    'next/navigation': { useRouter: () => ({ push: url => routes.push(url) }) },
    'lucide-react': ui,
    sonner: { toast: { success: message => notices.push(message) } },
    '@/components/connectors/picker': ui,
    '@/components/connectors/picker/connector-first-use-dialog': ui,
    '@/components/ui/confirm-dialog': ui,
    '@/components/connectors/setup/connector-create-dialog': ui,
    '@/components/connectors/connector-metadata': { localizeConnectorCatalogItem: value => value },
    '@/components/ui/button': ui,
    '@/components/ui/dialog': ui,
    '@/hooks/use-locale': { useLocale: () => ({ locale: 'zh-Hans' }) },
    '@/hooks/connectors/use-chat-connector-preferences': {
      useChatConnectorPreferences: () => ({
        data: { data: { revision: 0, defaults: {} } },
        scope: 'scope',
        isError: false,
      }),
      useSetChatConnectorPreference: () => ({
        isPending: false,
        mutateAsync: async input => {
          setupPreferenceWrites.push(input);
          if (setupPreferenceFails) throw new Error('default conflict');
          return { data: { revision: 1, defaults: {} } };
        },
      }),
    },
    '@/i18n': { useT: () => key => key },
    './connector-draft-model': model,
  }
);
let props = {
  draft: initial,
  open: true,
  anchorElement: null,
  onOpenChange() {},
  onChange: values => changes.push(values),
};
const renderDialog = patch =>
  dialogHarness.render(() => AIChatConnectorDraftDialog((props = { ...props, ...patch })));
let tree = renderDialog();
assert.equal(tree.type, 'ConnectorPickerDialog');
assert.equal(tree.props.draft, true);
assert.equal(tree.props.groupByConnector, true);
tree.props.onRequestAdd({ connection: { id: 'a', connector_id: 'github', token: 'not-copied' } });
assert.deepEqual(changes.pop(), [github]);
tree = renderDialog({ draft: { ...initial, bindings: [github] } });
assert.throws(
  () => tree.props.onRequestAdd({ connection: { id: 'c', connector_id: 'github' } }),
  /confirmation/
);
tree.props.onRequestSwitch({ connection: { id: 'c', connector_id: 'github' } });
assert.deepEqual(changes.pop(), [{ ...github, connection_id: 'c' }]);
tree.props.onRequestDetach({ binding: tree.props.bindings[0] });
assert.deepEqual(changes.pop(), []);
tree.props.onRequestConnectAndAdd({ connector: { id: 'github', name: 'GitHub' } });
tree = renderDialog();
const setup = nodes(tree).find(node => node.type === 'ConnectorCreateContent');
assert.equal(
  setup.props.target,
  undefined,
  'key setup must not bind a fake draft target on the server'
);
setup.props.onReady({ id: 'c', connector_id: 'github' });
assert.equal(changes.length, 0, 'saving another key cannot replace a selected account');
assert.equal(notices.length, 1);
tree = renderDialog();
tree.props.onRequestConnectAndAdd({ connector: { id: 'exa', name: 'Exa' } });
tree = renderDialog();
const lateReady = nodes(tree).find(node => node.type === 'ConnectorCreateContent').props.onReady;
renderDialog({ open: false });
renderDialog({ open: true });
lateReady({ id: 'b', connector_id: 'exa' });
assert.equal(changes.length, 0, 'closing picker invalidates an unfinished setup attachment');

tree = renderDialog({ draft: { ...initial, id: 'first-use-draft' } });
tree = renderDialog();
tree.props.onRequestConnectAndAdd({ connector: { id: 'exa', name: 'Exa' } });
tree = renderDialog();
nodes(tree)
  .find(node => node.type === 'ConnectorCreateContent')
  .props.onReady({ id: 'b', connector_id: 'exa', name: 'My search account' });
tree = renderDialog();
assert.equal(tree.type, 'ConnectorPickerDialog');
assert.deepEqual(changes.pop(), [exa], 'new connection fills only the empty draft slot');
assert.deepEqual(setupPreferenceWrites, [], 'host selection does not mutate defaults');
// Preference saving belongs to the optional creation field, not to draft/target attachment.

// Exercise the actual send hook and HTTP request object; no server or provider is used.
const requests = [],
  streamErrors = [];
let failRequest = true;
const stateRef = {
  current: {
    activeConversationId: null,
    conversations: [],
    messagesByConversation: {},
    streamingByMessageId: {},
    recoveringByConversation: {},
    stoppingByConversation: {},
    connectionByConversation: {},
    isSending: false,
    error: null,
  },
};
const setState = fn => {
  stateRef.current = fn(stateRef.current);
};
const replace = (items, item) => [...items.filter(existing => existing.id !== item.id), item];
let draftSequence = 0;
const { useChatRuntimeMessageActions } = load(
  'src/components/chat/runtime/controller/use-chat-runtime-message-actions.ts',
  {
    react: { useCallback: fn => fn },
    '@/components/chat/controllers/aichat/selectors': {
      getNextActiveSendingState: (state, id, sending) =>
        state.activeConversationId === id ? sending : state.isSending,
    },
    '@/components/chat/controllers/aichat/state-reducers': {
      mergeAIChatMessages: (a, b) => [...a, ...b],
    },
    '@/components/chat/utils/aichat-message': {
      isDraftAIChatConversationId: id => id?.startsWith('draft-'),
      isPersistedAIChatRuntimeId: id => Boolean(id && !id.startsWith('draft-')),
      createDraftAIChatConversation: (id, title) => ({ id, title }),
      createStreamingAIChatMessage: ({ id, conversationId, query }) => ({
        id,
        conversation_id: conversationId,
        query,
      }),
      replaceAIChatConversation: replace,
      upsertAIChatMessage: replace,
    },
    '@/components/chat/utils/aichat-parameters': { toAIChatParameters: values => values },
    '@/components/chat/utils/message-tree': { buildCurrentChatPath: (_, messages) => messages },
    '@/components/chat/runtime/controller/chat-runtime-controller-utils': {
      createClientDraftId: prefix => `${prefix}-${++draftSequence}`,
      getErrorMessage: error => error.message,
      isAbortError: () => false,
      isRecoverableStreamTransportError: () => false,
      messageFilesForReplay: () => [],
    },
    './use-chat-runtime-message-actions/continuation': {
      useWorkflowContinuationActions: () => ({}),
    },
  }
);
const noop = () => {};
const events = new Proxy(
  {
    applyStreamError: payload => {
      streamErrors.push(payload);
      setState(state => ({
        ...state,
        isSending: false,
        error: payload.message,
        conversations: state.conversations.map(chat =>
          chat.id === payload.conversation_id ? { ...chat, runtime_status: 'idle' } : chat
        ),
      }));
    },
  },
  { get: (values, key) => values[key] ?? noop }
);
const actions = useChatRuntimeMessageActions({
  stateRef,
  transportRef: {
    current: {
      streamChat: async (request, handlers) => {
        requests.push(request);
        if (failRequest) {
          const error = new Error('connection unavailable');
          handlers.onRequestError(error);
          throw error;
        }
      },
    },
  },
  requireModel: true,
  pendingStreamAbortRef: { current: null },
  streamAbortByConversationRef: { current: {} },
  streamingMessageRef: { current: null },
  setControllerState: setState,
  markSelectionTarget: () => 1,
  isLatestSelection: () => true,
  eventAppliers: events,
});
const input = {
  query: 'search',
  model: { model: 'test', provider: 'test' },
  initialConnectorBindings: [{ ...github, key: 'never-send' }],
  initialConnectorDefaultsRevision: 0,
};
await actions.send(input);
assert.equal(requests.length, 1);
assert.deepEqual(requests[0].initial_connector_bindings, [github]);
assert.equal(requests[0].initial_connector_defaults_revision, 0);
assert.equal(requests[0].conversation_id, undefined);
assert.equal(requests[0].parent_id, undefined);
assert.ok(streamErrors.every(error => error.message_id.startsWith('draft-')));
assert.equal(
  stateRef.current.conversations[0].runtime_status,
  'idle',
  'first-request failure must unlock the draft'
);
failRequest = false;
const failedMessage =
  stateRef.current.messagesByConversation[stateRef.current.activeConversationId][0];
await actions.regenerate(failedMessage.id, input.model, {
  initialConnectorBindings: input.initialConnectorBindings,
  initialConnectorDefaultsRevision: 2,
});
assert.equal(requests.length, 2, 'retry after failed initialization reaches transport');
assert.deepEqual(requests[1].initial_connector_bindings, [github]);
assert.equal(requests[1].initial_connector_defaults_revision, 2, 'retry uses reviewed revision');
assert.equal(
  requests[1].conversation_id,
  undefined,
  'local conversation ID is never sent as a persisted ID'
);
assert.equal(
  requests[1].parent_id,
  undefined,
  'failed local message is never sent as a server parent'
);
stateRef.current = {
  ...stateRef.current,
  isSending: false,
  activeConversationId: 'saved',
  conversations: [{ id: 'saved', runtime_status: 'idle' }],
  messagesByConversation: { saved: [{ id: 'saved-message' }] },
};
await actions.send(input);
assert.equal(requests.length, 3);
assert.equal(requests[2].conversation_id, 'saved');
assert.equal(requests[2].parent_id, 'saved-message');
assert.equal(
  'initial_connector_bindings' in requests[2],
  false,
  'existing chat cannot silently override a binding'
);
assert.equal('initial_connector_defaults_revision' in requests[2], false);
stateRef.current = {
  ...stateRef.current,
  isSending: false,
  activeConversationId: null,
};
await actions.send({ ...input, initialConnectorBindings: [], initialConnectorDefaultsRevision: 0 });
assert.deepEqual(requests[3].initial_connector_bindings, [], 'explicit empty is transmitted');
assert.equal(requests[3].initial_connector_defaults_revision, 0, 'empty still validates snapshot');
stateRef.current = { ...stateRef.current, isSending: false, activeConversationId: null };
await actions.send({ query: 'legacy client', model: input.model });
assert.equal(
  'initial_connector_bindings' in requests[4],
  false,
  'old no-field request stays omitted'
);
assert.equal('initial_connector_defaults_revision' in requests[4], false);

const picker = read('src/components/connectors/picker/connector-picker-dialog.tsx');
assert.match(picker, /draft \? \{\} : \{ target_type: target.type, target_id: target.id \}/);
assert.match(picker, /enabled=\{open && !draft\}/);
const chat = read('src/components/chat/variants/aichat/aichat-chat.tsx');
assert.match(chat, /connectorSnapshot && !connectorSnapshot\.isCurrent\(\)/);
assert.doesNotMatch(chat, /initialConnectorDefaultsRevision:/);
console.log(
  'AIChat draft selection, scope isolation, explicit account switching and first-send/retry HTTP checks passed.'
);
