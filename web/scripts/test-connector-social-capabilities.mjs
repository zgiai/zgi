import assert from 'node:assert/strict';
import { Blob } from 'node:buffer';
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
const image = new Blob(['fixture'], { type: 'image/png' });
await connectorService.prepareSocialImage(image);
assert.equal(calls[0][0], 'post');
assert.equal(calls[0][1], '/connectors/social-media/prepare-image');
assert.equal(calls[0][2], image, 'image bytes must not be JSON serialized or base64 doubled');
assert.equal(calls[0][3].headers['Content-Type'], 'application/octet-stream');
await connectorService.getSocialEvents('connection/id', { after_id: 9, limit: 7 });
assert.equal(calls[1][1], '/connector-connections/connection%2Fid/social-events');
assert.deepEqual(calls[1][3].params, { after_id: 9, limit: 7 });
await connectorService.acknowledgeSocialEvents('connection/id', 42);
assert.equal(calls[2][1], '/connector-connections/connection%2Fid/social-events/ack');
assert.deepEqual(calls[2][2], { through_id: 42 });
assert.equal(calls[2][3].retryAttemptsOverride, 0);
console.log('Social capability binary upload, cursor and acknowledgement client checks passed');
