import assert from 'node:assert/strict';
import { load } from './connector-fixture.mjs';

const calls = [];
class BaseService {
  request(...args) {
    calls.push(args);
    return Promise.resolve({ data: {} });
  }
}
const { connectorService } = load('src/services/connector.service.ts', {
  '@/lib/http/services': { BaseService },
});
const grant = {
  operations: ['x-social.post.create'],
  destinations: { account_id: 'account-1' },
  expires_at: '2026-10-21T00:00:00Z',
  max_invocations: 20,
};
await connectorService.issueBackgroundGrant('binding/id', grant);
assert.equal(calls[0][0], 'post');
assert.equal(calls[0][1], '/connector-target-bindings/binding%2Fid/background-grants');
assert.deepEqual(calls[0][2], grant);
assert.equal(calls[0][3].retryAttemptsOverride, 0);
await connectorService.getBackgroundGrants();
assert.equal(calls[1][1], '/connector-background-grants');
await connectorService.revokeBackgroundGrant('grant/id');
assert.equal(calls[2][0], 'delete');
assert.equal(calls[2][1], '/connector-background-grants/grant%2Fid');
assert.equal(calls[2][3].retryAttemptsOverride, 0);
console.log('Background grant issue/list/revoke client contracts passed');
