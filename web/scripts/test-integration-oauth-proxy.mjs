import assert from 'node:assert/strict';
import http from 'node:http';
import { once } from 'node:events';
import {
  proxyIntegrationOAuthRequest,
  proxyIntegrationProviderInstallRequest,
  proxyIntegrationWeComCallbackRequest,
} from '../src/server/integration-oauth-proxy.ts';

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
    contentType: request.headers['content-type'],
    forwardedHost: request.headers['x-forwarded-host'],
    forwardedProto: request.headers['x-forwarded-proto'],
    body: Buffer.concat(body).toString('utf8'),
  });
  if (
    request.url?.startsWith('/console/api/integrations/wecom/suite/events') ||
    request.url?.startsWith('/console/api/integrations/wecom/data/events/')
  ) {
    const payload = request.method === 'POST' ? 'success' : 'decrypted-echo';
    response.statusCode = 200;
    response.setHeader('content-type', 'text/plain; charset=utf-8');
    response.setHeader('transfer-encoding', 'chunked');
    response.write(payload.slice(0, 2));
    response.end(payload.slice(2));
    return;
  }
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

  const providerInstallStartResponse = await proxyIntegrationProviderInstallRequest(
    new globalThis.Request(
      'https://cloud.example.com/console/api/integrations/provider-install/flows?source=ui',
      {
        method: 'POST',
        headers: {
          authorization: 'Bearer access-token',
          cookie: 'session=current-session',
          'content-type': 'application/json',
        },
        body: JSON.stringify({ integration_id: 'wecom' }),
      }
    ),
    ['flows']
  );
  assert.equal(providerInstallStartResponse.status, 302);
  assert.equal(requests[3].url, '/console/api/integrations/provider-install/flows?source=ui');
  assert.equal(requests[3].authorization, 'Bearer access-token');
  assert.equal(requests[3].cookie, 'session=current-session');

  const providerInstallPollResponse = await proxyIntegrationProviderInstallRequest(
    new globalThis.Request(
      `https://cloud.example.com/console/api/integrations/provider-install/flows/${flowID}`,
      { headers: { cookie: 'session=current-session' } }
    ),
    ['flows', flowID]
  );
  assert.equal(providerInstallPollResponse.status, 302);
  assert.equal(requests[4].url, `/console/api/integrations/provider-install/flows/${flowID}`);

  const providerInstallCancelResponse = await proxyIntegrationProviderInstallRequest(
    new globalThis.Request(
      `https://cloud.example.com/console/api/integrations/provider-install/flows/${flowID}/cancel`,
      { method: 'POST', headers: { cookie: 'session=current-session' } }
    ),
    ['flows', flowID, 'cancel']
  );
  assert.equal(providerInstallCancelResponse.status, 302);
  assert.equal(
    requests[5].url,
    `/console/api/integrations/provider-install/flows/${flowID}/cancel`
  );

  const providerInstallCallbackResponse = await proxyIntegrationProviderInstallRequest(
    new globalThis.Request(
      'https://cloud.example.com/console/api/integrations/provider-install/callback'
    ),
    ['callback']
  );
  assert.equal(providerInstallCallbackResponse.status, 404);
  assert.equal(requests.length, 6);

  const installCallbackResponse = await proxyIntegrationWeComCallbackRequest(
    new globalThis.Request(
      'https://cloud.example.com/console/api/integrations/wecom/install/callback?auth_code=one-time-code&state=opaque-state',
      {
        headers: { cookie: '__Host-zgi_oauth_browser_binding=browser-secret' },
      }
    ),
    ['install', 'callback']
  );
  assert.equal(installCallbackResponse.status, 302);
  assert.equal(
    requests[6].url,
    '/console/api/integrations/wecom/install/callback?auth_code=one-time-code&state=opaque-state'
  );
  assert.equal(requests[6].cookie, '__Host-zgi_oauth_browser_binding=browser-secret');

  const invalidInstallEventResponse = await proxyIntegrationWeComCallbackRequest(
    new globalThis.Request(
      'https://cloud.example.com/console/api/integrations/wecom/install/callback',
      { method: 'POST', body: '<xml />' }
    ),
    ['install', 'callback']
  );
  assert.equal(invalidInstallEventResponse.status, 404);
  assert.equal(requests.length, 7);

  const suiteVerificationResponse = await proxyIntegrationWeComCallbackRequest(
    new globalThis.Request(
      'https://cloud.example.com/console/api/integrations/wecom/suite/events?msg_signature=signed&timestamp=1&nonce=2&echostr=encrypted'
    ),
    ['suite', 'events']
  );
  assert.equal(suiteVerificationResponse.status, 200);
  assert.equal(await suiteVerificationResponse.text(), 'decrypted-echo');
  assert.equal(suiteVerificationResponse.headers.get('content-length'), '14');
  assert.equal(suiteVerificationResponse.headers.get('transfer-encoding'), null);
  assert.equal(
    requests[7].url,
    '/console/api/integrations/wecom/suite/events?msg_signature=signed&timestamp=1&nonce=2&echostr=encrypted'
  );

  const encryptedSuiteEvent = '<xml><Encrypt>encrypted-event</Encrypt></xml>';
  const suiteEventResponse = await proxyIntegrationWeComCallbackRequest(
    new globalThis.Request(
      'https://cloud.example.com/console/api/integrations/wecom/suite/events?msg_signature=signed&timestamp=1&nonce=2',
      {
        method: 'POST',
        headers: { 'content-type': 'application/xml' },
        body: encryptedSuiteEvent,
      }
    ),
    ['suite', 'events']
  );
  assert.equal(suiteEventResponse.status, 200);
  assert.equal(await suiteEventResponse.text(), 'success');
  assert.equal(suiteEventResponse.headers.get('content-length'), '7');
  assert.equal(suiteEventResponse.headers.get('transfer-encoding'), null);
  assert.equal(
    requests[8].url,
    '/console/api/integrations/wecom/suite/events?msg_signature=signed&timestamp=1&nonce=2'
  );
  assert.equal(requests[8].contentType, 'application/xml');
  assert.equal(requests[8].forwardedHost, 'cloud.example.com');
  assert.equal(requests[8].forwardedProto, 'https');
  assert.equal(requests[8].body, encryptedSuiteEvent);

  const dataVerificationResponse = await proxyIntegrationWeComCallbackRequest(
    new globalThis.Request(
      'https://cloud.example.com/console/api/integrations/wecom/data/events/ww_provider?msg_signature=signed&timestamp=1&nonce=2&echostr=encrypted'
    ),
    ['data', 'events', 'ww_provider']
  );
  assert.equal(dataVerificationResponse.status, 200);
  assert.equal(await dataVerificationResponse.text(), 'decrypted-echo');
  assert.equal(dataVerificationResponse.headers.get('content-length'), '14');
  assert.equal(
    requests[9].url,
    '/console/api/integrations/wecom/data/events/ww_provider?msg_signature=signed&timestamp=1&nonce=2&echostr=encrypted'
  );

  const encryptedDataEvent = '<xml><Encrypt>encrypted-data-event</Encrypt></xml>';
  const dataEventResponse = await proxyIntegrationWeComCallbackRequest(
    new globalThis.Request(
      'https://cloud.example.com/console/api/integrations/wecom/data/events/ww_tenant?msg_signature=signed&timestamp=1&nonce=2',
      {
        method: 'POST',
        headers: { 'content-type': 'application/xml' },
        body: encryptedDataEvent,
      }
    ),
    ['data', 'events', 'ww_tenant']
  );
  assert.equal(dataEventResponse.status, 200);
  assert.equal(await dataEventResponse.text(), 'success');
  assert.equal(dataEventResponse.headers.get('content-length'), '7');
  assert.equal(
    requests[10].url,
    '/console/api/integrations/wecom/data/events/ww_tenant?msg_signature=signed&timestamp=1&nonce=2'
  );
  assert.equal(requests[10].body, encryptedDataEvent);

  const invalidDataReceiverResponse = await proxyIntegrationWeComCallbackRequest(
    new globalThis.Request(
      'https://cloud.example.com/console/api/integrations/wecom/data/events/not%20safe'
    ),
    ['data', 'events', 'not safe']
  );
  assert.equal(invalidDataReceiverResponse.status, 404);
  assert.equal(requests.length, 11);

  const invalidWeComCallbackResponse = await proxyIntegrationWeComCallbackRequest(
    new globalThis.Request('https://cloud.example.com/console/api/integrations/wecom/admin'),
    ['admin']
  );
  assert.equal(invalidWeComCallbackResponse.status, 404);
  assert.equal(requests.length, 11);

  const invalidResponse = await proxyIntegrationOAuthRequest(
    new globalThis.Request('https://cloud.example.com/console/api/integrations/oauth/admin', {
      method: 'POST',
    }),
    ['admin']
  );
  assert.equal(invalidResponse.status, 404);
  assert.equal(requests.length, 11);

  process.env.INTEGRATION_API_INTERNAL_URL = 'https://cloud.example.com';
  const loopResponse = await proxyIntegrationOAuthRequest(
    new globalThis.Request('https://cloud.example.com/console/api/integrations/oauth/callback'),
    ['callback']
  );
  assert.equal(loopResponse.status, 503);
  assert.equal(requests.length, 11);

  console.log('integration OAuth, provider-install, and WeCom callback proxy contracts passed');
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
