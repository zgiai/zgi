import assert from 'node:assert/strict';

import { conversationTitleNeedsRefresh } from '../src/components/chat/controllers/conversation-title.ts';

for (const title of [
  '',
  'New Conversation',
  '\u65b0\u5efa\u4f1a\u8bdd',
  'Conversation 2026-08-14 00:21:01',
  '\u4f1a\u8bdd 2026-08-14 00:21:01',
]) {
  assert.equal(conversationTitleNeedsRefresh(title), true, `expected refresh for ${title}`);
}

for (const title of ['\u91cf\u5b50\u8ba1\u7b97\u6838\u5fc3\u6982\u5ff5', '\u4f1a\u8bdd\u9000\u6b3e\u8fdb\u5ea6\u67e5\u8be2', 'Conversation about refunds']) {
  assert.equal(conversationTitleNeedsRefresh(title), false, `expected stable title for ${title}`);
}

assert.equal(
  conversationTitleNeedsRefresh('initial user query', { title_generation_status: 'pending' }),
  true,
  'pending runtime titles should refresh'
);
assert.equal(
  conversationTitleNeedsRefresh('initial user query', { title_generation_status: 'failed' }),
  true,
  'failed runtime titles should retry'
);
assert.equal(
  conversationTitleNeedsRefresh('initial user query', { title_generation_status: 'completed' }),
  false,
  'completed runtime titles should stay stable'
);

console.log('conversation title refresh tests passed');
