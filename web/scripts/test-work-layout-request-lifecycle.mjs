import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const layoutSource = fs.readFileSync(
  path.join(rootDir, 'src', 'app', 'console', 'work', 'layout.tsx'),
  'utf8'
);

assert.match(layoutSource, /if \(isCapabilitiesLoading\)/, 'initial loading should block work pages');
assert.doesNotMatch(
  layoutSource,
  /isCapabilitiesFetching/,
  'background capability refresh should not unmount active work pages'
);

console.log('Work layout request lifecycle checks passed.');
