import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const source = fs.readFileSync(
  path.join(root, 'src', 'hooks', 'organization', 'use-organizations.ts'),
  'utf8'
);

assert.match(
  source,
  /refetchInterval:\s*5 \* 60 \* 1000/,
  'current organization data should continue to refresh in the background'
);
assert.doesNotMatch(
  source,
  /fetchOrgFailed|hasErrorProcessed/,
  'background organization refresh failures must not show a global toast'
);

console.log('Organization background refresh policy checks passed.');
