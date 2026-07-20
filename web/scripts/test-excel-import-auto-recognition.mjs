import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { URL } from 'node:url';
import { runLatestRecognition } from '../src/components/db/excel-import/recognition-request-guard.mjs';

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
assert.ok(
  source.includes('const [hasRecognitionCompleted, setHasRecognitionCompleted] = useState(false);'),
  'the import flow must track whether smart recognition completed'
);
assert.ok(
  source.includes("toast.info(t('excelImport.schema.recognitionIncomplete'))"),
  'clicking import before recognition completes must explain why it is unavailable'
);
assert.ok(
  source.includes('aria-disabled={!hasRecognitionCompleted'),
  'the import button must expose its unavailable state while recognition is incomplete'
);

let resolveRecognitionA;
const recognitionA = new Promise(resolve => {
  resolveRecognitionA = resolve;
});
const recognitionRequestSeqRef = { current: 0 };
const currentAnalysisKeyRef = { current: 'job-a:sheet-a' };
let appliedResult = null;

const pendingRecognitionA = runLatestRecognition({
  recognitionRequestSeqRef,
  currentAnalysisKeyRef,
  analysisKey: 'job-a:sheet-a',
  request: () => recognitionA,
});

recognitionRequestSeqRef.current += 1;
currentAnalysisKeyRef.current = 'job-b:sheet-b';
resolveRecognitionA({ table: { name: 'table_a' }, columns: [{ name: 'column_a' }] });

const staleResult = await pendingRecognitionA;
if (staleResult) appliedResult = staleResult;

assert.equal(staleResult, null, 'recognition A must be discarded after switching to analysis B');
assert.equal(appliedResult, null, 'recognition A must not be applied to analysis B');

console.log('Excel import auto-recognition regression check passed.');
