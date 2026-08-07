import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';

import { reconcileConversationMessages } from '../src/components/chat/utils/conversation-message-reconcile.ts';

function message({
  messageId = '',
  workflowRunId = '',
  tempKey = '',
  query = '',
  answer = '',
  status = 'completed',
  phase = 'completed',
  nodes = [],
} = {}) {
  return {
    messageId,
    query,
    answer,
    parentId: '',
    model: null,
    clientState: { phase, ...(status === 'running' ? {} : { status }) },
    WorkflowRunInfo: workflowRunId ? { id: workflowRunId, status, runNodeInfo: nodes } : undefined,
    messageData: {
      ...(tempKey ? { tempKey } : {}),
      ...(messageId ? { message_id: messageId } : {}),
      ...(workflowRunId ? { workflow_run_id: workflowRunId } : {}),
    },
  };
}

const firstPersisted = message({
  messageId: 'message-1',
  workflowRunId: 'run-1',
  query: '第一轮',
  answer: '已完成',
});
const pausedLocal = message({
  workflowRunId: 'run-paused',
  tempKey: 'live-paused',
  query: '需要暂停',
  answer: '暂停前的正文',
  status: 'pending_approval',
  phase: 'streaming',
  nodes: [{ nodeId: 'pause-node', status: 'paused' }],
});
const followingLocal = message({
  tempKey: 'live-following',
  query: '暂停后继续发送',
  status: 'running',
  phase: 'streaming',
});

const staleSnapshotResult = reconcileConversationMessages(
  [firstPersisted],
  [firstPersisted, pausedLocal, followingLocal]
);
assert.equal(staleSnapshotResult.length, 3);
assert.equal(staleSnapshotResult[1].query, '需要暂停');
assert.equal(staleSnapshotResult[2].query, '暂停后继续发送');

const pausedProjection = message({
  messageId: 'message-paused',
  workflowRunId: 'run-paused',
  query: '需要暂停',
  status: 'pending_approval',
});
const reconciledPause = reconcileConversationMessages([pausedProjection], [pausedLocal]);
assert.equal(
  reconciledPause.length,
  1,
  'a persisted pause projection must deduplicate the live row'
);
assert.equal(reconciledPause[0].messageId, 'message-paused');
assert.equal(reconciledPause[0].answer, '暂停前的正文');
assert.equal(reconciledPause[0].WorkflowRunInfo.runNodeInfo[0].nodeId, 'pause-node');
assert.equal(reconciledPause[0].messageData.tempKey, 'live-paused');

const completedLocal = message({
  messageId: 'message-completed',
  workflowRunId: 'run-completed',
  answer: '页面上的旧结果',
});
const completedPersisted = message({
  messageId: 'message-completed',
  workflowRunId: 'run-completed',
  answer: '数据库中的最终结果',
});
const reconciledCompleted = reconcileConversationMessages([completedPersisted], [completedLocal]);
assert.equal(reconciledCompleted[0].answer, '数据库中的最终结果');

const controllerSource = await readFile(
  new URL('../src/components/chat/controllers/single-chat-controller.ts', import.meta.url),
  'utf8'
);
assert.match(
  controllerSource,
  /staleTime:\s*0/,
  'manual conversation selection must not reuse a fresh-looking stale detail snapshot'
);
assert.match(
  controllerSource,
  /refetchType:\s*'none'/,
  'stream lifecycle changes should invalidate snapshots without refreshing the active view'
);
assert.match(
  controllerSource,
  /onPaused:[\s\S]*?invalidateConversationDetail\(currentId\)/,
  'a paused workflow must invalidate its cached conversation detail'
);

console.log('conversation detail reconciliation regression checks passed');
