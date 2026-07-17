import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { URL } from 'node:url';

const source = readFileSync(
  new URL('../src/components/db/table-ingest/step-two.tsx', import.meta.url),
  'utf8'
);

assert.ok(
  source.includes('activeRecordIndexes'),
  'the review workspace must track the active record for each file'
);
assert.ok(
  source.includes('fieldExtraction?.records?.[activeRecordIndex]'),
  'field evidence must follow the active record'
);
assert.ok(
  /Math\.min\(\s*activeRecordIndexes\[activeFileId\] \|\| 0,\s*Math\.max\(current\.records\.length - 1, 0\)\s*\)/.test(
    source
  ),
  'editing must clamp a stale record index after reprocessing returns fewer records'
);
assert.ok(
  source.includes('validFiles.flatMap'),
  'saving must include every recognized record from every valid file'
);
assert.ok(
  source.includes('state.records.some('),
  'file validation must reject any invalid record, not only the active record'
);
assert.ok(
  source.includes('state.records.reduce('),
  'pending field counts must include every record'
);
assert.ok(
  !source.includes('values: DbTableRecord;'),
  'records must be the only editable record data source'
);

console.log('Table ingest multi-record regression check passed.');
