import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');

const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const headerSource = read('src/components/console/console-header.tsx');
const sidebarSource = read('src/components/console/console-sidebar.tsx');
const detailSource = read('src/components/files/detail/file-detail-shell.tsx');
const zhSource = read('src/i18n/modules/files/zh-Hans.ts');
const enSource = read('src/i18n/modules/files/en-US.ts');

const requiredHeaderSnippets = [
  "import { usePathname, useSearchParams } from 'next/navigation';",
  'getDatasetReturnTo(searchParams.get',
  "const effectivePathname = datasetReturnTo ? '/console/dataset' : pathname;",
  'route.match(effectivePathname)',
];

for (const snippet of requiredHeaderSnippets) {
  if (!headerSource.includes(snippet)) {
    throw new Error(`Console header is missing dataset return context snippet: ${snippet}`);
  }
}

const requiredSidebarSnippets = [
  "import { usePathname, useSearchParams } from 'next/navigation';",
  'getDatasetReturnTo(searchParams.get',
  "const activePathname = datasetReturnTo ? '/console/dataset' : pathname;",
  'isItemActive(activePathname, item.href)',
];

for (const snippet of requiredSidebarSnippets) {
  if (!sidebarSource.includes(snippet)) {
    throw new Error(`Console sidebar is missing dataset return context snippet: ${snippet}`);
  }
}

const requiredDetailSnippets = [
  'const parentHref = datasetReturnTo ?? \'/console/files\';',
  "const parentLabel = datasetReturnTo ? t('detail.datasetBreadcrumb') : t('detail.fileBreadcrumb');",
  'onClick={() => router.push(parentHref)}',
  '{parentLabel}',
];

for (const snippet of requiredDetailSnippets) {
  if (!detailSource.includes(snippet)) {
    throw new Error(`File detail page is missing source-aware breadcrumb snippet: ${snippet}`);
  }
}

if (detailSource.includes('onClick={returnToDataset}')) {
  throw new Error('File detail page should not render a separate right-side Back to Dataset button.');
}

const expectedZhCopy = ["datasetBreadcrumb: '知识库'"];
for (const snippet of expectedZhCopy) {
  if (!zhSource.includes(snippet)) {
    throw new Error(`Missing zh-Hans source-aware breadcrumb copy: ${snippet}`);
  }
}

const expectedEnCopy = ["datasetBreadcrumb: 'Knowledge Base'"];
for (const snippet of expectedEnCopy) {
  if (!enSource.includes(snippet)) {
    throw new Error(`Missing en-US source-aware breadcrumb copy: ${snippet}`);
  }
}

console.log('File detail return context checks passed.');
