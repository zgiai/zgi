import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { URL } from 'node:url';

const indexSource = readFileSync(
  new URL('../src/components/db/table-data/index.tsx', import.meta.url),
  'utf8'
);
const headerSource = readFileSync(
  new URL('../src/components/db/table-data/header.tsx', import.meta.url),
  'utf8'
);

assert.ok(
  indexSource.includes('className="flex min-h-0 flex-1 flex-col gap-4"'),
  'table data must fill the available page height'
);
assert.ok(
  indexSource.includes('className="min-h-0 flex-1 overflow-auto border rounded-md"'),
  'table rows must scroll inside the table region'
);
assert.ok(
  headerSource.includes("'sticky top-0 z-10"),
  'table header cells must stay visible while rows scroll'
);

console.log('DB table sticky header regression check passed.');
