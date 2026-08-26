import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const agentChat = read('src/components/chat/variants/aichat/aichat-chat.tsx');
const chatController = read('src/components/chat/chat-with-controller.tsx');
const webappRun = read('src/components/webapp/run/index.tsx');
const workflowMonitor = read('src/components/chat/ui/workflow-run-monitor/index.tsx');
const workflowNodes = read('src/components/workflow/ui/workflow-run-nodes-list/index.tsx');

assert.match(
  agentChat,
  /showWorkflowFailureDetails=\{surface !== 'agent-webapp'\}/,
  'published Agent apps should hide workflow failure details while draft debugging keeps them'
);
assert.match(
  chatController,
  /showWorkflowFailureDetails=\{surface !== 'webapp'\}/,
  'published conversational workflow webapps should hide workflow failure details'
);
assert.match(
  webappRun,
  /<ExecutionTab[\s\S]*?showFailureDetails=\{false\}/,
  'published task workflow webapps should hide node failure details'
);
assert.match(
  workflowMonitor,
  /showFailureDetails && status === 'error' && error/,
  'workflow-level failure text should not mount when failure details are hidden'
);
assert.match(
  workflowNodes,
  /!isCanvasVariant && showFailureDetails && raw\.error/,
  'node-level failure cards should not mount when failure details are hidden'
);
assert.match(
  workflowNodes,
  /<WorkflowRunNodesList[\s\S]*?showFailureDetails=\{showFailureDetails\}[\s\S]*?variant="panel"/,
  'nested loop and iteration nodes should inherit the failure-detail policy'
);
assert.match(
  workflowMonitor,
  /showFailureDetails = true/,
  'debug and other internal workflow viewers should keep failure details by default'
);

console.log('Workflow failure detail visibility regression checks passed.');
