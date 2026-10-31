import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';
import ts from 'typescript';

// ADR 0050 step 1b: stream events are checked once at the transport edge, and
// each stream's events are applied once, in order.

const read = path => readFile(new URL(path, import.meta.url), 'utf8');
const statementsOf = (path, text) =>
  ts.createSourceFile(path, text, ts.ScriptTarget.Latest, true).statements;
const withoutImports = (path, text) => {
  const source = ts.createSourceFile(path, text, ts.ScriptTarget.Latest, true);
  return source.statements
    .filter(statement => !ts.isImportDeclaration(statement))
    .map(statement => statement.getText(source))
    .join('\n');
};
const load = async source =>
  import(
    `data:text/javascript;base64,${Buffer.from(
      ts.transpileModule(source, {
        compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
      }).outputText
    ).toString('base64')}`
  );

const rejected = [];
const decoderPath = '../src/components/chat/transports/stream-event-decoder.ts';
const decoder = await load(
  `const debugAIChatTimeline = (label, data) => globalThis.__rejected.push({ label, ...data });\n` +
    withoutImports(decoderPath, await read(decoderPath))
);
globalThis.__rejected = rejected;
const { decodeStreamEventPayload, isMessageScopedStreamEvent } = decoder;

// Every event recorded from the local stack decodes unchanged.
const fixtures = JSON.parse(await read('./fixtures/chat-runtime-streams.json'));
let recorded = 0;
for (const stream of fixtures.streams) {
  for (const { event, data } of stream.events) {
    const payload = decodeStreamEventPayload(event, data);
    assert.ok(payload, `${stream.name}: recorded ${event} was rejected`);
    assert.equal(payload, data, `${stream.name}: ${event} payload is passed through unchanged`);
    recorded += 1;
  }
}
assert.ok(recorded > 50, 'the fixtures hold real streams');
assert.equal(rejected.length, 0);

// Malformed events stop here instead of reaching a handler as an empty object.
for (const [event, data, reason] of [
  ['skill_call_end', null, 'no payload'],
  ['skill_call_end', [], 'payload is not an object'],
  ['message', 'chunk', 'payload is not an object'],
  ['agent_progress', { conversation_id: 'c1' }, 'missing conversation_id or message_id'],
  [
    'plan_updated',
    { conversation_id: 'c1', message_id: ' ' },
    'missing conversation_id or message_id',
  ],
  [
    'connector_confirmation_required',
    { message_id: 'm1' },
    'missing conversation_id or message_id',
  ],
]) {
  rejected.length = 0;
  assert.equal(decodeStreamEventPayload(event, data), null, `${event} ${JSON.stringify(data)}`);
  assert.deepEqual(rejected, [{ label: 'stream:rejected', event, reason }]);
}
// Events that do not need the ids still reach their lifecycle handlers.
assert.deepEqual(decodeStreamEventPayload('message_end', null), {});
assert.deepEqual(decodeStreamEventPayload('error', { message: 'x' }), { message: 'x' });
assert.deepEqual(decodeStreamEventPayload('client_action_required', { action_id: 'a' }), {
  action_id: 'a',
});

// Every event the dispatcher handles is classified, so a new event must say
// whether it belongs to one message.
const transportPath = '../src/components/chat/transports/aichat-transport.ts';
const dispatcherCases = [];
const collectCases = node => {
  if (ts.isFunctionDeclaration(node) && node.name?.text === 'dispatchAIChatStreamEvent') {
    const visit = child => {
      if (ts.isCaseClause(child) && ts.isStringLiteral(child.expression)) {
        dispatcherCases.push(child.expression.text);
      }
      ts.forEachChild(child, visit);
    };
    visit(node);
  }
};
for (const statement of statementsOf(transportPath, await read(transportPath))) {
  collectCases(statement);
}
const unscoped = new Set([
  'message_start',
  'message_end',
  'error',
  'client_action_required',
  'client_action_result',
]);
assert.ok(dispatcherCases.length > 40);
for (const event of dispatcherCases) {
  assert.ok(
    isMessageScopedStreamEvent(event) || unscoped.has(event),
    `${event} is neither message-scoped nor a known exception`
  );
}

// The event table keeps one cursor per stream.
const sharedPath = '../src/components/chat/controllers/aichat/reducers/shared.ts';
const sharedText = await read(sharedPath);
const sharedSource = ts.createSourceFile(sharedPath, sharedText, ts.ScriptTarget.Latest, true);
const cursorHelpers = sharedSource.statements
  .filter(
    statement =>
      (ts.isFunctionDeclaration(statement) &&
        [
          'redisStreamEventIdParts',
          'compareAIChatStreamEventIds',
          'isStaleAIChatStreamEvent',
        ].includes(statement.name?.text)) ||
      (ts.isInterfaceDeclaration(statement) && statement.name.text === 'RedisStreamEventIdParts')
  )
  .map(statement => statement.getText(sharedSource));
assert.equal(cursorHelpers.length, 4, 'the stream id helpers stay in reducers/shared.ts');
const tablePath = '../src/components/chat/runtime/controller/runtime-event-callbacks.ts';
const { createRuntimeEventCallbacks } = await load(
  cursorHelpers.join('\n') + '\n' + withoutImports(tablePath, await read(tablePath))
);
const applied = [];
const appliers = new Proxy(
  {},
  { get: (_target, name) => (payload, eventId) => applied.push([name, eventId ?? null]) }
);
const callbacks = createRuntimeEventCallbacks({
  appliers,
  refreshAccountMemoryAfterMemoryMutation: () => {},
  isAborted: () => false,
});
const tool = fixtures.streams.find(stream => stream.name === 'tools-1');
const replayable = tool.events.filter(
  ({ event }) => event === 'skill_call_start' || event === 'skill_call_end'
);
assert.ok(replayable.length >= 2);
const handlerFor = event => (event === 'skill_call_start' ? 'onSkillCallStart' : 'onSkillCallEnd');
for (const { event, id, data } of replayable) callbacks[handlerFor(event)](data, id);
const firstPass = applied.length;
assert.equal(firstPass, replayable.length, 'each event applies once');
for (const { event, id, data } of replayable) callbacks[handlerFor(event)](data, id);
assert.equal(applied.length, firstPass, 'a stream repeating its events applies nothing twice');
callbacks.onSkillCallEnd({}, undefined);
assert.equal(applied.length, firstPass + 1, 'an event without an id is applied');
const later = createRuntimeEventCallbacks({
  appliers,
  refreshAccountMemoryAfterMemoryMutation: () => {},
  isAborted: () => false,
});
const [first] = replayable;
later.onSkillCallStart(first.data, first.id);
assert.equal(applied.length, firstPass + 2, 'a new stream, as on recovery, has its own cursor');

console.log(`chat stream decoder: ${recorded} recorded events ok`);
