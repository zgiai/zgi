import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';
import ts from 'typescript';

const source = await readFile(
  new URL(
    '../src/components/chat/variants/aichat/agent-execution-job-presentation.ts',
    import.meta.url
  ),
  'utf8'
);
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const { getAgentExecutionJobPresentation: present } = await import(
  `data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`
);
const shell = {
  kind: 'tool_call',
  skill_id: 'agent_execution',
  tool_name: 'shell_exec',
  status: 'success',
};
for (const [status, exitCode, tone, label] of [
  ['succeeded', 0, 'success', 'jobSucceeded'],
  ['failed', 1, 'error', 'jobFailed'],
  ['failed', null, 'error', 'jobFailed'],
  ['succeeded', 7, 'error', 'jobFailed'],
  ['canceled', null, 'error', 'jobFailed'],
  ['timed_out', -1, 'error', 'jobFailed'],
  ['running', null, 'running', 'jobRunning'],
  ['queued', null, 'running', 'jobRunning'],
  ['succeeded', null, 'warning', 'jobUnconfirmed'],
  ['unknown', 0, 'warning', 'jobUnconfirmed'],
  ['succeeded', '0', 'warning', 'jobUnconfirmed'],
  ['succeeded', NaN, 'warning', 'jobUnconfirmed'],
]) {
  const result = present({ ...shell, result: { job_id: 'job-1', status, exit_code: exitCode } });
  assert.equal(result.tone, tone, `${status}, exit=${exitCode}`);
  assert.equal(result.labelKey, `consoleChat.skills.agentic.${label}`);
}
for (const invocation of [
  shell,
  { ...shell, result: {} },
  { ...shell, skill_id: 'other', result: { job_id: 'job-1', status: 'failed', exit_code: 1 } },
  { ...shell, tool_name: 'workspace_export_file', result: { job_id: 'job-1', status: 'failed' } },
])
  assert.equal(present(invocation), null, 'non-Shell tools must retain their own presentation');
const transportFailure = present({
  ...shell,
  status: 'error',
  result: { job_id: 'job-1', status: 'succeeded', exit_code: 0 },
});
assert.equal(transportFailure.tone, 'error', 'a stale Job result cannot hide a transport error');
assert.equal(transportFailure.labelKey, 'consoleChat.skills.agentic.jobUnconfirmed');
assert.equal(
  present({
    ...shell,
    status: 'error',
    result: { job_id: 'job-1', status: 'failed', exit_code: -1 },
  }).labelKey,
  'consoleChat.skills.agentic.jobFailed',
  'known Job failure remains explicit when the tool also reports an error'
);
for (const component of ['agentic-timeline.tsx', 'skill-trace-panel.tsx']) {
  const text = await readFile(
    new URL(`../src/components/chat/variants/aichat/${component}`, import.meta.url),
    'utf8'
  );
  assert.match(text, /getAgentExecutionJobPresentation\(invocation\)/);
  assert.match(text, /job\.labelKey/);
}
// Run the real selector functions; unrelated imports belong to other controller
// tests and are not needed for these plain Shell timeline records.
const selectorsText = await readFile(
  new URL('../src/components/chat/controllers/aichat/selectors.ts', import.meta.url),
  'utf8'
);
const selectorsAst = ts.createSourceFile(
  'selectors.ts',
  selectorsText,
  ts.ScriptTarget.Latest,
  true
);
const selectorsSource = selectorsAst.statements
  .filter(statement => !ts.isImportDeclaration(statement))
  .map(statement => statement.getText(selectorsAst))
  .join('\n');
const selectorsCompiled = ts.transpileModule(selectorsSource, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText;
const { dedupeTimelineItems, mergeRuntimeTimelineWithMessageTimeline } = await import(
  `data:text/javascript;base64,${Buffer.from(selectorsCompiled).toString('base64')}`
);
function item(id, callID, jobID, status, exitCode, invocationStatus = 'success') {
  return {
    id,
    type: 'skill_event',
    created_at: 1,
    invocation: {
      ...shell,
      invocation_id: callID,
      status: invocationStatus,
      arguments: { command_bytes: 40 },
      result: jobID ? { job_id: jobID, status, exit_code: exitCode } : undefined,
    },
  };
}
const failed = item('failed', 'call-1', 'job-1', 'failed', 1);
const succeeded = item('success', 'call-2', 'job-2', 'succeeded', 0);
assert.equal(
  dedupeTimelineItems([failed, succeeded]).length,
  2,
  'later success must not hide failure'
);
assert.equal(
  dedupeTimelineItems([
    { ...failed, invocation: { ...failed.invocation, invocation_id: undefined } },
    { ...succeeded, invocation: { ...succeeded.invocation, invocation_id: undefined } },
  ]).length,
  2,
  'Job IDs distinguish older records without call IDs'
);
const pending = item('pending', 'call-1', '', 'running', null, 'running');
const distinctPending = item('pending-2', 'call-2', '', 'running', null, 'running');
const terminal = dedupeTimelineItems([pending, distinctPending, failed, succeeded]);
assert.equal(terminal.length, 2, 'pending updates only replace the matching call');
assert.deepEqual(
  terminal.map(event => event.invocation.result.exit_code),
  [1, 0]
);
assert.equal(dedupeTimelineItems([failed, { ...failed, id: 'duplicate' }]).length, 1);
const oldFailed = { ...failed, invocation: { ...failed.invocation, invocation_id: undefined } };
const merged = mergeRuntimeTimelineWithMessageTimeline(
  [oldFailed, succeeded],
  [pending, failed, succeeded]
);
assert.equal(merged.length, 2, 'streaming and persisted copies merge, independent Jobs do not');
assert.equal(merged.filter(event => event.invocation.result?.exit_code === 1).length, 1);
const legacy = {
  ...failed,
  invocation: { ...failed.invocation, skill_id: 'legacy', tool_name: 'run_script' },
};
assert.equal(
  dedupeTimelineItems([legacy, { ...legacy, id: 'duplicate' }]).length,
  1,
  'legacy deduplication unchanged'
);
console.log('Agent execution Job presentation and timeline regression checks passed.');
