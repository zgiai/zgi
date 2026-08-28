import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { URL } from 'node:url';

import {
  applyRuntimeInputApplication,
  applyAuthoritativeRuntimeInputSnapshot,
  reconcileAIChatRuntimeInput,
  selectRuntimeInputQueue,
} from '../src/components/chat/utils/runtime-input-reconcile.ts';
import {
  enqueueQueuedChatTurn,
  MAX_QUEUED_CHAT_TURNS,
  removeQueuedChatTurn,
} from '../src/components/chat/utils/queued-turns.ts';

const runtimeInput = {
  id: 'input-1',
  client_input_id: 'client-input-1',
  content: '补充要求',
  status: 'applied_follow_up',
  target_message_id: 'message-1',
  consumed_message_id: 'message-2',
  sequence: 1,
  created_at: 1,
};

const message = (id, runtimeInputs, query = '') => ({
  id,
  conversation_id: 'conversation-1',
  query,
  answer: '',
  status: 'completed',
  model_name: 'test-model',
  ...(runtimeInputs === undefined ? {} : { runtime_inputs: runtimeInputs }),
  created_at: 1,
  updated_at: 1,
});

const optimistic = { ...runtimeInput, id: 'optimistic', status: 'pending' };
const duplicate = { ...runtimeInput, id: 'duplicate' };
const reconciledSubmit = reconcileAIChatRuntimeInput(
  [message('message-1', [optimistic]), message('message-2', [duplicate])],
  runtimeInput.client_input_id,
  runtimeInput
);
assert.deepEqual(reconciledSubmit[0].runtime_inputs, [runtimeInput]);
assert.deepEqual(reconciledSubmit[1].runtime_inputs, []);

const serverSnapshot = [message('message-1', []), message('message-2', [], runtimeInput.content)];
const reconciledSnapshot = applyAuthoritativeRuntimeInputSnapshot(
  [message('message-1', [optimistic]), message('message-2', [duplicate])],
  serverSnapshot
);
assert.deepEqual(reconciledSnapshot[0].runtime_inputs, []);
assert.deepEqual(reconciledSnapshot[1].runtime_inputs, []);
assert.equal(
  reconciledSnapshot.flatMap(item => item.runtime_inputs ?? []).length,
  0,
  'a consumed follow-up must move to its ordinary user message instead of remaining projected'
);

const streamingMerge = applyAuthoritativeRuntimeInputSnapshot(
  [message('message-1', [optimistic])],
  [message('message-1')]
);
assert.deepEqual(
  streamingMerge[0].runtime_inputs,
  [optimistic],
  'stream events without a runtime input snapshot must not erase optimistic state'
);

const authoritativePending = {
  ...optimistic,
  id: 'input-pending',
  optimistic: false,
};
const queue = selectRuntimeInputQueue(
  [message('message-1', [{ ...optimistic, optimistic: true }, authoritativePending])],
  'message-1'
);
assert.deepEqual(queue, [authoritativePending], 'the server item replaces optimistic UI state');
assert.deepEqual(
  selectRuntimeInputQueue([message('message-1', [authoritativePending])], 'message-2'),
  [],
  'an input targeting the previous turn leaves the queue when the follow-up starts'
);
assert.deepEqual(
  selectRuntimeInputQueue([
    message('message-1', [
      { ...authoritativePending, status: 'applied_current_run' },
      { ...authoritativePending, id: 'failed', client_input_id: 'failed', status: 'failed' },
    ]),
  ]),
  [],
  'terminal inputs are represented by the execution timeline or ordinary message, not the queue'
);

const appliedMessages = applyRuntimeInputApplication(
  [message('message-1', [authoritativePending])],
  {
    id: authoritativePending.id,
    client_input_id: authoritativePending.client_input_id,
    status: 'applied_current_run',
    target_message_id: 'message-1',
    consumed_message_id: 'message-1',
  }
);
assert.equal(appliedMessages[0].runtime_inputs[0].status, 'applied_current_run');
assert.equal(appliedMessages[0].runtime_inputs[0].optimistic, false);

const optimisticAppliedBeforeSubmitResponse = applyRuntimeInputApplication(
  [message('message-1', [{ ...optimistic, optimistic: true }])],
  {
    id: authoritativePending.id,
    client_input_id: optimistic.client_input_id,
    status: 'applied_current_run',
    target_message_id: 'message-1',
    consumed_message_id: 'message-1',
  }
);
assert.equal(
  optimisticAppliedBeforeSubmitResponse[0].runtime_inputs[0].status,
  'applied_current_run',
  'the live application event must reconcile an optimistic input by client id'
);
const lateSubmitResponse = reconcileAIChatRuntimeInput(
  optimisticAppliedBeforeSubmitResponse,
  optimistic.client_input_id,
  authoritativePending
);
assert.equal(
  lateSubmitResponse[0].runtime_inputs[0].status,
  'applied_current_run',
  'a late pending submit response must not regress a server-confirmed application event'
);
assert.equal(lateSubmitResponse[0].runtime_inputs[0].id, authoritativePending.id);

const selectors = readFileSync(
  new URL('../src/components/chat/controllers/aichat/selectors.ts', import.meta.url),
  'utf8'
);
const runningSelector = selectors.slice(
  selectors.indexOf('export function shouldTreatConversationAsRunning'),
  selectors.indexOf('export function getNextActiveSendingState')
);
assert.doesNotMatch(
  runningSelector,
  /runtime_inputs|queued_follow_up/,
  'queued supplements are not proof that a conversation is still running'
);
assert.match(
  runningSelector,
  /!conversation && hasRunningMessageState/,
  'a known idle conversation must override stale local streaming-message state'
);

const messageBubble = readFileSync(
  new URL('../src/components/chat/variants/aichat/message-bubble.tsx', import.meta.url),
  'utf8'
);
assert.doesNotMatch(
  messageBubble,
  /runtimeInputs\.map/,
  'runtime-input guidance must not render as a user message bubble'
);

const runtimeInputQueueComponent = readFileSync(
  new URL('../src/components/chat/variants/aichat/runtime-input-queue.tsx', import.meta.url),
  'utf8'
);
assert.match(runtimeInputQueueComponent, /aria-live="polite"/);
assert.match(runtimeInputQueueComponent, /queued_follow_up/);
assert.match(runtimeInputQueueComponent, /guideCurrent/);
assert.match(runtimeInputQueueComponent, /withdrawEdit/);

const queuedTurn = (id, createdAt) => ({
  id,
  content: `queued ${id}`,
  useMemory: false,
  createdAt,
});
const queuedTurns = [queuedTurn('second', 2), queuedTurn('first', 1)].reduce(
  enqueueQueuedChatTurn,
  []
);
assert.deepEqual(
  queuedTurns.map(turn => turn.id),
  ['first', 'second'],
  'queued turns are kept in submission order'
);
assert.equal(MAX_QUEUED_CHAT_TURNS, 2);
assert.deepEqual(
  enqueueQueuedChatTurn(queuedTurns, queuedTurn('third', 3)),
  queuedTurns,
  'the frontend queue accepts at most two turns'
);
assert.deepEqual(
  removeQueuedChatTurn(queuedTurns, 'first').map(turn => turn.id),
  ['second'],
  'withdrawing a queued turn removes only that turn'
);

const inputToolbar = readFileSync(
  new URL('../src/components/chat/variants/aichat/input-toolbar.tsx', import.meta.url),
  'utf8'
);
assert.match(
  inputToolbar,
  /\(canStop \?\? isSending\) && !hasInput/,
  'stop and send share one slot, with typed input taking precedence'
);
assert.equal(
  inputToolbar.match(/onClick=\{onSend\}/g)?.length,
  1,
  'the toolbar has a single send action'
);
assert.match(inputToolbar, /aria-keyshortcuts="Enter"/);
assert.match(inputToolbar, /aria-keyshortcuts="Escape"/);

const inputArea = readFileSync(
  new URL('../src/components/chat/variants/aichat/input-area.tsx', import.meta.url),
  'utf8'
);
assert.match(inputArea, /event\.key === 'Escape'/);
assert.match(inputArea, /!input\.trim\(\)/);
assert.match(inputArea, /event\.key !== 'Enter' \|\| event\.shiftKey/);
assert.match(inputArea, /shouldIgnoreCompositionEnter\(event\)/);
assert.match(inputArea, /aria-keyshortcuts="Enter Shift\+Enter Escape"/);

const aiChat = readFileSync(
  new URL('../src/components/chat/variants/aichat/aichat-chat.tsx', import.meta.url),
  'utf8'
);
assert.match(aiChat, /activeQueuedTurns/);
assert.match(aiChat, /controller\.submitRuntimeInput\(turn\.content\)/);
assert.match(aiChat, /sendOrdinaryTurn\(nextTurn\.content/);
assert.match(aiChat, /onStop=\{handleStop\}/);

console.log('chat runtime input reconciliation checks passed');
