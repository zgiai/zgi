import assert from 'node:assert/strict';

import {
  WorkflowConversationRuntimeController,
  WorkflowRunEventSession,
  wrapWorkflowRunCallbacksWithSession,
} from '../src/hooks/workflow/workflow-runtime-controller.ts';
import {
  resolveConversationRouteSync,
  shouldStartNewConversationForRoute,
} from '../src/components/chat/runtime/conversation-route-handoff.ts';
import { classifyWorkflowRuntimeError } from '../src/utils/workflow/runtime-error.ts';
import {
  isStaleWorkflowRuntimeEvent,
  normalizeWorkflowRuntimeEvent,
} from '../src/utils/workflow/runtime-event-envelope.js';

const first = new WorkflowRunEventSession('run-a', 'conversation-a');
const second = new WorkflowRunEventSession('run-b', 'conversation-b');

first.dispatch({ type: 'connection', state: 'connected' });
second.dispatch({ type: 'connection', state: 'connected' });
first.dispatch({
  type: 'event',
  event: 'workflow_paused',
  sequence: 2,
  payload: { sequence: 2, data: { reasons: [{ type: 'question_answer' }] } },
});
assert.equal(first.getState().runtimeStatus, 'pending_question');
assert.equal(second.getState().runtimeStatus, 'running');

first.dispatch({
  type: 'event',
  event: 'workflow_resumed',
  sequence: 3,
  payload: { sequence: 3, data: {} },
});
assert.equal(first.getState().runtimeStatus, 'running');
assert.equal(first.getState().activePause, null);

first.dispatch({
  type: 'event',
  event: 'workflow_stopped',
  sequence: 4,
  payload: { sequence: 4, data: { status: 'stopped' } },
});
assert.equal(first.getState().runtimeStatus, 'idle');

const nestedEnvelope = normalizeWorkflowRuntimeEvent({
  event: 'node_finished',
  sequence: 12,
  execution_id: 'execution-1',
  data: { node_id: 'node-1', sequence: 3 },
});
assert.equal(nestedEnvelope.sequence, 12);
assert.equal(nestedEnvelope.payload.node_id, 'node-1');
assert.equal(nestedEnvelope.payload.execution_id, 'execution-1');
assert.equal(isStaleWorkflowRuntimeEvent({ sequence: 12 }, 12), true);

const sequencedSession = new WorkflowRunEventSession('run-sequenced');
let nodeCallbacks = 0;
const sequencedCallbacks = wrapWorkflowRunCallbacksWithSession(sequencedSession, {
  onNodeStarted: () => {
    nodeCallbacks += 1;
  },
});
sequencedCallbacks.onNodeStarted?.({ sequence: 7, data: { node_id: 'node-1' } });
sequencedCallbacks.onNodeStarted?.({ sequence: 7, data: { node_id: 'node-1' } });
assert.equal(nodeCallbacks, 1, 'every durable workflow event must be applied once');
assert.equal(sequencedSession.getState().cursor, 7);
assert.equal(first.getState().connectionState, 'idle');

let stalePauseCallbacks = 0;
const guardedCallbacks = wrapWorkflowRunCallbacksWithSession(first, {
  onWorkflowPaused: () => {
    stalePauseCallbacks += 1;
  },
});
guardedCallbacks.onWorkflowPaused?.({ sequence: 2, data: { status: 'paused' } });
assert.equal(stalePauseCallbacks, 0, 'events behind the durable cursor must not reach UI handlers');
assert.equal(first.getState().runtimeStatus, 'idle');

const resumedSnapshot = new WorkflowRunEventSession('run-resuming');
resumedSnapshot.dispatch({
  type: 'snapshot',
  payload: {
    data: {
      workflow_run: { status: 'running' },
      active_pause: {
        pause: { status: 'resume_ready' },
        reasons: [{ type: 'approval_required', status: 'completed' }],
      },
      last_sequence: 6,
    },
  },
});
assert.equal(resumedSnapshot.getState().runtimeStatus, 'running');
assert.equal(resumedSnapshot.getState().activePause, null);

const terminalSnapshot = new WorkflowRunEventSession('run-terminal');
terminalSnapshot.dispatch({
  type: 'snapshot',
  payload: {
    data: {
      workflow_run: { status: 'succeeded' },
      active_pause: {
        pause: { status: 'paused' },
        reasons: [{ type: 'approval_required', status: 'pending' }],
      },
      last_sequence: 40,
    },
  },
});
assert.equal(terminalSnapshot.getState().runtimeStatus, 'idle');
assert.equal(terminalSnapshot.getState().activePause, null);

let stoppedRunId = '';
const conversations = new WorkflowConversationRuntimeController({
  stopRun: async workflowRunId => {
    stoppedRunId = workflowRunId;
  },
});
conversations.attachRun('conversation-a', 'run-a');
conversations.attachRun('conversation-b', 'run-b');
await conversations.stopRun('conversation-b');
assert.equal(stoppedRunId, 'run-b');
assert.equal(conversations.getConversationState('conversation-a')?.runtimeStatus, 'running');
assert.equal(conversations.getConversationState('conversation-b')?.runtimeStatus, 'stopping');

assert.equal(shouldStartNewConversationForRoute(null, 'conversation-a', false), true);

const persistedDraft = resolveConversationRouteSync({
  activeConversationId: 'conversation-created',
  currentConversationId: null,
  routeHandoff: { conversationId: null, mode: 'draft-persistence' },
  activeConversationIsDraft: false,
});
assert.deepEqual(persistedDraft.action, {
  type: 'replace',
  conversationId: 'conversation-created',
});

const missingConversation = resolveConversationRouteSync({
  activeConversationId: null,
  currentConversationId: 'conversation-missing',
  routeHandoff: undefined,
  activeConversationIsDraft: false,
});
assert.deepEqual(missingConversation.action, { type: 'clear' });

assert.equal(
  classifyWorkflowRuntimeError(
    'workflow execution failed: Post "https://api.example.com": net/http: TLS handshake timeout'
  ),
  'model_service_timeout'
);
assert.equal(
  classifyWorkflowRuntimeError('failed to invoke LLM: provider stream call failed'),
  'model_invocation_failed'
);

console.log('workflow conversation runtime controller regression checks passed');
