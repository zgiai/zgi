import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';
import ts from 'typescript';

const source = await readFile(
  new URL('../src/components/chat/variants/aichat/skill-load-timeline.ts', import.meta.url),
  'utf8'
);
const moduleSource = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const { isRoutineSkillLoadInvocation, isUserRelevantSkillLoadFailure } = await import(
  `data:text/javascript;base64,${Buffer.from(moduleSource).toString('base64')}`
);

for (const invocation of [
  { kind: 'skill_load', status: 'success' },
  { kind: 'skill_load_attempt', status: 'auto_restored' },
  {
    kind: 'skill_load_attempt',
    status: 'skipped',
    arguments: { outcome: 'not_exposed_current_surface' },
  },
  {
    kind: 'skill_load_attempt',
    status: 'blocked',
    arguments: { outcome: 'policy_denied', access_status: 'unavailable' },
  },
]) {
  assert.equal(isRoutineSkillLoadInvocation(invocation), true, JSON.stringify(invocation));
}

for (const invocation of [
  { kind: 'skill_load', status: 'error' },
  {
    kind: 'skill_load_attempt',
    status: 'blocked',
    arguments: { outcome: 'policy_denied', access_status: 'denied' },
  },
  {
    kind: 'skill_load_attempt',
    status: 'error',
    arguments: { outcome: 'load_failed' },
  },
]) {
  assert.equal(isUserRelevantSkillLoadFailure(invocation), true, JSON.stringify(invocation));
  assert.equal(isRoutineSkillLoadInvocation(invocation), false, JSON.stringify(invocation));
}

console.log('Skill load timeline filtering regression checks passed.');
