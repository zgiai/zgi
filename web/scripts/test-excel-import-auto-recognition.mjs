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
  enterSchema.indexOf("setStep('schema')") < enterSchema.indexOf('await recognizeCurrentColumns()'),
  'preview-to-schema must enter the schema step before model recognition completes'
);
assert.ok(
  enterSchema.includes('if (!selectedModel?.model || recognizeMutation.isPending) return;'),
  'missing or busy models must not block the schema step'
);
assert.ok(
  source.includes('onClick={handleEnterSchema}'),
  'the preview next button must use the recognition handler'
);
assert.ok(
  source.includes('type: suggestion.type || col.type'),
  'recognized field types must replace preview-inferred types'
);
const previewNextButton = source.match(
  /<Button\s+onClick=\{handleEnterSchema\}([\s\S]*?)<\/Button>/
)?.[1];
assert.ok(previewNextButton, 'the preview next button must exist');
assert.ok(
  !previewNextButton.includes('selectedModel') && !previewNextButton.includes('recognizeMutation'),
  'the preview next button must not be disabled by model availability'
);
assert.ok(
  source.includes('schemaEditRevisionRef.current !== recognitionStartRevision'),
  'late recognition results must not overwrite manual schema edits'
);

console.log('Excel import auto-recognition regression check passed.');
