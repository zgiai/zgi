import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import {
  parseWatchedServices,
  serverWatchProblems,
} from '../src/components/digital-human/digital-human-task-server-watch-model.ts';

assert.deepEqual(parseWatchedServices(' nginx, mysql，redis、nginx  php-fpm '), [
  'nginx',
  'mysql',
  'redis',
  'php-fpm',
]);
assert.deepEqual(parseWatchedServices('  '), []);

assert.deepEqual(
  serverWatchProblems({
    state: {
      redis: { healthy: false, observed: 'failed' },
      '@host': { healthy: false, observed: 'unreachable' },
      nginx: { healthy: true, observed: 'active' },
    },
  }),
  [
    { name: '@host', observed: 'unreachable' },
    { name: 'redis', observed: 'failed' },
  ]
);

// The watch is part of the shared task center, never the personal view.
const panel = readFileSync('src/components/digital-human/digital-human-task-panel.tsx', 'utf8');
assert.match(panel, /title === 'shared' \? <TaskServerWatch task=\{selected\} \/> : null/);

console.log('DigitalHuman server watch checks passed');
