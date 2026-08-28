import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const types = read('src/services/types/aichat.ts');
assert.match(types, /'context_compaction'/);

const reducer = read('src/components/chat/controllers/aichat/reducers/skill.ts');
assert.match(reducer, /payload\.phase === 'context_compaction'/);
assert.match(reducer, /item\.progress_id === payload\.progress_id/);
assert.match(reducer, /status !== 'completed'/);

const timeline = read('src/components/chat/variants/aichat/agentic-timeline.tsx');
assert.match(timeline, /consoleChat\.contextCompaction\.running/);
assert.match(timeline, /consoleChat\.contextCompaction\.completed/);
assert.match(timeline, /item\.status !== 'completed'/);

for (const locale of ['zh-Hans', 'en-US']) {
  const messages = read(`src/i18n/modules/webapp/${locale}.ts`);
  assert.match(messages, /contextCompaction:/);
  assert.match(messages, /running:/);
  assert.match(messages, /completed:/);
}

console.log('Context compaction progress checks passed.');
