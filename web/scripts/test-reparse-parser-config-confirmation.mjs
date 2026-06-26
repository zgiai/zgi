import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');

const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const shellSource = read('src/components/files/detail/file-detail-shell.tsx');

const requiredShellSnippets = [
  "import { ConfirmDialog } from '@/components/ui/confirm-dialog';",
  "useState<'mineru' | 'reducto' | null>(null)",
  'setPendingParserConfigProvider(provider);',
  'handleConfirmConfigureParser',
  "title={t('detail.reparse.configureProviderConfirmTitle')}",
  "description={t('detail.reparse.configureProviderConfirmDescription')}",
  "confirmText={t('detail.reparse.configureProviderConfirmAction')}",
  "cancelText={common('cancel')}",
];

for (const snippet of requiredShellSnippets) {
  if (!shellSource.includes(snippet)) {
    throw new Error(`Missing reparse parser configuration confirmation snippet: ${snippet}`);
  }
}

const configureHandlerMatch = shellSource.match(
  /const handleConfigureParser = \(provider: 'mineru' \| 'reducto'\) => \{([\s\S]*?)\n {2}\};/
);

if (!configureHandlerMatch) {
  throw new Error('handleConfigureParser was not found.');
}

if (configureHandlerMatch[1].includes('router.push(')) {
  throw new Error('handleConfigureParser should open confirmation instead of navigating directly.');
}

const zhSource = read('src/i18n/modules/files/zh-Hans.ts');
const enSource = read('src/i18n/modules/files/en-US.ts');
const normalizedZhSource = zhSource.replace(/\s+/g, ' ');

const expectedZhCopy = [
  "configureProviderConfirmTitle: '前往解析器配置页面？'",
  "configureProviderConfirmDescription: '将离开当前重新解析窗口并打开解析器配置页面。当前文件的重新解析请求尚未提交。'",
  "configureProviderConfirmAction: '前往配置'",
];

for (const snippet of expectedZhCopy) {
  if (!normalizedZhSource.includes(snippet)) {
    throw new Error(`Missing zh-Hans parser configuration confirmation copy: ${snippet}`);
  }
}

const normalizedEnSource = enSource.replace(/\s+/g, ' ');
const expectedEnCopy = [
  "configureProviderConfirmTitle: 'Go to parser settings?'",
  "configureProviderConfirmDescription: 'You will leave the current reparse window and open parser settings. This file has not been submitted for reparsing yet.'",
  "configureProviderConfirmAction: 'Go to Settings'",
];

for (const snippet of expectedEnCopy) {
  if (!normalizedEnSource.includes(snippet)) {
    throw new Error(`Missing en-US parser configuration confirmation copy: ${snippet}`);
  }
}

console.log('Reparse parser configuration confirmation checks passed.');
