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
  generatedImages,
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
    ...(generatedImages ? { generatedImages } : {}),
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

const stalePendingLocal = message({
  messageId: 'message-terminal',
  workflowRunId: 'run-terminal',
  tempKey: 'live-terminal',
  query: 'terminal turn',
  answer: 'partial local answer',
  status: 'pending_approval',
  phase: 'streaming',
  nodes: [{ nodeId: 'local-node', status: 'paused' }],
});
const completedTerminal = message({
  messageId: 'message-terminal',
  workflowRunId: 'run-terminal',
  query: 'terminal turn',
  answer: 'authoritative persisted answer',
  status: 'completed',
});
const terminalResult = reconcileConversationMessages([completedTerminal], [stalePendingLocal]);
assert.equal(terminalResult[0].WorkflowRunInfo.status, 'completed');
assert.equal(terminalResult[0].clientState.status, 'completed');
assert.equal(terminalResult[0].clientState.phase, 'completed');
assert.equal(terminalResult[0].answer, 'authoritative persisted answer');
assert.equal(terminalResult[0].messageData.tempKey, 'live-terminal');
assert.equal(terminalResult[0].WorkflowRunInfo.runNodeInfo[0].nodeId, 'local-node');
for (const terminalStatus of ['stopped', 'error', 'expired']) {
  const persistedTerminal = message({
    messageId: 'message-terminal',
    workflowRunId: 'run-terminal',
    answer: `${terminalStatus} persisted answer`,
    status: terminalStatus,
  });
  const result = reconcileConversationMessages([persistedTerminal], [stalePendingLocal]);
  assert.equal(result[0].WorkflowRunInfo.status, terminalStatus);
  assert.equal(result[0].clientState.status, terminalStatus);
  assert.equal(result[0].answer, `${terminalStatus} persisted answer`);
}

for (const terminalStatus of ['completed', 'stopped', 'error', 'expired']) {
  const stalePersisted = message({
    messageId: 'message-local-terminal',
    workflowRunId: 'run-local-terminal',
    answer: 'stale persisted partial answer',
    status: 'running',
    phase: 'streaming',
  });
  const localTerminal = message({
    messageId: 'message-local-terminal',
    workflowRunId: 'run-local-terminal',
    tempKey: 'local-terminal-key',
    answer: `${terminalStatus} local final answer`,
    status: terminalStatus,
    nodes: [{ nodeId: 'local-terminal-node', status: 'success' }],
  });
  const result = reconcileConversationMessages([stalePersisted], [localTerminal]);
  assert.equal(result[0].WorkflowRunInfo.status, terminalStatus);
  assert.equal(result[0].clientState.status, terminalStatus);
  assert.equal(result[0].clientState.phase, 'completed');
  assert.equal(result[0].answer, `${terminalStatus} local final answer`);
  assert.equal(result[0].messageData.tempKey, 'local-terminal-key');
  assert.equal(result[0].WorkflowRunInfo.runNodeInfo[0].nodeId, 'local-terminal-node');
}

const stoppedPersistedWithoutImages = message({
  messageId: 'message-stopped-images',
  workflowRunId: 'run-stopped-images',
  answer: '',
  status: 'stopped',
});
const stoppedLocalWithImageSkeleton = message({
  messageId: 'message-stopped-images',
  workflowRunId: 'run-stopped-images',
  answer: '',
  status: 'stopped',
  generatedImages: [{ url: '', alt: 'pending image', isLoading: true }],
});
const stoppedImageResult = reconcileConversationMessages(
  [stoppedPersistedWithoutImages],
  [stoppedLocalWithImageSkeleton]
);
assert.deepEqual(
  stoppedImageResult[0].generatedImages ?? [],
  [],
  'terminal reconciliation must discard image loading placeholders'
);

const stoppedLocalWithCompletedImage = message({
  messageId: 'message-stopped-completed-image',
  workflowRunId: 'run-stopped-completed-image',
  status: 'stopped',
  generatedImages: [
    { url: '', alt: 'pending image', isLoading: true },
    { url: '/generated/result.png', alt: 'completed image', isLoading: false },
  ],
});
const stoppedCompletedImageResult = reconcileConversationMessages(
  [
    message({
      messageId: 'message-stopped-completed-image',
      workflowRunId: 'run-stopped-completed-image',
      status: 'stopped',
    }),
  ],
  [stoppedLocalWithCompletedImage]
);
assert.deepEqual(stoppedCompletedImageResult[0].generatedImages, [
  { url: '/generated/result.png', alt: 'completed image', isLoading: false },
]);

const thirdPersisted = message({
  messageId: 'message-3',
  workflowRunId: 'run-3',
  query: 'third turn',
});
const middleLocal = message({
  workflowRunId: 'run-2',
  tempKey: 'live-middle',
  query: 'second pending turn',
  status: 'pending_approval',
  phase: 'streaming',
});
const orderedResult = reconcileConversationMessages(
  [firstPersisted, thirdPersisted],
  [firstPersisted, middleLocal, thirdPersisted]
);
assert.deepEqual(
  orderedResult.map(item => item.query),
  [firstPersisted.query, 'second pending turn', 'third turn'],
  'an unprojected local message must remain between its persisted neighbors'
);

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
assert.match(
  controllerSource,
  /private detailRequestSequence = 0/,
  'conversation detail loading must track the latest request independently of the active id'
);
assert.match(
  controllerSource,
  /requestSequence !== this\.detailRequestSequence \|\|[\s\S]*?activeId !== id/,
  'an obsolete conversation selection must discard its result'
);
assert.equal(
  (
    controllerSource.match(
      /if \(requestSequence === this\.detailRequestSequence\) \{\s*this\.store\.getState\(\)\.setIsLoadingDetail\(false\);\s*\}/g
    ) ?? []
  ).length,
  2,
  'only the latest select or loadAndSelect request may clear the shared detail loading state'
);

console.log('conversation detail reconciliation regression checks passed');
