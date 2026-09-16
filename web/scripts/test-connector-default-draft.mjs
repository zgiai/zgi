import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import ts from 'typescript';

const load = (path, require = () => { throw new Error('Unexpected runtime import'); }) => {
  const exports = {};
  new Function('require', 'exports', ts.transpileModule(readFileSync(path, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText)(require, exports);
  return exports;
};
const m = load('src/components/chat/variants/aichat/connector-draft-model.ts');
const a = { connector_id: 'web-search', connection_id: 'a' };
const b = { connector_id: 'web-search', connection_id: 'b' };
const preferences = (revision = 1, available = true) => ({
  revision,
  defaults: { 'web-search': { mode: 'selected', connection_id: 'a', available }, github: { mode: 'none' } },
});
const sync = (draft, data = preferences(), rest = {}) =>
  m.synchronizeChatConnectorDefaults(draft, { enabled: true, data, ...rest });
const fresh = () => m.newChatConnectorDraft('owner/org/workspace', null);
let draft = sync(fresh());
assert.deepEqual(draft.bindings, [a]);
assert.equal(sync(draft, preferences(2)), draft, 'changing defaults never changes an open draft');
assert.equal(sync(draft, undefined, { isError: true }), draft, 'background errors do not gate a captured selection');
draft = m.updateChatConnectorDraftBindings(draft, []);
assert.equal(draft.overrides['web-search'], null);
assert.deepEqual(sync(draft).bindings, [], 'removed default never reappears');
draft = m.updateChatConnectorDraftBindings(fresh(), [b]);
assert.deepEqual(sync(draft).bindings, [b], 'first response cannot overwrite manual choice');
draft = m.updateChatConnectorDraftBindings(fresh(), []);
assert.deepEqual(sync(draft).bindings, [], 'explicit empty choice seals initialization');
draft = m.sealChatConnectorDraft(sync(fresh(), undefined, { isError: true }));
assert.deepEqual(sync(draft).bindings, [], 'late success cannot attach after sending without defaults');
draft = m.sealChatConnectorDraft(m.updateChatConnectorDraftBindings(fresh(), [b]));
assert.deepEqual(sync(draft).bindings, [b], 'sealing does not discard user choices');
draft = sync(fresh(), preferences(1, false));
assert.deepEqual(draft.bindings, [a], 'unavailable identities are not silently filtered');
assert.equal(m.chatConnectorDraftAttention(draft), 'unavailable');
assert.equal(m.chatConnectorDraftAttention(m.updateChatConnectorDraftBindings(draft, [])), null);
const empty = sync(fresh(), { revision: 0, defaults: {} });
assert.equal(sync(empty, preferences()), empty, 'successfully loaded empty is still a snapshot');
for (const scope of ['other/org/workspace', 'owner/other/workspace', 'owner/org/other']) {
  const isolated = m.reconcileChatConnectorDraft(draft, scope, null);
  assert.deepEqual(isolated.bindings, []);
  assert.equal(isolated.defaultsRevision, undefined);
  assert.equal(isolated.defaultsSealed, false);
}
draft = m.reconcileChatConnectorDraft(sync(fresh()), 'owner/org/workspace', 'draft-send');
assert.deepEqual(sync(draft, preferences(3)).bindings, [a]);
draft = m.reconcileChatConnectorDraft(draft, 'owner/org/workspace', 'saved-chat');
assert.equal(sync(draft), draft, 'saved chats never inherit');
assert.deepEqual(draft.bindings, []);

// Execute hook callbacks with deterministic React state slots, without a server.
let cursor = 0;
const slots = [];
const react = {
  useState(initial) {
    const i = cursor++;
    if (!(i in slots)) slots[i] = typeof initial === 'function' ? initial() : initial;
    return [slots[i], value => { slots[i] = typeof value === 'function' ? value(slots[i]) : value; }];
  },
  useRef(initial) { return (slots[cursor++] ??= { current: initial }); },
  useMemo: fn => fn(), useCallback: fn => fn,
};
const hook = load('src/components/chat/variants/aichat/use-chat-connector-draft.ts', name => {
  if (name === 'react') return react;
  if (name === './connector-draft-model') return m;
  throw new Error('Unexpected import: ' + name);
});
const render = (scope = 'owner/org', data, isError = false) => {
  cursor = 0;
  return hook.useChatConnectorDraft(scope, null, { enabled: true, data, isError });
};
let state = render();
const captured = state.capture();
assert.deepEqual(captured.bindings, []);
state = render('owner/org', preferences());
assert.deepEqual(state.draft.bindings, []);
assert.equal(captured.isCurrent(), true, 'late initialization does not cancel an ordinary send');
state.update([b]);
assert.equal(captured.isCurrent(), false, 'explicit account edits invalidate in-flight preparation');
state = render('owner/org', preferences());
const oldScope = state;
const beforeScopeSwitch = state.capture();
state = render('other/org', { revision: 0, defaults: {} });
assert.equal(beforeScopeSwitch.isCurrent(), false);
oldScope.update([a]);
assert.equal(oldScope.capture(), undefined, 'old scope cannot freeze the new scope');
assert.deepEqual(render('other/org').draft.bindings, []);
const oldDraft = state;
state.reset();
oldDraft.update([a]);
assert.equal(oldDraft.capture(), undefined);
assert.deepEqual(render('other/org').draft.bindings, []);
console.log('Connector one-time defaults, send snapshots, races and scope isolation passed');
