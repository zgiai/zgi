import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';
import ts from 'typescript';

const presentationSource = await readFile(
  new URL('../src/components/chat/controllers/aichat/presentation-order.ts', import.meta.url),
  'utf8'
);
const presentationJavaScript = ts.transpileModule(presentationSource, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const { orderedPresentationTimeline, removePresentationSegment } = await import(
  `data:text/javascript;base64,${Buffer.from(presentationJavaScript).toString('base64')}`
);

const items = [
  {
    presentation_version: 2,
    presentation_id: 'message:1:text:1',
    presentation_sequence: 1,
    kind: 'text',
    segment_id: 'message:1:text:1',
    content_phase: 'provisional',
    content: 'Internal activation narration',
  },
  {
    presentation_version: 2,
    presentation_id: 'message:1:event:tool-1',
    presentation_sequence: 2,
    kind: 'event',
    event_type: 'skill_call_start',
  },
];

assert.deepEqual(removePresentationSegment(items, 'message:1:text:1'), [items[1]]);
assert.deepEqual(removePresentationSegment(items, undefined), items);

const governedOperation = {
  id: 'history-governance-1',
  type: 'tool_governance_decision',
  event: { correlation_id: 'operation-1' },
};
const governedOrder = orderedPresentationTimeline(
  [
    {
      presentation_version: 2,
      presentation_id: 'message:1:text:7',
      presentation_sequence: 7,
      kind: 'text',
      segment_id: 'message:1:text:7',
      content_phase: 'process',
      content: 'Preparing the document.',
    },
    {
      presentation_version: 2,
      presentation_id: 'message:1:event:tool_governance:operation-1',
      presentation_sequence: 9,
      kind: 'event',
      event_type: 'tool_governance_decision',
      event_ref: 'tool_governance:operation-1',
    },
  ],
  [governedOperation]
);
assert.deepEqual(
  governedOrder.map(item => item.id),
  ['message:1:text:7', 'history-governance-1'],
  'persisted governed operations must recover their original presentation position'
);

const reducerSource = await readFile(
  new URL('../src/components/chat/controllers/aichat/reducers/message.ts', import.meta.url),
  'utf8'
);
assert.match(reducerSource, /payload\.presentation_disposition === 'discard'/);
assert.match(reducerSource, /removePresentationSegment\(/);
assert.match(reducerSource, /modelProcessing: undefined/);
assert.match(
  reducerSource,
  /mergePresentationItems\(\s*previousStreaming\?\.presentationItems,\s*terminalPresentationState\.presentationItems/s,
  'message_end must merge the live presentation instead of replacing it with backend metadata'
);
assert.doesNotMatch(
  reducerSource,
  /hasAuthoritativeAnswer/,
  'message_end must not treat the backend answer as authoritative over the current page'
);
assert.match(
  reducerSource,
  /previousStreaming && \(nextTimeline\.length > 0 \|\| hasLivePresentation\)/,
  'message_end must keep the live event snapshot after completion'
);

const eventAppliersSource = await readFile(
  new URL(
    '../src/components/chat/runtime/controller/use-chat-runtime-event-appliers.ts',
    import.meta.url
  ),
  'utf8'
);
assert.doesNotMatch(
  eventAppliersSource,
  /refreshMessagesSilently\(payload\.conversation_id\)/,
  'normal message_end handling must not refetch and replace the displayed message'
);

const messageActionsSource = await readFile(
  new URL(
    '../src/components/chat/runtime/controller/use-chat-runtime-message-actions.ts',
    import.meta.url
  ),
  'utf8'
);
assert.match(
  messageActionsSource,
  /files: messageFilesForReplay\(source\)/,
  'branched regeneration must replay source attachments'
);

const sharedReducerSource = await readFile(
  new URL('../src/components/chat/controllers/aichat/reducers/shared.ts', import.meta.url),
  'utf8'
);
assert.match(sharedReducerSource, /delete next\.presentation;/);
assert.match(sharedReducerSource, /delete next\.presentation_version;/);

const chatShellSource = await readFile(
  new URL('../src/components/chat/variants/aichat/aichat-chat.tsx', import.meta.url),
  'utf8'
);
assert.match(
  chatShellSource,
  /files: messageFilesForReplay\(message\)/,
  'historical message editing must replay source attachments'
);

const timelineSource = await readFile(
  new URL('../src/components/chat/variants/aichat/agentic-timeline.tsx', import.meta.url),
  'utf8'
);
assert.match(timelineSource, /defaultOpen = true/);
assert.match(timelineSource, /consoleChat\.skills\.agentic\.processTitle/);

const messageBubbleSource = await readFile(
  new URL('../src/components/chat/variants/aichat/message-bubble.tsx', import.meta.url),
  'utf8'
);
assert.doesNotMatch(messageBubbleSource, /shouldPreferPersistedTimeline/);
assert.match(
  messageBubbleSource,
  /runtimeTimeline\.length > 0 \? runtimeTimeline : historicalTimeline/,
  'the current-page runtime timeline must remain visually authoritative after completion'
);
assert.match(messageBubbleSource, /const shouldOpenTimelineByDefault = isActiveMessage/);
assert.match(messageBubbleSource, /defaultOpen=\{shouldOpenTimelineByDefault\}/);

console.log('Native process commentary regression checks passed.');
