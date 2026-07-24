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

process.env.NEXT_PUBLIC_SENSITIVE_WORD_FILTER_ENABLED = 'true';
const sensitiveWordFilter = loadSensitiveWordFilter();
const {
  SENSITIVE_OUTPUT_BLOCKED_TOKEN,
  wrapModelOutputSseCallbacks,
} = loadModelOutputFilter(sensitiveWordFilter);

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

console.log('model output snapshot filter behavior passed');
