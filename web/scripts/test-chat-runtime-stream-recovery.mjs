import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const sseClient = read('src/lib/http/sse-client.ts');
assert.match(sseClient, /const SSE_IDLE_TIMEOUT_MS = 45_000/);
assert.match(sseClient, /SSE stream ended before a terminal event/);
assert.match(sseClient, /controller\.signal\.aborted/);

const recovery = read('src/components/chat/runtime/controller/use-chat-runtime-stream-recovery.ts');
assert.match(recovery, /AICHAT_RECOVERY_RETRY_DELAYS/);
assert.match(recovery, /refreshConversation\(conversationId\)/);
assert.match(recovery, /refreshMessagesSilently\(conversationId\)/);
assert.match(recovery, /stillRunning \? 'disconnected' : 'idle'/);

const chat = read('src/components/chat/variants/aichat/aichat-chat.tsx');
assert.match(chat, /canStop=\{canStopPendingWorkflowInteraction \|\| activeConversationRunning\}/);
assert.match(chat, /controller\.connectionState === 'disconnected'/);
assert.match(chat, /controller\.recoverStreamingConversation/);

console.log('ChatRuntime stream recovery regression checks passed.');
