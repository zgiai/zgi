import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';
import {
  describeAccessLoadError,
  describePermissionLoadError,
  observePermissionLoad,
  shouldRetryAccessLoadError,
  shouldShowAccessLoadToast,
} from '../src/utils/access-load-error.ts';

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
assert.equal(describeAccessLoadError({ code: 'ERR_AUTH_REFRESH_STALE' }).kind, 'session');
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
const secret = 'private-test-value';
const unsafeError = {
  code: secret,
  message: secret,
  config: { headers: { Authorization: secret }, data: secret },
  response: { status: 503, data: { code: secret, message: secret }, headers: {} },
};
assert.deepEqual(describePermissionLoadError(unsafeError), {
  kind: 'server',
  status: 503,
  code: undefined,
  requestId: undefined,
  clientCode: 'unknown',
});
for (const code of ['ERR_NETWORK', 'ERR_AUTH_SESSION_MISSING', 'ERR_CANCELED', 'ERR_BUSINESS']) {
  assert.equal(describePermissionLoadError({ code }).clientCode, code);
}
assert.equal(shouldRetryAccessLoadError(0, { code: 'ERR_NETWORK' }), true);
assert.equal(shouldRetryAccessLoadError(1, { response: { status: 503 } }), true);
assert.equal(shouldRetryAccessLoadError(2, { code: 'ERR_NETWORK' }), false);
for (const error of [
  { response: { status: 401 } },
  { response: { status: 403 } },
  { response: { status: 404 } },
  { response: { status: 429 } },
  { code: 'ERR_CANCELED' },
]) {
  assert.equal(shouldRetryAccessLoadError(0, error), false);
}
assert.equal(shouldShowAccessLoadToast({ code: 'ERR_NETWORK' }), false);
assert.equal(shouldShowAccessLoadToast({ code: 'ERR_AUTH_SESSION_MISSING' }), false);
assert.equal(shouldShowAccessLoadToast({ code: 'ERR_CANCELED' }), false);
assert.equal(shouldShowAccessLoadToast({ name: 'AbortError' }), false);
assert.equal(shouldShowAccessLoadToast({ response: { status: 403 } }), true);
assert.equal(shouldShowAccessLoadToast({ response: { status: 503 } }), true);

const events = [];
const payload = Object.freeze({ permissions: Object.freeze(['asset.read']) });
assert.equal(
  await observePermissionLoad(
    async () => payload,
    event => events.push(event)
  ),
  payload
);
assert.equal(events.length, 0, 'successful reads do not create failure events');
await assert.rejects(
  observePermissionLoad(
    async () => {
      throw unsafeError;
    },
    event => events.push(event)
  ),
  error => error === unsafeError,
  'diagnostics must preserve the original failure and fail closed'
);
assert.equal(events.length, 1);
assert.ok(!JSON.stringify(events).includes(secret));
await assert.rejects(
  observePermissionLoad(
    async () => {
      throw unsafeError;
    },
    () => {
      throw new Error('reporter failed');
    }
  ),
  error => error === unsafeError,
  'reporter failures must not replace the original failure'
);
await assert.rejects(
  observePermissionLoad(
    async () => undefined,
    event => events.push(event)
  ),
  error => error.code === 'ERR_PERMISSION_RESPONSE_EMPTY'
);
assert.equal(events.at(-1).clientCode, 'ERR_PERMISSION_RESPONSE_EMPTY');
const hook = await readFile(
  new URL('../src/hooks/organization/use-account-permissions.ts', import.meta.url),
  'utf8'
);
assert.match(hook, /observePermissionLoad\(/);
assert.match(hook, /console\.warn\('workspace\.permissions\.load_failed', attributes\)/);
assert.match(hook, /captureEvent\('workspace\.permissions\.load_failed'/);
assert.match(hook, /shouldRetryAccessLoadError\(failureCount, queryError\)/);
assert.match(hook, /id: 'workspace-permissions-load-error'/);
const organizationsHook = await readFile(
  new URL('../src/hooks/organization/use-organizations.ts', import.meta.url),
  'utf8'
);
assert.equal(
  organizationsHook.match(/shouldRetryAccessLoadError\(failureCount, queryError\)/g)?.length,
  2
);
assert.match(organizationsHook, /id: 'organization-load-error'/);
const compatibilityExport = await readFile(
  new URL('../src/features/developer-access/load-error.ts', import.meta.url),
  'utf8'
);
assert.match(
  compatibilityExport,
  /export \{ describeAccessLoadError \} from '@\/utils\/access-load-error'/
);
console.log('Developer access error and permission load diagnostic checks passed.');
