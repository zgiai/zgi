import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const sseTypes = read('src/lib/http/types.ts');
assert.match(sseTypes, /idleTimeoutMs\?: number/);

const sseClient = read('src/lib/http/sse-client.ts');
assert.match(sseClient, /export const SSE_IDLE_TIMEOUT_MS = 45_000/);
assert.match(sseClient, /if \(idleTimeoutMs === null\) return reader\.read\(\)/);
assert.match(sseClient, /SSE stream ended before a terminal event/);
assert.match(sseClient, /controller\.signal\.aborted/);

for (const transportPath of [
  'src/services/aichat.service.ts',
  'src/components/chat/transports/agent-runtime-transport.ts',
]) {
  const transport = read(transportPath);
  assert.equal(
    transport.match(/idleTimeoutMs: SSE_IDLE_TIMEOUT_MS/g)?.length,
    6,
    `${transportPath} must opt every ChatRuntime stream into the idle watchdog`
  );
}

const recovery = read('src/components/chat/runtime/controller/use-chat-runtime-stream-recovery.ts');
assert.match(recovery, /AICHAT_RECOVERY_RETRY_DELAYS/);
assert.match(recovery, /refreshConversation\(conversationId\)/);
assert.match(recovery, /refreshMessagesSilently\(conversationId\)/);
assert.match(recovery, /stillRunning \? 'disconnected' : 'idle'/);

const chat = read('src/components/chat/variants/aichat/aichat-chat.tsx');
assert.match(chat, /canStop=\{canStopPendingWorkflowInteraction \|\| activeConversationRunning\}/);
assert.match(chat, /controller\.connectionState === 'disconnected'/);
assert.match(chat, /controller\.recoverStreamingConversation/);

const chatPage = read('src/app/console/work/chat/page.tsx');
assert.match(
  chatPage,
  /!conversationIdParam\s*&&\s*activeConversationId\s*&&\s*!isDraftAIChatConversationId\(activeConversationId\)/,
  'route synchronization must not reset a newly submitted draft conversation'
);
assert.match(
  chatPage,
  /routeSelectionTarget === null\s*&&\s*isDraftAIChatConversationId\(active\)/,
  'the null route handoff must preserve a live draft until message_start assigns its server id'
);

const agentWebAppChat = read('src/components/webapp/agent-chat/index.tsx');
assert.match(agentWebAppChat, /initController\(conversationIdParam\)/);
assert.match(agentWebAppChat, /onSelectConversation=\{handleSelectConversation\}/);
assert.match(agentWebAppChat, /onStartNewConversation=\{handleStartNewConversation\}/);
assert.match(
  agentWebAppChat,
  /isDraftAIChatConversationId\(activeConversationId\)/,
  'Agent WebApp must persist only server-backed conversation ids in its route'
);

console.log('ChatRuntime stream recovery regression checks passed.');
