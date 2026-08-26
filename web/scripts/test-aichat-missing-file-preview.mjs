import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const service = read('src/services/file-manage.service.ts');
const previewHook = read('src/hooks/file/use-file-original-preview-url.ts');
const messageBubble = read('src/components/chat/variants/aichat/message-bubble.tsx');

assert.match(
  service,
  /getOriginalPreviewUrl[\s\S]*skipErrorHandling:\s*true[\s\S]*retryAttemptsOverride:\s*0/,
  'missing original previews should bypass transport retries and global error handling'
);
assert.match(
  previewHook,
  /retry:\s*false,[\s\S]*retryOnMount:\s*false,[\s\S]*refetchOnReconnect:\s*false/,
  'a failed preview query should remain terminal across remounts and reconnects'
);
assert.doesNotMatch(
  previewHook,
  /toast\.error|from 'sonner'/,
  'preview lookup failures should not emit a toast'
);
assert.match(
  messageBubble,
  /const canPreview = Boolean\(previewUrl\) && !isError && !isFiltered/,
  'only successfully loaded historical images should be expandable'
);
assert.match(
  messageBubble,
  /disabled=\{!canPreview\}[\s\S]*<Dialog open=\{isPreviewOpen && canPreview\}/,
  'missing image attachments should disable expansion and keep the dialog closed'
);
assert.match(
  messageBubble,
  /const showUnavailableTooltip = !isLoading && \(isError \|\| isFiltered\)[\s\S]*<TooltipTrigger asChild>[\s\S]*\{previewButton\}[\s\S]*<TooltipContent[\s\S]*\{title \|\| t\('consoleChat\.attachments\.previewLoadError'\)\}/,
  'missing image errors should be shown only from the image hover tooltip'
);
assert.doesNotMatch(
  messageBubble,
  /toast\.error|aichat-image-preview-error/,
  'image load failures should not emit a toast'
);
assert.match(
  messageBubble,
  /refetchOnWindowFocus:\s*false,[\s\S]*refetchOnReconnect:\s*false,[\s\S]*retryOnMount:\s*false/,
  'missing managed generated files should not be requested again on focus, reconnect, or remount'
);

console.log('AIChat missing file preview regression checks passed.');
