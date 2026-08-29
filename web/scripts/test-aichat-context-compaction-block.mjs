import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const errorUtils = read('src/components/chat/variants/aichat/error-utils.ts');
assert.match(errorUtils, /aichat\.context\.compaction_unavailable/);
assert.match(errorUtils, /contextCompactionBlocked\.title/);
assert.match(errorUtils, /contextCompactionBlocked\.description/);

const chat = read('src/components/chat/variants/aichat/aichat-chat.tsx');
assert.match(chat, /const contextBlockedMessage = useMemo/);
assert.match(chat, /current_leaf_message_id/);
assert.match(chat, /message\.id === contextBlockedMessage\?\.id/);
assert.match(chat, /disabled=\{contextBlocked\}/);
assert.match(chat, /contextCompactionBlocked\.retry/);

const input = read('src/components/chat/variants/aichat/input-area.tsx');
assert.match(input, /disabled\?: boolean/);
assert.match(input, /disabled=\{disabled\}/);

for (const locale of ['zh-Hans', 'en-US']) {
  const messages = read(`src/i18n/modules/webapp/${locale}.ts`);
  assert.match(messages, /contextCompactionBlocked:/);
  assert.match(messages, /retry:/);
}

console.log('AIChat context compaction blocking checks passed.');
