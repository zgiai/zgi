import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');

const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const contentSource = read('src/components/files/file-management-content.tsx');
const hookSource = read('src/hooks/use-files.ts');

const forbiddenCurrentPageCountSnippets = [
  'displayedTotal = processingStatusFilter !==',
  'loadedNeedsActionCount',
  'loadedReadyCount',
  'loadedActiveProcessingCount',
  'derivedStoredOnlyCount',
  'scopeFilteredFiles.filter(file => fileMatchesProcessingStatusFilter(file, filter.id))',
  "processingStatusFilter === 'all' &&",
];

for (const snippet of forbiddenCurrentPageCountSnippets) {
  if (contentSource.includes(snippet)) {
    throw new Error(
      `File management status counts must not be derived from current page data: ${snippet}`
    );
  }
}

const requiredContentSnippets = [
  'getProcessingStatusQueryParam(processingStatusFilter)',
  'countProcessingStatuses',
  'processingStatusFilterCounts[filter.id]',
  'const displayedTotal = total;',
  'const shouldShowPagination = !isLoading && files.length > 0 && totalPages > 1;',
];

for (const snippet of requiredContentSnippets) {
  if (!contentSource.includes(snippet)) {
    throw new Error(
      `File management status counts are missing full-scope status count logic: ${snippet}`
    );
  }
}

const requiredHookSnippets = [
  'processingStatus?: string;',
  'processingStatus,',
  'effectiveProcessingStatus',
  'processing_status: effectiveProcessingStatus',
  'processingStatus ||',
];

for (const snippet of requiredHookSnippets) {
  if (!hookSource.includes(snippet)) {
    throw new Error(
      `useFiles must pass an explicit processing_status filter to the API: ${snippet}`
    );
  }
}

if (
  hookSource.includes("processing_status: category === 'needs_action' ? 'parse_failed' : undefined")
) {
  throw new Error(
    'useFiles must not hardcode processing_status only for the needs_action category.'
  );
}

console.log('File management status count checks passed.');
