import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const helperPath = path.join(
  root,
  'src/components/chat/variants/aichat/external-action-operation-status.ts'
);

const helperModule = new Module(helperPath);
helperModule.filename = helperPath;
helperModule.paths = Module._nodeModulePaths(path.dirname(helperPath));
helperModule._compile(
  ts.transpileModule(readFileSync(helperPath, 'utf8'), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
    fileName: helperPath,
  }).outputText,
  helperPath
);

const {
  getAIChatExternalActionOperationState,
  isWeComNoValidRecipientCode,
  safeExternalActionDiagnosticCode,
} = helperModule.exports;

const invocationWithStatus = operationStatus => ({
  status: 'success',
  skill_id: 'external-apps',
  tool_name: 'execute_action',
  result: { operation_status: operationStatus },
});

assert.deepEqual(getAIChatExternalActionOperationState(invocationWithStatus('completed')), {
  status: 'completed',
  tone: 'success',
});
assert.deepEqual(getAIChatExternalActionOperationState(invocationWithStatus('executing')), {
  status: 'executing',
  tone: 'running',
});
assert.deepEqual(getAIChatExternalActionOperationState(invocationWithStatus('failed_safe')), {
  status: 'failed_safe',
  tone: 'error',
});
assert.deepEqual(
  getAIChatExternalActionOperationState(invocationWithStatus('partially_succeeded')),
  {
    status: 'partially_succeeded',
    tone: 'warning',
  }
);
assert.deepEqual(getAIChatExternalActionOperationState(invocationWithStatus('outcome_unknown')), {
  status: 'outcome_unknown',
  tone: 'warning',
});
assert.equal(
  getAIChatExternalActionOperationState({
    skill_id: 'external-apps',
    tool_name: 'search_actions',
    result: { operation_status: 'failed_safe' },
  }),
  null
);
assert.equal(
  getAIChatExternalActionOperationState({
    skill_id: 'calculator',
    tool_name: 'execute_action',
    result: { operation_status: 'failed_safe' },
  }),
  null
);

assert.equal(safeExternalActionDiagnosticCode('provider_rejected'), 'provider_rejected');
assert.equal(safeExternalActionDiagnosticCode('81013'), '81013');
assert.equal(safeExternalActionDiagnosticCode('secret token=abc'), null);
assert.equal(safeExternalActionDiagnosticCode('upstream\nresponse body'), null);
assert.equal(safeExternalActionDiagnosticCode('a'.repeat(129)), null);
assert.equal(isWeComNoValidRecipientCode('wecom', '81013'), true);
assert.equal(isWeComNoValidRecipientCode('feishu', '81013'), false);

const summarySource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/skill-result-summary.tsx'),
  'utf8'
);
const timelineSource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/agentic-timeline.tsx'),
  'utf8'
);
const traceSource = readFileSync(
  path.join(root, 'src/components/chat/variants/aichat/skill-trace-panel.tsx'),
  'utf8'
);
const enMessages = readFileSync(path.join(root, 'src/i18n/modules/webapp/en-US.ts'), 'utf8');
const zhMessages = readFileSync(path.join(root, 'src/i18n/modules/webapp/zh-Hans.ts'), 'utf8');

assert.match(summarySource, /operation_status/);
for (const field of ['error_code', 'reason_code', 'provider_error_code', 'retry_safe']) {
  assert.match(summarySource, new RegExp(`result\\.${field}`));
}
assert.match(summarySource, /safeExternalActionDiagnosticCode/);
assert.match(summarySource, /providerDiagnostics\.wecom81013/);
assert.doesNotMatch(summarySource, /result\.(?:errmsg|response_body|access_token|provider_token)/i);
assert.match(summarySource, /tone === 'warning'/);
assert.match(summarySource, /tone === 'error'/);
assert.match(timelineSource, /getAIChatExternalActionOperationState\(invocation\)/);
assert.match(timelineSource, /externalActionOutcomeUnknown/);
assert.match(timelineSource, /skillId=\{event\.item\.invocation\.skill_id\}/);
assert.match(timelineSource, /toolName=\{event\.item\.invocation\.tool_name\}/);
assert.match(traceSource, /latestWarning/);
assert.match(traceSource, /externalActionPartiallySucceeded/);
assert.match(enMessages, /WeCom accepted no recipients \(81013\)/);
assert.match(zhMessages, /企业微信未接受任何接收人（81013）/);

console.log('AIChat external action business status display checks passed.');
