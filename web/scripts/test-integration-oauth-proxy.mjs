import assert from 'node:assert/strict';
import http from 'node:http';
import { once } from 'node:events';
import { proxyIntegrationOAuthRequest } from '../src/server/integration-oauth-proxy.ts';

const originalInternalURL = process.env.INTEGRATION_API_INTERNAL_URL;
const originalAPIURL = process.env.API_URL;

const requests = [];
const server = http.createServer(async (request, response) => {
  const body = [];
  for await (const chunk of request) body.push(chunk);
  requests.push({
    method: request.method,
    url: request.url,
    authorization: request.headers.authorization,
    cookie: request.headers.cookie,
    body: Buffer.concat(body).toString('utf8'),
  });
  response.statusCode = 302;
  response.setHeader('location', 'https://cloud.example.com/console/integrations/oauth/result');
  response.setHeader('set-cookie', [
    '__Host-zgi_oauth_browser_binding=browser-secret; Path=/; HttpOnly; Secure; SameSite=Lax',
    'oauth-secondary=value; Path=/; HttpOnly; Secure; SameSite=Lax',
  ]);
  response.end();
});

try {
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const address = server.address();
  assert(address && typeof address !== 'string');
  process.env.INTEGRATION_API_INTERNAL_URL = `http://127.0.0.1:${address.port}`;
  delete process.env.API_URL;

  const startResponse = await proxyIntegrationOAuthRequest(
    new globalThis.Request(
      'https://cloud.example.com/console/api/integrations/oauth/flows?source=ui',
      {
        method: 'POST',
        headers: {
          authorization: 'Bearer access-token',
          cookie: 'session=current-session',
          'content-type': 'application/json',
        },
        body: JSON.stringify({ integration_id: 'feishu' }),
      }
    ),
    ['flows']
  );
  assert.equal(startResponse.status, 302);
  assert.equal(
    startResponse.headers.get('location'),
    'https://cloud.example.com/console/integrations/oauth/result'
  );
  const setCookies = startResponse.headers.getSetCookie();
  assert.equal(setCookies.length, 2);
  assert(setCookies[0].includes('__Host-zgi_oauth_browser_binding=browser-secret'));
  assert.equal(requests[0].method, 'POST');
  assert.equal(requests[0].url, '/console/api/integrations/oauth/flows?source=ui');
  assert.equal(requests[0].authorization, 'Bearer access-token');
  assert.equal(requests[0].cookie, 'session=current-session');
  assert.equal(requests[0].body, JSON.stringify({ integration_id: 'feishu' }));

  const flowID = 'a'.repeat(43);
  const pollResponse = await proxyIntegrationOAuthRequest(
    new globalThis.Request(
      `https://cloud.example.com/console/api/integrations/oauth/flows/${flowID}`,
      {
        headers: { cookie: '__Host-zgi_oauth_browser_binding=browser-secret' },
      }
    ),
    ['flows', flowID]
  );
  assert.equal(pollResponse.status, 302);
  assert.equal(requests[1].url, `/console/api/integrations/oauth/flows/${flowID}`);

  const callbackResponse = await proxyIntegrationOAuthRequest(
    new globalThis.Request(
      'https://cloud.example.com/console/api/integrations/oauth/callback?code=one-time-code&state=opaque-state',
      {
        headers: { cookie: '__Host-zgi_oauth_browser_binding=browser-secret' },
      }
    ),
    ['callback']
  );
  assert.equal(callbackResponse.status, 302);
  assert.equal(
    requests[2].url,
    '/console/api/integrations/oauth/callback?code=one-time-code&state=opaque-state'
  );
  assert.equal(requests[2].cookie, '__Host-zgi_oauth_browser_binding=browser-secret');

  const invalidResponse = await proxyIntegrationOAuthRequest(
    new globalThis.Request('https://cloud.example.com/console/api/integrations/oauth/admin', {
      method: 'POST',
    }),
    ['admin']
  );
  assert.equal(invalidResponse.status, 404);
  assert.equal(requests.length, 3);

  process.env.INTEGRATION_API_INTERNAL_URL = 'https://cloud.example.com';
  const loopResponse = await proxyIntegrationOAuthRequest(
    new globalThis.Request('https://cloud.example.com/console/api/integrations/oauth/callback'),
    ['callback']
  );
  assert.equal(loopResponse.status, 503);
  assert.equal(requests.length, 3);

  console.log('integration OAuth same-origin proxy contract passed');
} finally {
  server.close();
  await once(server, 'close');
  if (originalInternalURL === undefined) {
    delete process.env.INTEGRATION_API_INTERNAL_URL;
  } else {
    process.env.INTEGRATION_API_INTERNAL_URL = originalInternalURL;
  }
  if (originalAPIURL === undefined) {
    delete process.env.API_URL;
  } else {
    process.env.API_URL = originalAPIURL;
  }
}
