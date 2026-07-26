import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const helperPath = path.join(root, 'src/components/chat/controllers/aichat/user-input-response.ts');
function loadTypeScriptModule(filePath) {
  const source = readFileSync(filePath, 'utf8');
  const output = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
    fileName: filePath,
  }).outputText;
  const testModule = new Module(filePath);
  testModule.filename = filePath;
  testModule.paths = Module._nodeModulePaths(path.dirname(filePath));
  testModule._compile(output, filePath);
  return testModule.exports;
}

const { buildOptimisticUserInputResponse, upsertUserInputResponse } =
  loadTypeScriptModule(helperPath);
const {
  captureAnswerTimelineBoundary,
  clearAnswerTimelineBoundaryWithoutDurableTimeline,
  finalPresentationAnswer,
  mergePresentationItems,
  optimisticUserInputResponsePresentationPosition,
  orderedPresentationTimeline,
  processTextPresentationItem,
  splitAnswerAroundTimeline,
  upsertPresentationItem,
  withPresentationItems,
  withOptimisticUserInputResponsePresentation,
} = loadTypeScriptModule(
  path.join(root, 'src/components/chat/controllers/aichat/presentation-order.ts')
);
const response = buildOptimisticUserInputResponse(
  {
    request_id: 'request-1',
    message: '  Confirm these details  ',
    questions: [
      { id: 'theme', question: ' Theme? ' },
      { question: ' Length? ' },
      { id: 'optional', question: ' Optional? ' },
    ],
  },
  'request-1',
  {
    theme: ' Nature ',
    q2: ' Short ',
  },
  123
);

assert.deepEqual(response, {
  request_id: 'request-1',
  message: 'Confirm these details',
  status: 'answered',
  answers: [
    { question_id: 'theme', question: 'Theme?', value: 'Nature' },
    { question_id: 'q2', question: 'Length?', value: 'Short' },
  ],
  answer_count: 2,
  answered_at: 123,
  optimistic: true,
});

assert.equal(buildOptimisticUserInputResponse(undefined, 'request-1', { theme: 'Nature' }), null);

const { optimistic: _optimistic, ...responseFields } = response;
const authoritative = { ...responseFields, answered_at: 456 };
assert.deepEqual(
  upsertUserInputResponse(
    [{ request_id: 'request-0', answers: [], answered_at: 100 }, response],
    authoritative
  ),
  [{ request_id: 'request-0', answers: [], answered_at: 100 }, authoritative]
);

const { mergeMessageMetadata } = loadTypeScriptModule(
  path.join(root, 'src/components/chat/controllers/aichat/reducers/shared.ts')
);
const mergedMetadata = mergeMessageMetadata(
  {
    user_input_request: {
      request_id: 'request-1',
      questions: [{ id: 'theme', question: 'Theme?' }],
    },
    user_input_responses: [response],
  },
  {
    user_input_request: {
      request_id: 'request-2',
      questions: [{ id: 'scene', question: 'Scene?' }],
    },
    user_input_responses: [authoritative],
  }
);
assert.equal(mergedMetadata.user_input_responses.length, 1);
assert.equal(mergedMetadata.user_input_responses[0].answered_at, 456);
assert.equal(mergedMetadata.user_input_request.request_id, 'request-2');
assert.equal(
  mergeMessageMetadata(
    { answer_before_timeline_length: 12 },
    { user_input_request: mergedMetadata.user_input_request }
  ).answer_before_timeline_length,
  12
);

const rejectedMetadata = mergeMessageMetadata(
  { user_input_responses: [response] },
  {
    user_input_request: {
      request_id: 'request-1',
      questions: [{ id: 'theme', question: 'Theme?' }],
    },
    user_input_responses: [],
  }
);
assert.equal(rejectedMetadata.user_input_responses.length, 0);
assert.equal(rejectedMetadata.user_input_request.request_id, 'request-1');

const answeredRequestMetadata = mergeMessageMetadata(
  {
    user_input_request: {
      request_id: 'request-1',
      questions: [{ id: 'theme', question: 'Theme?' }],
    },
  },
  { user_input_responses: [authoritative] }
);
assert.equal(answeredRequestMetadata.user_input_request, undefined);

const firstBoundary = captureAnswerTimelineBoundary(undefined, '先说明，再调用工具。');
assert.equal(firstBoundary, '先说明，再调用工具。'.length);
assert.equal(
  captureAnswerTimelineBoundary(firstBoundary, '先说明，再调用工具。后续回答'),
  firstBoundary
);
assert.deepEqual(splitAnswerAroundTimeline('先说明，再调用工具。\n\n后续回答', firstBoundary), {
  leadingAnswer: '先说明，再调用工具。',
  trailingAnswer: '\n\n后续回答',
});
assert.deepEqual(splitAnswerAroundTimeline('完整回答', undefined), {
  leadingAnswer: '',
  trailingAnswer: '完整回答',
});

const selectorsSource = readFileSync(
  path.join(root, 'src/components/chat/controllers/aichat/selectors.ts'),
  'utf8'
);
const timelineSource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/agentic-timeline.tsx'),
  'utf8'
);
const messageBubbleSource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/message-bubble.tsx'),
  'utf8'
);
assert.match(selectorsSource, /metadata\?\.user_input_responses/);
assert.match(selectorsSource, /metadata\?\.user_input_request/);
assert.match(selectorsSource, /type: 'user_input_request'/);
assert.match(selectorsSource, /type: 'user_input_response'/);
assert.match(timelineSource, /function UserInputRequestTimelineRow/);
assert.match(timelineSource, /function UserInputResponseTimelineRow/);
assert.match(messageBubbleSource, /item\.type === 'user_input_request'/);
assert.match(messageBubbleSource, /item\.type === 'user_input_response'/);
assert.match(messageBubbleSource, /answerBeforeTimelineLength/);
assert.match(messageBubbleSource, /orderedPresentationTimeline\(/);
assert.match(messageBubbleSource, /!isPresentationV2 && hasLeadingAnswer/);
assert.match(messageBubbleSource, /const showInitialPlanningStatus =/);
assert.match(messageBubbleSource, /!hasTimeline/);
assert.match(messageBubbleSource, /!resolvedPresentationItems\?\.length/);
assert.match(messageBubbleSource, /!answer/);
assert.match(messageBubbleSource, /\) : showInitialPlanningStatus \? \(/);
assert.match(messageBubbleSource, /streamingStatus\.key !== 'toolCompleted'/);
assert.match(timelineSource, /data-process-narrative/);
assert.match(timelineSource, /case 'process_text'/);

const processTextA = processTextPresentationItem(
  {
    presentation_version: 2,
    presentation_id: 'segment-a',
    presentation_sequence: 1,
    segment_id: 'segment-a',
    content_phase: 'process',
    segment_content: 'Before the tool',
    created_at_ms: 1000,
  },
  ''
);
const processTextB = processTextPresentationItem(
  {
    presentation_version: 2,
    presentation_id: 'segment-b',
    presentation_sequence: 3,
    segment_id: 'segment-b',
    content_phase: 'process',
    segment_content: 'Before the question',
    created_at_ms: 3000,
  },
  ''
);
assert.ok(processTextA);
assert.ok(processTextB);

const orderedPresentation = orderedPresentationTimeline(
  [
    processTextA,
    {
      presentation_version: 2,
      presentation_id: 'event-question',
      presentation_sequence: 2,
      kind: 'event',
      event_type: 'user_input_requested',
      event_ref: 'request-2',
      created_at_ms: 2000,
    },
    processTextB,
  ],
  [
    {
      id: 'question-request-2',
      type: 'user_input_request',
      request_id: 'request-2',
      request: {
        request_id: 'request-2',
        message: 'Need more information',
        questions: [],
      },
    },
  ]
);
assert.deepEqual(
  orderedPresentation.map(item => item.id),
  ['segment-a', 'question-request-2', 'segment-b']
);

const responsePosition = optimisticUserInputResponsePresentationPosition(
  {
    presentation_version: 2,
    presentation: {
      version: 2,
      last_sequence: 1,
      items: [
        {
          presentation_version: 2,
          presentation_id: 'event-question',
          presentation_sequence: 1,
          kind: 'event',
          event_type: 'user_input_requested',
          event_ref: 'request-2',
          created_at_ms: 1000,
        },
      ],
    },
  },
  'message-1',
  'request-2'
);
assert.deepEqual(responsePosition, {
  presentation_version: 2,
  presentation_id: 'message:message-1:event:user_input_response:request-2',
  presentation_sequence: 2,
});

const optimisticPresentation = withOptimisticUserInputResponsePresentation(
  {
    presentation_version: 2,
    presentation: {
      version: 2,
      last_sequence: 1,
      items: [
        {
          presentation_version: 2,
          presentation_id: 'event-question',
          presentation_sequence: 1,
          kind: 'event',
          event_type: 'user_input_requested',
          event_ref: 'request-2',
          created_at_ms: 1000,
        },
      ],
    },
  },
  'message-1',
  'request-2'
);
assert.equal(optimisticPresentation.position?.presentation_sequence, 2);
assert.deepEqual(
  optimisticPresentation.metadata.presentation?.items?.map(item => item.presentation_sequence),
  [1, 2]
);
assert.equal(
  optimisticPresentation.metadata.presentation?.items?.[1]?.event_ref,
  'user_input_response:request-2'
);

const stableContinuationOrder = orderedPresentationTimeline(
  [
    ...(optimisticPresentation.metadata.presentation?.items ?? []),
    {
      ...processTextB,
      presentation_sequence: 3,
    },
  ],
  [
    {
      id: 'question-request-2',
      type: 'user_input_request',
      request_id: 'request-2',
      request: {
        request_id: 'request-2',
        message: 'Need more information',
        questions: [],
      },
    },
    {
      id: 'response-request-2',
      type: 'user_input_response',
      request_id: 'request-2',
      answers: [],
      ...responsePosition,
    },
  ]
);
assert.deepEqual(
  stableContinuationOrder.map(item => item.id),
  ['question-request-2', 'response-request-2', 'segment-b']
);

const finalizedSegment = upsertPresentationItem([processTextB], {
  ...processTextB,
  content_phase: 'final',
  content: 'Final response',
});
assert.equal(finalizedSegment.length, 1);
assert.equal(finalizedSegment[0].content_phase, 'final');
assert.equal(finalizedSegment[0].content, 'Final response');
assert.equal(finalPresentationAnswer([processTextA, processTextB]), undefined);
assert.equal(finalPresentationAnswer(finalizedSegment), 'Final response');

const mergedTerminalPresentation = mergePresentationItems(
  [processTextA],
  [
    {
      presentation_version: 2,
      presentation_id: 'event-question',
      presentation_sequence: 2,
      kind: 'event',
      event_type: 'user_input_requested',
      event_ref: 'request-2',
      created_at_ms: 2000,
    },
  ]
);
assert.deepEqual(
  mergedTerminalPresentation.map(item => item.presentation_id),
  ['segment-a', 'event-question']
);
const mergedTerminalMetadata = withPresentationItems(
  { trace_id: 'trace-1' },
  mergedTerminalPresentation
);
assert.equal(mergedTerminalMetadata.presentation_version, 2);
assert.equal(mergedTerminalMetadata.presentation?.last_sequence, 2);
assert.equal(mergedTerminalMetadata.trace_id, 'trace-1');

const messageListSource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/message-list.tsx'),
  'utf8'
);
assert.match(messageListSource, /message\.metadata\?\.answer_before_timeline_length/);

const reducerSource = readFileSync(
  path.join(root, 'src/components/chat/controllers/aichat/reducers/message.ts'),
  'utf8'
);
assert.match(reducerSource, /answer_before_timeline_length: answerBeforeTimelineLength/);
assert.match(reducerSource, /clearAnswerTimelineBoundaryWithoutDurableTimeline/);
assert.match(reducerSource, /typeof payload\.answer === 'string'/);
assert.match(reducerSource, /mergePresentationItems\(/);
assert.match(reducerSource, /withPresentationItems\(/);
assert.equal(clearAnswerTimelineBoundaryWithoutDurableTimeline(0, false), undefined);
assert.equal(clearAnswerTimelineBoundaryWithoutDurableTimeline(firstBoundary, true), firstBoundary);

const skillReducerSource = readFileSync(
  path.join(root, 'src/components/chat/controllers/aichat/reducers/skill.ts'),
  'utf8'
);
assert.match(
  skillReducerSource,
  /function preserveLegacyAnswerBoundary\([\s\S]*previousStreaming\.presentationVersion === 2/
);
assert.match(skillReducerSource, /preserveLegacyAnswerBoundary\(previousStreaming\)/);

console.log('User-input response history checks passed.');
