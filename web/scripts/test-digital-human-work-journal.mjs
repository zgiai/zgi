import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import ts from 'typescript';
const modelPath = new URL(
  '../src/components/digital-human/digital-human-work-journal-model.ts',
  import.meta.url
);
const source = await readFile(modelPath, 'utf8').catch(() => '');
assert.ok(source, 'work journal model must exist');
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ES2022 },
}).outputText;
const { appendWorkJournalPrompt, workJournalQueryKey, sameWorkJournalScope } = await import(
  `data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`
);
assert.equal(
  appendWorkJournalPrompt('My unsaved persona', 'Example instructions'),
  'My unsaved persona\n\nExample instructions'
);
assert.equal(appendWorkJournalPrompt('', 'Example instructions'), 'Example instructions');
const scope = {
  accountId: 'account',
  organizationId: 'org',
  workspaceId: 'workspace',
  employeeId: 'employee',
};
assert.deepEqual(workJournalQueryKey(scope), [
  'digital-humans',
  'work-journal',
  'account',
  'org',
  'workspace',
  'employee',
]);
for (const key of Object.keys(scope)) {
  assert.notDeepEqual(
    workJournalQueryKey(scope),
    workJournalQueryKey({ ...scope, [key]: 'different' })
  );
  assert.equal(sameWorkJournalScope(scope, { ...scope, [key]: 'different' }), false);
}
assert.equal(sameWorkJournalScope(scope, undefined), false);
assert.equal(sameWorkJournalScope(scope, { ...scope }), true);
const hook = await readFile(
  new URL('../src/hooks/digital-human/use-work-journal.ts', import.meta.url),
  'utf8'
);
assert.match(hook, /retry: false/);
assert.match(hook, /sameWorkJournalScope/);
assert.doesNotMatch(hook, /keepPreviousData|placeholderData/);
const component = await readFile(
  new URL('../src/components/digital-human/digital-human-work-journal.tsx', import.meta.url),
  'utf8'
);
assert.match(component, /digital_human.logs.view/);
assert.match(component, /onAppendPrompt/);
assert.doesNotMatch(component, /dispatchOperation|recoverOperation|resetOperation/);
const config = await readFile(
  new URL('../src/components/digital-human/digital-human-configuration.tsx', import.meta.url),
  'utf8'
);
assert.match(config, /DigitalHumanWorkJournal/);
assert.doesNotMatch(config, /DigitalHumanSocialOperations/);
console.log('Work journal scope, draft preservation and read-only surface contracts passed');
