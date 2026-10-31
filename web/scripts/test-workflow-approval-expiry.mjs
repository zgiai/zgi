import assert from 'node:assert/strict';
import { loadTsModule } from './lib/load-ts-module.mjs';

// An expired workflow approval is one event: the dispatcher applies it once,
// and the run and its approval node expire in the same update, also when the
// event carries a durable sequence.

const reducers = await loadTsModule('components/chat/controllers/aichat/reducers/index.ts');
const store = await loadTsModule('components/chat/controllers/aichat/store.ts');
const { dispatchAIChatStreamEvent } = await loadTsModule(
  'components/chat/transports/aichat-transport.ts'
);

let state = reducers.applyMessageStartState(
  store.createAIChatInitialState(),
  { conversation_id: 'c1', message_id: 'm1', created_at: 1 },
  {},
  '1000-0'
);
const appliers = {
  onWorkflowPaused: reducers.applyWorkflowPausedState,
  onWorkflowNodeFinished: reducers.applyWorkflowNodeFinishedState,
  onWorkflowApprovalRequested: reducers.applyWorkflowApprovalRequestedState,
};
let applied = 0;
const callbacks = new Proxy(
  {},
  {
    get: (_, name) =>
      appliers[name]
        ? (payload, eventId) => {
            applied += 1;
            state = appliers[name](state, payload, eventId);
          }
        : () => {},
  }
);
const run = { conversation_id: 'c1', message_id: 'm1', workflow_run_id: 'run-1' };
const approvalNode = { node_id: 'approval-1', node_type: 'approval', title: 'Approve' };

dispatchAIChatStreamEvent(
  'workflow_paused',
  { ...run, ...approvalNode, sequence: 4, status: 'paused' },
  '1001-0',
  callbacks
);
const workflowRun = () =>
  state.streamingByMessageId.m1.timeline.find(item => item.type === 'workflow_run');
assert.equal(workflowRun().status, 'pending_approval');
assert.equal(workflowRun().nodes.find(node => node.nodeId === 'approval-1')?.status, 'paused');

applied = 0;
dispatchAIChatStreamEvent(
  'approval_expired',
  { ...run, ...approvalNode, sequence: 5 },
  '1002-0',
  callbacks
);
assert.equal(applied, 1, 'the expiry is one transition');
assert.equal(workflowRun().status, 'expired');
assert.equal(
  workflowRun().nodes.find(node => node.nodeId === 'approval-1')?.status,
  'failed',
  'the expired approval node is no longer shown as paused'
);

console.log('workflow approval expiry: one update, run and node expired');
