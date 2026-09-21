// Focused regression test for private-key safe HTTP error reporting.
//
// HttpClient.reportRequestError forwards the whole error to observability, and
// AxiosError.config.data can still hold a submitted SSH private key. Requests
// that set skipErrorHandling must therefore stay out of error reporting on
// every branch of the response interceptor.
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';
import ts from 'typescript';

const require = createRequire(import.meta.url);
const axios = require('axios');
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../src');
const readSource = relativePath => readFileSync(path.join(process.cwd(), relativePath), 'utf8');

const SECRET = 'serverops-ssh-private-key-sentinel';

function createFixture() {
  const captures = [];
  const networkNotices = [];
  const modules = new Map();
  const defaultTokenManager = {
    ensureValidToken: async () => 'access-token',
    getAuthToken: () => 'access-token',
    canUseToken: () => true,
    getRefreshToken: () => 'refresh-token',
    getOrRefreshToken: async () => 'refreshed-access-token',
    clearAuthData() {},
    redirectToLogin() {},
  };
  const tokenManager = { ...defaultTokenManager };

  const mocks = {
    '@/utils/error-notifications': {
      ErrorNotificationService: { showNetworkError: () => networkNotices.push('network') },
    },
    '@/lib/auth/logout-state': {
      isAuthRedirectInProgress: () => false,
      isLogoutInProgress: () => false,
    },
    '@/lib/observability': {
      captureError: (error, name, options) => captures.push({ error, name, options }),
    },
  };
  const replacements = {
    './config': {
      getEndpointConfig: () => ({ name: 'main', baseURL: 'http://localhost', timeout: 0 }),
      getHttpConfig: () => ({ globalTimeout: 1_000, retryAttempts: 0, retryDelay: 0 }),
    },
    './token-manager': {
      TokenManager: class {
        constructor() {
          return tokenManager;
        }
      },
      isStaleTokenRefreshError: () => false,
      StaleTokenRefreshError: class StaleTokenRefreshError extends Error {},
    },
    './sse-client': {
      SseClient: class {
        sse() {}
        ssePost() {}
      },
    },
  };

  function load(relative) {
    const filename = path.resolve(root, relative);
    if (modules.has(filename)) return modules.get(filename).exports;
    const module = { exports: {} };
    modules.set(filename, module);
    const code = ts.transpileModule(readFileSync(filename, 'utf8'), {
      fileName: filename,
      compilerOptions: {
        module: ts.ModuleKind.CommonJS,
        target: ts.ScriptTarget.ES2022,
        esModuleInterop: true,
      },
    }).outputText;
    const resolveImport = name => {
      if (Object.hasOwn(mocks, name)) return mocks[name];
      if (Object.hasOwn(replacements, name)) return replacements[name];
      if (name.startsWith('@/') || name.startsWith('.')) {
        const base = name.startsWith('@/')
          ? path.join(root, name.slice(2))
          : path.resolve(path.dirname(filename), name);
        if (existsSync(base + '.ts')) return load(base + '.ts');
        if (existsSync(base + '.tsx')) return load(base + '.tsx');
      }
      return require(name);
    };
    vm.runInNewContext(code, { module, exports: module.exports, require: resolveImport }, { filename });
    return module.exports;
  }

  const { HttpClient } = load('lib/http/client.ts');
  const client = new HttpClient();
  return {
    client,
    captures,
    networkNotices,
    // A submitted payload is JSON-encoded by axios before it reaches the adapter.
    networkError: () => async config => {
      throw new axios.AxiosError('Network Error', 'ERR_NETWORK', {
        ...config,
        data: JSON.stringify({ private_key: SECRET }),
      });
    },
    unauthorized: () => async config => {
      throw new axios.AxiosError('Unauthorized', 'ERR_BAD_RESPONSE', config, undefined, {
        status: 401,
        data: {},
        config,
      });
    },
    failRefresh: () => {
      Object.assign(tokenManager, {
        getOrRefreshToken: async () => {
          throw new Error('refresh failed');
        },
      });
    },
  };
}

const sensitiveConfig = extra => ({
  retryAttemptsOverride: 0,
  ...extra,
});

// --- Network-error branch ---------------------------------------------------

{
  const fixture = createFixture();
  fixture.client.getInstance().defaults.adapter = fixture.networkError();
  await assert.rejects(
    fixture.client.post('/connectors/x/credential-setups', { values: {} }, sensitiveConfig({}))
  );
  assert.equal(fixture.captures.length, 1, 'default callers must still report network failures');
  assert.equal(fixture.captures[0].name, 'http.request.failed');
  assert.equal(fixture.captures[0].options.attributes.auth.reason, 'network_error');
  assert.equal(fixture.networkNotices.length, 1);
  assert.ok(
    String(fixture.captures[0].error?.config?.data).includes(SECRET),
    'without the flag the reported AxiosError still carries the raw request payload'
  );
}

{
  const fixture = createFixture();
  fixture.client.getInstance().defaults.adapter = fixture.networkError();
  await assert.rejects(
    fixture.client.post(
      '/connectors/x/credential-setups',
      { values: {} },
      sensitiveConfig({ skipErrorHandling: true })
    )
  );
  assert.equal(
    fixture.captures.length,
    0,
    'skipErrorHandling must suppress reportRequestError on network failures'
  );
  assert.equal(fixture.networkNotices.length, 0, 'skipErrorHandling must suppress the network toast');
}

// --- Auth-refresh-failed branch --------------------------------------------

{
  const fixture = createFixture();
  fixture.client.getInstance().defaults.adapter = fixture.unauthorized();
  fixture.failRefresh();
  await assert.rejects(
    fixture.client.post('/connectors/x/credential-setups', { values: {} }, sensitiveConfig({}))
  );
  assert.equal(fixture.captures.length, 1, 'default callers must still report refresh failures');
  assert.equal(fixture.captures[0].options.attributes.auth.reason, 'token_refresh_failed');
}

{
  const fixture = createFixture();
  fixture.client.getInstance().defaults.adapter = fixture.unauthorized();
  fixture.failRefresh();
  await assert.rejects(
    fixture.client.post(
      '/connectors/x/credential-setups',
      { values: {} },
      sensitiveConfig({ skipErrorHandling: true })
    )
  );
  assert.equal(
    fixture.captures.length,
    0,
    'skipErrorHandling must suppress reportRequestError on refresh failures'
  );
}

// --- Source contract --------------------------------------------------------
// The token-refresh guard is defensive: the non-2xx early return already covers
// it today, but the network path did not, so pin both guards to the flag.
const clientSource = readSource('src/lib/http/client.ts');
assert.match(
  clientSource,
  /!shouldSilence\s*&&\s*!config\?\.skipErrorHandling/,
  'network reportRequestError must honor skipErrorHandling'
);
assert.match(
  clientSource,
  /!isStaleTokenRefreshError\(refreshError\)\s*&&\s*!config\?\.skipErrorHandling/,
  'token-refresh reportRequestError must honor skipErrorHandling'
);

console.log('HTTP sensitive error-reporting checks passed.');
