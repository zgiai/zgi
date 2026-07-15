import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

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

const messageActions = read(
  'src/components/chat/runtime/controller/use-chat-runtime-message-actions.ts'
);
assert.equal(
  messageActions.match(/surface: runtimeSurface/g)?.length,
  1,
  'only new-message chat requests may send the runtime surface; root regeneration must use the persisted conversation surface'
);

const contextualTransport = read('src/components/aichat/contextual/context-envelope.ts');
assert.equal(
  contextualTransport.match(/surface: 'contextual_sidebar'/g)?.length,
  3,
  'contextual surface hints belong only to list, search, and new-chat routing; continuation and regeneration must use the persisted surface'
);
const userInputSubmit = chat.slice(
  chat.indexOf('const handleUserInputRequestSubmit'),
  chat.indexOf('const handleRegenerate')
);
assert.doesNotMatch(
  userInputSubmit,
  /surface:/,
  'user-input continuation must not resend a client-owned surface'
);

const chatPage = read('src/app/console/work/chat/page.tsx');
assert.match(
  chatPage,
  /isRestoringConversationRoute\s*\?\s*\(\s*<ChatLoading/,
  'a persisted conversation route must stay in its loading state until the controller selects it'
);
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

const conversationActions = read(
  'src/components/chat/runtime/controller/use-chat-runtime-conversation-actions.ts'
);
const initializationBlock = conversationActions.slice(
  conversationActions.indexOf('const init = useCallback'),
  conversationActions.indexOf('const startNew = useCallback')
);
const initialRouteAdoption = initializationBlock.indexOf('activeConversationId: conversationId');
const initialListRefresh = initializationBlock.indexOf('void refreshList().then');
assert.ok(
  initialRouteAdoption >= 0 && initialRouteAdoption < initialListRefresh,
  'the controller must adopt the initial conversation route before waiting for the list request'
);
assert.match(
  initializationBlock,
  /isLatestSelection\(selectionSeq, conversationId\)/,
  'a stale initial-route load must not override a newer conversation selection'
);

const routeStateSource = read('src/components/chat/runtime/conversation-route-state.ts');
const routeStateJavaScript = ts.transpileModule(routeStateSource, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
  fileName: 'route-state.ts',
}).outputText;
const routeState = await import(
  `data:text/javascript;base64,${Buffer.from(routeStateJavaScript).toString('base64')}`
);
assert.equal(
  routeState.isConversationRouteRestoring('conversation-a', null),
  true,
  'a persisted route must suppress the new-chat home before its conversation is selected'
);
assert.equal(
  routeState.isConversationRouteRestoring('conversation-a', 'conversation-a'),
  false,
  'the conversation UI may render once the controller owns the routed conversation'
);
assert.equal(
  routeState.isConversationRouteRestoring(null, 'conversation-a'),
  false,
  'an explicit new-chat route must not be treated as conversation restoration'
);

const agentWebAppChat = read('src/components/webapp/agent-chat/index.tsx');
assert.match(agentWebAppChat, /onSelectConversation=\{handleSelectConversation\}/);
assert.match(agentWebAppChat, /onStartNewConversation=\{handleStartNewConversation\}/);
assert.match(
  agentWebAppChat,
  /if \(isRestoringConversationRoute\) \{\s*return \(\s*<div/,
  'a published Agent route must stay in a loading state until its persisted conversation is selected'
);

const routeHandoffSource = read('src/components/webapp/agent-chat/route-handoff.ts');
const routeHandoffJavaScript = ts.transpileModule(routeHandoffSource, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
  fileName: 'route-handoff.ts',
}).outputText;
const routeHandoff = await import(
  `data:text/javascript;base64,${Buffer.from(routeHandoffJavaScript).toString('base64')}`
);

const existingConversationId = 'conversation-a';
assert.equal(
  routeHandoff.shouldStartNewConversationForRoute(null, existingConversationId, false),
  true,
  'a browser history transition from a conversation route to the empty route must start new chat'
);

const clearedRouteHandoff = { conversationId: null, mode: 'new-chat' };
assert.deepEqual(
  routeHandoff.resolveConversationRouteSync({
    activeConversationId: existingConversationId,
    currentConversationId: null,
    routeHandoff: clearedRouteHandoff,
    activeConversationIsDraft: false,
  }),
  { action: { type: 'none' }, routeHandoff: clearedRouteHandoff },
  'the stale active conversation must not rewrite a URL-driven new-chat route before startNew settles'
);
assert.deepEqual(
  routeHandoff.resolveConversationRouteSync({
    activeConversationId: null,
    currentConversationId: null,
    routeHandoff: clearedRouteHandoff,
    activeConversationIsDraft: false,
  }),
  { action: { type: 'none' }, routeHandoff: undefined },
  'the URL-driven new-chat handoff must finish after the controller clears its active conversation'
);

const draftRouteHandoff = { conversationId: null, mode: 'draft-persistence' };
assert.equal(
  routeHandoff.shouldStartNewConversationForRoute(null, 'draft-conversation', true),
  false,
  'an immediately submitted draft must not be reset while the new-chat route settles'
);
assert.deepEqual(
  routeHandoff.resolveConversationRouteSync({
    activeConversationId: 'draft-conversation',
    currentConversationId: null,
    routeHandoff: draftRouteHandoff,
    activeConversationIsDraft: true,
  }),
  { action: { type: 'none' }, routeHandoff: draftRouteHandoff },
  'a draft must retain the null-route handoff until message_start assigns a server id'
);
assert.deepEqual(
  routeHandoff.resolveConversationRouteSync({
    activeConversationId: 'conversation-persisted',
    currentConversationId: null,
    routeHandoff: draftRouteHandoff,
    activeConversationIsDraft: false,
  }),
  {
    action: { type: 'replace', conversationId: 'conversation-persisted' },
    routeHandoff: undefined,
  },
  'a persisted draft must update the route after message_start'
);

console.log('ChatRuntime stream recovery regression checks passed.');
