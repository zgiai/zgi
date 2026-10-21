import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const source = readFileSync(
  path.join(process.cwd(), 'src/components/workflow/hooks/use-combined-workflow-save.ts'),
  'utf8'
);
const leaveGuard = readFileSync(
  path.join(process.cwd(), 'src/components/workflow/hooks/use-workflow-leave-guard.tsx'),
  'utf8'
);

assert.match(source, /useRef<Promise<boolean> \| null>\(null\)/);
assert.match(
  source,
  /const activeSave = inFlightSaveRef\.current;[\s\S]*await activeSave[\s\S]*continue;/,
  'a publish-time save must await the exact in-flight draft save promise'
);
assert.match(source, /inFlightSaveRef\.current = savePromise;[\s\S]*await savePromise/);
assert.match(
  source,
  /if \(!stateAfterFlush\.isDirty && !stateAfterFlush\.hasLayoutChanges\)[\s\S]*return true/,
  'a clean draft must be reported as safely saved'
);
assert.match(
  source,
  /catch \(error\)[\s\S]*return false/,
  'draft persistence failures must propagate as an explicit false result'
);
assert.match(
  source,
  /const saved = await handleCombinedSave\(\{ silent: true \}\);[\s\S]*if \(!saved\)[\s\S]*return;[\s\S]*publishWorkflowMutation\.mutateAsync/,
  'publishing must stop before the publish mutation when draft saving fails'
);
assert.match(source, /current\.graphVersion !== graphVersion/);
assert.match(source, /current\.nodes !== nodes/);
assert.match(source, /currentMetadata !== submittedMetadata/);
assert.match(
  leaveGuard,
  /const saved = await onSave\(\{ silent: true \}\);[\s\S]*if \(saved === false\) return;/,
  'save-and-leave must stay in the editor when draft persistence reports failure'
);

console.log('Workflow save/publish barrier checks passed.');
