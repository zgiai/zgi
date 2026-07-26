import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();

function compileCommonJS(source, fileName) {
  return ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
    fileName,
  }).outputText;
}

function loadSensitiveWordFilter() {
  const fileName = path.join(root, 'src/utils/sensitive-word-filter.ts');
  const automaton = {
    transitions: [
      { s: 1 },
      { e: 2 },
      { c: 3 },
      { r: 4 },
      { e: 5 },
      { t: 6 },
      {},
    ],
    failures: [0, 0, 0, 0, 0, 0, 0],
    outputs: [0, 0, 0, 0, 0, 0, 1],
    wordCount: 1,
    maxWordLength: 6,
  };
  const source = readFileSync(fileName, 'utf8').replace(
    "import { sensitiveWordAutomatonData } from '@/generated/sensitive-word-automaton';",
    `const sensitiveWordAutomatonData = ${JSON.stringify(automaton)};`
  );
  const testModule = new Module(fileName);
  testModule.filename = fileName;
  testModule.paths = Module._nodeModulePaths(path.dirname(fileName));
  testModule._compile(compileCommonJS(source, fileName), fileName);
  return testModule.exports;
}

function loadModelOutputFilter(sensitiveWordFilter) {
  const fileName = path.join(root, 'src/utils/model-output-filter.ts');
  const source = readFileSync(fileName, 'utf8');
  const testModule = new Module(fileName);
  testModule.filename = fileName;
  testModule.paths = Module._nodeModulePaths(path.dirname(fileName));
  const originalRequire = testModule.require.bind(testModule);
  testModule.require = request =>
    request === '@/utils/sensitive-word-filter'
      ? sensitiveWordFilter
      : originalRequire(request);
  testModule._compile(compileCommonJS(source, fileName), fileName);
  return testModule.exports;
}

function loadAIChatTransport(modelOutputFilter) {
  const fileName = path.join(root, 'src/components/chat/transports/aichat-transport.ts');
  const source = readFileSync(fileName, 'utf8');
  const testModule = new Module(fileName);
  testModule.filename = fileName;
  testModule.paths = Module._nodeModulePaths(path.dirname(fileName));
  const originalRequire = testModule.require.bind(testModule);
  testModule.require = request => {
    if (request === '@/services/aichat.service') {
      return { aichatService: {} };
    }
    if (request === '@/utils/model-output-filter') {
      return modelOutputFilter;
    }
    if (request === '@/components/chat/controllers/aichat') {
      return { DEFAULT_AICHAT_MESSAGE_PAGINATION: { page: 1, limit: 20 } };
    }
    return originalRequire(request);
  };
  testModule._compile(compileCommonJS(source, fileName), fileName);
  return testModule.exports;
}

process.env.NEXT_PUBLIC_SENSITIVE_WORD_FILTER_ENABLED = 'true';
const sensitiveWordFilter = loadSensitiveWordFilter();
const modelOutputFilter = loadModelOutputFilter(sensitiveWordFilter);
const {
  SENSITIVE_OUTPUT_BLOCKED_TOKEN,
  wrapModelOutputSseCallbacks,
} = modelOutputFilter;
const { sanitizeAIChatMessage } = loadAIChatTransport(modelOutputFilter);

{
  const snapshots = [];
  const deltas = [];
  const replacements = [];
  const wrapped = wrapModelOutputSseCallbacks({
    onWorkflowSnapshot: payload => snapshots.push(payload),
    onMessage: payload => deltas.push(payload),
    onTextReplace: payload => replacements.push(payload),
  });

  wrapped.onWorkflowSnapshot?.({ data: { message: { answer: 'sec' } } });
  wrapped.onMessage?.({ data: { answer_delta: 'ret' } });
  wrapped.onMessage?.({ data: { answer_delta: 'must-not-append' } });

  assert.equal(snapshots.length, 1);
  assert.equal(deltas.length, 0, 'a sensitive word split across snapshot and delta must not leak');
  assert.equal(replacements.length, 1);
  assert.equal(replacements[0].answer, SENSITIVE_OUTPUT_BLOCKED_TOKEN);
}

{
  const snapshots = [];
  const deltas = [];
  const replacements = [];
  const wrapped = wrapModelOutputSseCallbacks({
    onWorkflowSnapshot: payload => snapshots.push(payload),
    onMessage: payload => deltas.push(payload),
    onTextReplace: payload => replacements.push(payload),
  });

  wrapped.onWorkflowSnapshot?.({ data: { message: { answer: 'secret' } } });
  wrapped.onMessage?.({ data: { answer_delta: 'must-not-append' } });

  assert.equal(snapshots.length, 1);
  assert.equal(snapshots[0].data.message.answer, SENSITIVE_OUTPUT_BLOCKED_TOKEN);
  assert.equal(deltas.length, 0, 'a blocked snapshot must remain blocked for later deltas');
  assert.equal(replacements.length, 0, 'snapshot replacement is carried by the sanitized snapshot');
}

{
  const message = sanitizeAIChatMessage({
    id: 'secret-message-id',
    conversation_id: 'secret-conversation-id',
    query: 'secret user query',
    answer: 'secret model answer',
    status: 'completed',
    model_name: 'secret-model',
    created_at: 1,
    updated_at: 2,
    metadata: {
      presentation: {
        items: [
          {
            kind: 'text',
            segment_id: 'secret-segment-id',
            presentation_id: 'secret-presentation-id',
            content_phase: 'final',
            content: 'secret presentation text',
          },
          {
            kind: 'event',
            event_type: 'secret-tool-name',
            event_ref: 'secret-event-ref',
          },
        ],
      },
      skill_invocations: [
        {
          skill_id: 'secret-skill-id',
          tool_name: 'secret-tool-name',
          result: { output: 'secret skill result' },
        },
      ],
      workflow_runs: [
        {
          workflow_run_id: 'secret-workflow-run-id',
          outputs: { answer: 'secret workflow output' },
          nodes: [
            {
              node_id: 'secret-node-id',
              outputs: { answer: 'secret node output' },
            },
          ],
        },
      ],
    },
  });

  assert.equal(message.id, 'secret-message-id');
  assert.equal(message.conversation_id, 'secret-conversation-id');
  assert.equal(message.query, 'secret user query');
  assert.equal(message.model_name, 'secret-model');
  assert.equal(message.answer, SENSITIVE_OUTPUT_BLOCKED_TOKEN);
  assert.equal(message.metadata.presentation.items[0].content, SENSITIVE_OUTPUT_BLOCKED_TOKEN);
  assert.equal(message.metadata.presentation.items[0].segment_id, 'secret-segment-id');
  assert.equal(message.metadata.presentation.items[1].event_type, 'secret-tool-name');
  assert.equal(message.metadata.skill_invocations[0].skill_id, 'secret-skill-id');
  assert.equal(message.metadata.skill_invocations[0].tool_name, 'secret-tool-name');
  assert.equal(message.metadata.skill_invocations[0].result.output, SENSITIVE_OUTPUT_BLOCKED_TOKEN);
  assert.equal(message.metadata.workflow_runs[0].workflow_run_id, 'secret-workflow-run-id');
  assert.equal(message.metadata.workflow_runs[0].outputs.answer, SENSITIVE_OUTPUT_BLOCKED_TOKEN);
  assert.equal(message.metadata.workflow_runs[0].nodes[0].node_id, 'secret-node-id');
  assert.equal(
    message.metadata.workflow_runs[0].nodes[0].outputs.answer,
    SENSITIVE_OUTPUT_BLOCKED_TOKEN
  );
}

console.log('model output snapshot filter behavior passed');
