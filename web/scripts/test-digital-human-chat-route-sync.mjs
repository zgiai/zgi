import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';
import ts from 'typescript';

const read = relativePath => readFile(new URL(`../${relativePath}`, import.meta.url), 'utf8');

const chat = await read('src/components/digital-human/digital-human-chat.tsx');
const shell = await read('src/components/chat/variants/aichat/aichat-chat.tsx');
const sidebar = await read('src/components/chat/variants/common/sidebar.tsx');
const routeHandoffSource = await read('src/components/chat/runtime/conversation-route-handoff.ts');
const routeHandoffJavaScript = ts.transpileModule(routeHandoffSource, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  fileName: 'conversation-route-handoff.ts',
}).outputText;
const routeHandoff = await import(
  `data:text/javascript;base64,${Buffer.from(routeHandoffJavaScript).toString('base64')}`
);

assert.match(chat, /const conversationParam = 'conversation'/);
assert.match(chat, /resolveConversationRouteSync/);
assert.match(chat, /shouldStartNewConversationForRoute/);
assert.match(chat, /lastInitializedConversationIdRef/);
assert.match(chat, /routeHandoffRef/);
assert.match(chat, /replaceConversationParam\(null, 'draft-persistence'\)/);
assert.doesNotMatch(chat, /controller\.init\(initialConversationId\)/);
assert.doesNotMatch(chat, /controller\.select\(conversationId\)/);
assert.match(chat, /isConversationTransitioning=\{isRestoringConversationRoute\}/);
assert.match(shell, /effectiveIsLoadingMessages = isLoadingMessages \|\| isConversationTransitioning/);
assert.match(shell, /activeId=\{selectedConversationId\}/);
assert.match(
  shell,
  /blue: 'hsl\(217 91% 60%\)'[\s\S]*violet: 'hsl\(262 83% 58%\)'/,
  'chat theme overrides must remain valid CSS colors so primary buttons are visible before hover'
);
assert.match(sidebar, /<button[\s\S]*?aria-pressed=\{activeId === conv\.id\}/);
assert.match(sidebar, /group-focus-within:opacity-100/);

const selectionHandoff = { conversationId: 'conversation-b', mode: 'selection' };
assert.deepEqual(
  routeHandoff.resolveConversationRouteSync({
    activeConversationId: 'conversation-a',
    currentConversationId: 'conversation-b',
    routeHandoff: selectionHandoff,
    activeConversationIsDraft: false,
  }),
  { action: { type: 'none' }, routeHandoff: selectionHandoff },
  'the old active conversation must not rewrite a newly selected route'
);
assert.deepEqual(
  routeHandoff.resolveConversationRouteSync({
    activeConversationId: 'conversation-b',
    currentConversationId: 'conversation-b',
    routeHandoff: selectionHandoff,
    activeConversationIsDraft: false,
  }),
  { action: { type: 'none' }, routeHandoff: undefined },
  'the selection handoff must finish only after the controller owns the target conversation'
);

const newChatHandoff = { conversationId: null, mode: 'draft-persistence' };
assert.deepEqual(
  routeHandoff.resolveConversationRouteSync({
    activeConversationId: null,
    currentConversationId: null,
    routeHandoff: newChatHandoff,
    activeConversationIsDraft: false,
  }),
  { action: { type: 'none' }, routeHandoff: undefined },
  'new chat must settle with an empty route and no active persisted conversation'
);
assert.deepEqual(
  routeHandoff.resolveConversationRouteSync({
    activeConversationId: 'draft-aichat-1',
    currentConversationId: null,
    routeHandoff: newChatHandoff,
    activeConversationIsDraft: true,
  }),
  { action: { type: 'none' }, routeHandoff: newChatHandoff },
  'a first message submitted during new-chat routing must preserve its draft'
);

console.log('DigitalHuman chat route synchronization checks passed.');
