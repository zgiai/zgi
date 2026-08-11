import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const typesSource = readFileSync('src/services/types/aichat.ts', 'utf8');
const reducerSource = readFileSync(
  'src/components/chat/controllers/aichat/reducers/skill.ts',
  'utf8'
);

assert.match(typesSource, /invocation_id\?: string/);
assert.match(reducerSource, /invocation_id: payload\.invocation_id/);
assert.match(
  reducerSource,
  /if \(existing\.invocation_id && incoming\.invocation_id\)[\s\S]*return existing\.invocation_id === incoming\.invocation_id/,
  'consecutive calls to the same tool must be correlated by invocation id before semantic fallback'
);
assert.match(
  reducerSource,
  /incoming\.invocation_id &&[\s\S]*invocation\.invocation_id !== incoming\.invocation_id[\s\S]*return false/,
  'a running invocation with a different id must not absorb a consecutive call to the same tool'
);

console.log('Skill invocation correlation checks passed.');
