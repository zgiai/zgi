import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const chatShell = fs.readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/aichat-chat.tsx'),
  'utf8'
);
const messageList = fs.readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/message-list.tsx'),
  'utf8'
);
const messageBubble = fs.readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/message-bubble.tsx'),
  'utf8'
);

assert.match(
  chatShell,
  /showContextualOperationStatus=\{effectiveRuntimeSurface === 'contextual_sidebar'\}/,
  'the chat shell should enable page operation status only for the contextual sidebar'
);
assert.match(
  messageList,
  /showContextualOperationStatus=\{showContextualOperationStatus\}/,
  'the message list should forward the contextual operation-status policy'
);
assert.match(
  messageBubble,
  /showContextualOperationStatus = false/,
  'the shared message bubble should hide contextual operation status by default'
);
assert.match(
  messageBubble,
  /showContextualOperationStatus && \(isStreaming \|\| isWaitingForClientAction\)/,
  'page operation status should require the contextual-sidebar policy'
);

console.log('Contextual operation status surface regression checks passed.');
