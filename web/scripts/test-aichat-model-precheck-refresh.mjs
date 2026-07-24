import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const workChat = fs.readFileSync(path.join(root, 'src/app/console/work/chat/page.tsx'), 'utf8');

assert.match(
  workChat,
  /previousIsSendingRef\s*=\s*useRef\(controller\.isSending\)/,
  'work chat should remember whether a real model request was running'
);
assert.match(
  workChat,
  /const\s*\{\s*refetch:\s*refetchModelPrecheck\s*\}\s*=\s*modelPrecheck[\s\S]*previousIsSendingRef\.current\s*&&\s*!controller\.isSending[\s\S]*refetchModelPrecheck\(\)/,
  'work chat should refresh model precheck when a real request reaches a terminal state'
);
assert.match(
  workChat,
  /previousIsSendingRef\.current\s*=\s*controller\.isSending/,
  'work chat should update the request transition marker after each sending state change'
);

console.log('AIChat model precheck refresh regression checks passed');
