import assert from 'node:assert/strict';
import { setTimeout as delay } from 'node:timers/promises';

import { runUploadPool } from '../src/components/common/file-upload/upload-pool.ts';

let active = 0;
let maxActive = 0;
const items = Array.from({ length: 12 }, (_, index) => index);

const results = await runUploadPool(items, 3, async item => {
  active += 1;
  maxActive = Math.max(maxActive, active);
  await delay((item % 3) + 1);
  active -= 1;
  return item * 2;
});

assert.equal(maxActive, 3);
assert.deepEqual(
  results,
  items.map(item => item * 2)
);

active = 0;
maxActive = 0;
await runUploadPool(items.slice(0, 4), 0, async item => {
  active += 1;
  maxActive = Math.max(maxActive, active);
  await delay(1);
  active -= 1;
  return item;
});
assert.equal(maxActive, 1);

console.log('File upload pool checks passed.');
