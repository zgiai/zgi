import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';
import ts from 'typescript';

// ADR 0050 step 1: one status vocabulary, and one handler for every stream event.

const statusSource = await readFile(
  new URL('../src/components/chat/controllers/aichat/status.ts', import.meta.url),
  'utf8'
);
const { invocationOutcome, governanceDecisionStatus } = await import(
  `data:text/javascript;base64,${Buffer.from(
    ts.transpileModule(statusSource, {
      compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
    }).outputText
  ).toString('base64')}`
);

for (const [status, outcome] of [
  ['running', 'running'],
  ['loading', 'running'],
  ['pending', 'running'],
  ['needs_approval', 'waiting'],
  ['waiting_client_action', 'waiting'],
  ['success', 'succeeded'],
  ['Succeeded', 'succeeded'],
  ['completed', 'succeeded'],
  ['allowed', 'succeeded'],
  ['already_loaded', 'succeeded'],
  ['error', 'failed'],
  ['denied', 'failed'],
  ['timed_out', 'failed'],
  ['cancelled', 'cancelled'],
  ['stopped', 'cancelled'],
  ['something_new', 'unknown'],
  ['', 'unknown'],
  [undefined, 'unknown'],
  [{}, 'unknown'],
]) {
  assert.equal(invocationOutcome(status), outcome, `outcome of ${JSON.stringify(status)}`);
}

// Every message and governance status the backend defines has a meaning here.
const goConstants = async (path, prefix) => {
  const text = await readFile(new URL(path, import.meta.url), 'utf8');
  const words = [
    ...text.matchAll(new RegExp(`${prefix}\\w*\\s+(?:\\w+\\s+)?=\\s+"([a-z_]+)"`, 'g')),
  ].map(match => match[1]);
  assert.ok(words.length > 0, `no ${prefix} constants found in ${path}`);
  return words;
};
for (const word of await goConstants(
  '../../api/internal/capabilities/chatruntime/model/message.go',
  'MessageStatus'
)) {
  assert.notEqual(invocationOutcome(word), 'unknown', `message status ${word} has no outcome`);
}
for (const word of await goConstants(
  '../../api/internal/capabilities/toolgovernance/types.go',
  'DecisionStatus'
)) {
  assert.equal(
    governanceDecisionStatus(word),
    word,
    `governance decision ${word} is read as itself`
  );
}

// A decision the client does not know is not read as a request for approval.
assert.equal(governanceDecisionStatus('approved_by_policy'), null);
assert.equal(governanceDecisionStatus(''), null);
assert.equal(governanceDecisionStatus(undefined), null);
assert.equal(governanceDecisionStatus('success'), 'allowed');
assert.equal(governanceDecisionStatus('failed'), 'error');
assert.equal(governanceDecisionStatus('needs_approval', 'approved'), 'allowed');
assert.equal(governanceDecisionStatus('needs_approval', 'rejected'), 'denied');

// The shared event table handles every stream callback that is not part of the
// stream lifecycle; an optional callback cannot go unhandled.
const callbackNames = async (path, collect) => {
  const text = await readFile(new URL(path, import.meta.url), 'utf8');
  const source = ts.createSourceFile(path, text, ts.ScriptTarget.Latest, true);
  const names = [];
  const visit = node => {
    collect(node, names);
    ts.forEachChild(node, visit);
  };
  visit(source);
  return new Set(names);
};
const transportCallbacks = await callbackNames(
  '../src/components/chat/transports/aichat-transport.ts',
  (node, names) => {
    if (ts.isInterfaceDeclaration(node) && node.name.text === 'AIChatStreamCallbacks') {
      for (const member of node.members) {
        if (member.name && ts.isIdentifier(member.name)) names.push(member.name.text);
      }
    }
  }
);
const tableCallbacks = await callbackNames(
  '../src/components/chat/runtime/controller/runtime-event-callbacks.ts',
  (node, names) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === 'handlers' &&
      node.initializer &&
      ts.isObjectLiteralExpression(node.initializer)
    ) {
      for (const property of node.initializer.properties) {
        if (property.name && ts.isIdentifier(property.name)) names.push(property.name.text);
      }
    }
  }
);
const lifecycle = new Set([
  'onOpen',
  'onMessageStart',
  'onMessageEnd',
  'onErrorEvent',
  'onRequestError',
  'onClose',
]);
const expected = [...transportCallbacks].filter(name => !lifecycle.has(name)).sort();
assert.deepEqual([...tableCallbacks].sort(), expected, 'the event table covers every stream event');

console.log('chat runtime status and event table: ok');
