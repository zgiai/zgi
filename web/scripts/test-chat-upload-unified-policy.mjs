import assert from 'node:assert/strict';
import fs from 'node:fs';

const read = file => fs.readFileSync(new URL(`../${file}`, import.meta.url), 'utf8');
const input = read('src/components/chat/variants/aichat/input-area.tsx');
const workflow = read('src/components/chat/ui/user-input/index.tsx');
const history = read('src/components/chat/variants/aichat/message-bubble.tsx');

for (const field of ['number_limit', 'document_max_bytes', 'image_max_bytes', 'turn_max_bytes', 'allowed_extensions']) {
  assert.ok(input.includes(field), `chat composer must consume server ${field}`);
  assert.ok(workflow.includes(field), `workflow composer must consume server ${field}`);
}
assert.ok(workflow.includes('uploadFeature?.effective_policy'), 'published workflow policy must take priority');
assert.match(input, /source:\s*'chat'/, 'console uploads must request the chat policy');
assert.doesNotMatch(input, /supportsVision\s*&&\s*imageUploadEnabled|requireVision:\s*true/, 'images must not force vision model selection');
assert.match(history, /const canPreview = Boolean\(previewUrl\) && !isError/, 'model filtering must not disable original-file preview');
assert.doesNotMatch(history, /canPreview\s*=.*!isFiltered/, 'model filtering is not a file authorization decision');
assert.doesNotMatch(history, /isFileError\s*\|\|\s*isFiltered/, 'content filtering must not be rendered as a file failure');
assert.match(history, /isFiltered\s*\?\s*\(\s*<FileText/, 'filtered document metadata must keep a neutral file icon');
assert.match(history, /attachments\.filteredContentHint/, 'filtered content must explain retained original-file access');
for (const locale of ['zh-Hans', 'en-US']) {
  const messages = read(`src/i18n/modules/webapp/${locale}.ts`);
  assert.match(messages, /filteredContentHint:/, `${locale} must explain model content injection separately from tool file use`);
  assert.doesNotMatch(messages, /filtered: '未参与回答'/, 'content injection does not prove whether a tool used the file');
}
console.log('Unified document/image upload UI policy checks passed.');
