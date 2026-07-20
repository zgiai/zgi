import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { URL } from 'node:url';

const source = readFileSync(
  new URL('../src/components/db/table-ingest/file-support.ts', import.meta.url),
  'utf8'
);
const documentExtensions = source.match(
  /TABLE_INGEST_DOCUMENT_EXTENSIONS = \[([\s\S]*?)\] as const;/
)?.[1];

assert.ok(documentExtensions, 'table ingest document extension policy must exist');
assert.ok(!documentExtensions.includes("'xlsx'"), 'smart ingest must reject .xlsx files');
assert.ok(!documentExtensions.includes("'xls'"), 'smart ingest must reject .xls files');
assert.ok(documentExtensions.includes("'csv'"), 'this change must not remove CSV support');

console.log('Table ingest Excel policy regression check passed.');
