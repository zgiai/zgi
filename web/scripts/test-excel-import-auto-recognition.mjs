import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { URL } from 'node:url';

const source = readFileSync(
  new URL('../src/components/db/excel-import/excel-import-shell.tsx', import.meta.url),
  'utf8'
);

const enterSchema = source.match(
  /const handleEnterSchema = async \(\) => \{([\s\S]*?)\n[ ]{2}\};/
)?.[1];

assert.ok(enterSchema, 'preview-to-schema handler must exist');
assert.ok(
  enterSchema.includes('await recognizeCurrentColumns()'),
  'preview-to-schema must wait for model recognition'
);
assert.ok(
  enterSchema.indexOf('applyRecognizedColumns') < enterSchema.indexOf("setStep('schema')"),
  'recognized columns must be applied before entering the schema step'
);
assert.ok(
  source.includes('onClick={handleEnterSchema}'),
  'the preview next button must use the recognition handler'
);
assert.ok(
  source.includes('type: suggestion.type || col.type'),
  'recognized field types must replace preview-inferred types'
);

console.log('Excel import auto-recognition regression check passed.');
