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

console.log('User-input response history checks passed.');
