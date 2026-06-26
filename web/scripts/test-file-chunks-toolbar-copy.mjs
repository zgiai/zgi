import { readFileSync } from 'node:fs';

const panelSource = readFileSync('src/components/files/detail/file-chunks-panel.tsx', 'utf8');
const zhSource = readFileSync('src/i18n/modules/files/zh-Hans.ts', 'utf8');
const enSource = readFileSync('src/i18n/modules/files/en-US.ts', 'utf8');

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

assert(
  !panelSource.includes("t('detail.chunks.expandAll')") &&
    !panelSource.includes("t('detail.chunks.collapseAll')"),
  'Chunk toolbar should not render the expand/collapse all button.'
);

assert(
  zhSource.includes("all: '全部切片'"),
  'Chinese chunk filter "all" label should be "全部切片".'
);

assert(
  enSource.includes("all: 'All chunks'"),
  'English chunk filter "all" label should be "All chunks".'
);

console.log('File chunks toolbar copy checks passed.');
