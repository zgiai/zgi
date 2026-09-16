import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import ts from 'typescript';

const code = ts.transpileModule(readFileSync('src/components/connectors/setup/use-creation-default.tsx', 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX },
}).outputText;
let cursor = 0, slots = [], effects = [], scope = 'org/owner', revision = 5, failed = false, error = false;
const writes = [], notices = [];
const imports = {
  react: {
    useRef: initial => slots[cursor++] ??= { current: initial },
    useState(initial) { const i = cursor++; if (!(i in slots)) slots[i] = initial; return [slots[i], v => { slots[i] = v; }]; },
    useEffect(fn, deps) { const i = cursor++; if (!slots[i] || deps.some((v,j) => slots[i].deps[j] !== v)) { slots[i]?.cleanup?.(); slots[i] = { deps }; effects.push(() => { slots[i].cleanup = fn(); }); } },
  },
  'react/jsx-runtime': { jsx: (type, props) => ({type, props}), jsxs: (type, props) => ({type, props}) },
  '@/components/ui/checkbox': { Checkbox: 'Checkbox' },
  '@/hooks/connectors/use-chat-connector-preferences': {
    useChatConnectorPreferences: () => ({ scope, isError: error, data: {data: {revision}} }),
    useSetChatConnectorPreference: () => ({ mutateAsync: async request => { writes.push(request); if (failed) throw Error('conflict'); } }),
  },
  '@/i18n': { useT: () => k => k },
  sonner: { toast: {success: x => notices.push(x), error: x => notices.push(x)} },
};
const module = {exports: {}};
new Function('require','module','exports', code)(id => { assert.ok(imports[id], id); return imports[id]; }, module, module.exports);
const render = (enabled = true) => { cursor=0; const result=module.exports.useCreationDefault(enabled); effects.splice(0).forEach(fn=>fn()); return result; };
const checkbox = hook => hook.option(false).props.children[0];
const connection = { id:'saved-account', connector_id:'feishu' };
let hook = render();
assert.equal(checkbox(hook).props.checked, false, 'no implicit new-chat default');
hook.begin(); await hook.complete(connection);
assert.equal(writes.length, 0);
checkbox(hook).props.onCheckedChange(true);
hook=render(); hook.begin(); revision=6; hook=render();
await hook.complete(connection);
assert.deepEqual(writes[0], {connectorID:'feishu', input:{mode:'selected',connection_id:'saved-account',expected_revision:5}}, 'freeze intent and CAS revision when setup starts');
await hook.complete(connection); assert.equal(writes.length, 1, 'one preference write per setup');
failed=true; hook.begin(); await hook.complete(connection);
assert.equal(notices.at(-1), 'defaults.selectionSavedDefaultFailed', 'connection remains saved after preference failure');
assert.equal(writes.length, 2, 'no automatic retry');
error=true; hook=render(); hook.begin(); await hook.complete(connection); assert.equal(writes.length,2);
error=false; hook=render(); hook.begin(); const late=hook; scope='different-owner'; render(); await late.complete(connection); assert.equal(writes.length,2);
hook=render(false); assert.equal(hook.option(false), null, 'credential repair must not offer a default mutation');
console.log('Creation default opt-in, frozen CAS, independent failure and scope isolation passed.');
