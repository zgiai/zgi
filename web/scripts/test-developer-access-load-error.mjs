import assert from 'node:assert/strict';
import { describeAccessLoadError } from '../src/features/developer-access/load-error.ts';

for (const [status, kind] of [
  [401, 'session'],
  [403, 'forbidden'],
  [404, 'notFound'],
  [500, 'server'],
  [503, 'server'],
  [429, 'unknown'],
]) {
  assert.equal(describeAccessLoadError({ response: { status } }).kind, kind);
}
assert.equal(
  describeAccessLoadError({ response: { status: 400, data: { code: '212012' } } }).kind,
  'session'
);
assert.equal(describeAccessLoadError({ code: 'ERR_AUTH_SESSION_MISSING' }).kind, 'session');
assert.equal(describeAccessLoadError({ code: 'ERR_NETWORK' }).kind, 'network');
assert.equal(describeAccessLoadError({ code: 'ECONNABORTED' }).kind, 'network');
assert.equal(describeAccessLoadError(new Error('Query data cannot be undefined')).kind, 'unknown');
assert.equal(describeAccessLoadError(null).kind, 'unknown');
const requestId = 'cd655c30-8e15-4cf2-8193-9ed2126c5c38';
assert.deepEqual(
  describeAccessLoadError({
    response: {
      status: 500,
      data: { code: 500001, message: 'private database details' },
      headers: { 'x-request-id': requestId },
    },
  }),
  {
    kind: 'server',
    status: 500,
    code: '500001',
    requestId,
  }
);
assert.deepEqual(
  describeAccessLoadError({
    message: 'secret',
    response: {
      status: '500',
      data: { code: 'sk-secret', message: 'secret' },
      headers: { 'x-request-id': 'Bearer secret', authorization: 'secret' },
    },
  }),
  {
    kind: 'unknown',
    status: undefined,
    code: undefined,
    requestId: undefined,
  }
);
console.log('Developer access load error checks passed.');
