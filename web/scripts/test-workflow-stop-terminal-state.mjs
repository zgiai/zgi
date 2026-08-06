import assert from 'node:assert/strict';

import { terminalizeWorkflowTimeline } from '../src/components/chat/controllers/aichat/workflow-terminal-state.ts';

const runningTimeline = [
  {
    id: 'transient',
    type: 'progress_text',
    content: '',
    phase: 'model_processing',
    transient: true,
  },
  {
    id: 'run-1',
    type: 'workflow_run',
    workflowRunId: 'run-1',
    status: 'running',
    nodes: [
      { status: 'success', nodeId: 'completed' },
      {
        status: 'running',
        nodeId: 'loop',
        loopRounds: [
          {
            index: 0,
            nodes: [
              { status: 'success', nodeId: 'completed-child' },
              { status: 'running', nodeId: 'running-child' },
            ],
          },
        ],
      },
    ],
  },
];

const stopped = terminalizeWorkflowTimeline(runningTimeline, 'stopped');
assert.equal(stopped.length, 1, 'transient processing state should be removed');
assert.equal(stopped[0].status, 'stopped');
assert.equal(stopped[0].nodes[0].status, 'success', 'completed nodes must remain completed');
assert.equal(stopped[0].nodes[1].status, 'stopped');
assert.equal(stopped[0].nodes[1].loopRounds[0].nodes[0].status, 'success');
assert.equal(stopped[0].nodes[1].loopRounds[0].nodes[1].status, 'stopped');

const failed = terminalizeWorkflowTimeline(runningTimeline, 'error');
assert.equal(failed[0].status, 'error');
assert.equal(failed[0].nodes[1].status, 'failed');
assert.equal(failed[0].nodes[1].loopRounds[0].nodes[1].status, 'failed');

console.log('workflow stop terminal-state regression checks passed');
