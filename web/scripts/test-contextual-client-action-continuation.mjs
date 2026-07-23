import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';
import ts from 'typescript';

const gateSource = await readFile(
  new URL('../src/components/chat/runtime/client-action-continuation.ts', import.meta.url),
  'utf8'
);
const gateModuleSource = ts.transpileModule(gateSource, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const {
  canStartClientActionContinuation,
  openClientActionCompletionGate,
  takeClientActionCompletion,
} = await import(`data:text/javascript;base64,${Buffer.from(gateModuleSource).toString('base64')}`);

const pending = {
  continuationReady: false,
};
const routeResult = {
  status: 'succeeded',
  result: { event_type: 'route_already_loaded' },
};

assert.equal(
  takeClientActionCompletion(pending, routeResult),
  null,
  'page completion must wait until the message reaches waiting_client_action'
);
assert.equal(pending.deferredCompletion, routeResult);
assert.equal(
  openClientActionCompletionGate(pending),
  routeResult,
  'message_end should release the deferred page result exactly once'
);
assert.equal(openClientActionCompletionGate(pending), null);

assert.equal(
  canStartClientActionContinuation({
    messageStatus: 'streaming',
    streamingStatus: 'streaming',
    isSending: true,
    conversationRuntimeStatus: 'streaming',
    activeMessageMatches: true,
  }),
  false,
  'client_action_required must not resume while the original stream still owns the message'
);
assert.equal(
  canStartClientActionContinuation({
    messageStatus: 'waiting_client_action',
    streamingStatus: 'waiting_client_action',
    isSending: false,
    conversationRuntimeStatus: 'idle',
    activeMessageMatches: false,
  }),
  true,
  'the continuation should start after message_end persists the waiting state'
);

const dockSource = await readFile(
  new URL('../src/components/aichat/contextual/contextual-ai-chat-dock.tsx', import.meta.url),
  'utf8'
);
const startedGuardIndex = dockSource.indexOf('if (!started)');
const dedupeIndex = dockSource.indexOf(
  'markClientActionDedupe(processedClientActionsRef.current, pending.key)',
  startedGuardIndex
);
assert.ok(startedGuardIndex >= 0, 'the dock must handle a continuation that did not start');
assert.ok(
  dedupeIndex > startedGuardIndex,
  'the dock must not mark the action completed before confirming continuation start'
);

console.log('Contextual client action continuation regression checks passed.');
