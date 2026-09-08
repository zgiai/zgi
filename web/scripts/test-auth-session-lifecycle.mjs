import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';
import test from 'node:test';
import ts from 'typescript';

const require = createRequire(import.meta.url);
const axios = require('axios');
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../src');
const jwt = seconds =>
  `header.${Buffer.from(JSON.stringify({ exp: Date.now() / 1000 + seconds })).toString('base64url')}.signature`;

// Run production modules with controlled storage, transport, and React effect boundaries.
function fixture() {
  const storage = new Map();
  const notices = [];
  const redirects = [];
  const requests = [];
  const effects = [];
  const diagnostics = [];
  let diagnosticFailure = false;
  const dependencies = [];
  let effectIndex = 0;
  let mode = 200;
  let queryError = null;
  let sessionClears = 0;
  let executeTimeouts = false;
  const auth = {
    isAuthenticated: true,
    isSystemReady: true,
    isLoading: false,
    user: null,
    initializeAuth: async () => {},
    reset: () => {
      auth.isAuthenticated = false;
    },
  };
  const storageAPI = {
    getItem: key => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, value),
    removeItem: key => storage.delete(key),
  };
  const window = {
    localStorage: storageAPI,
    sessionStorage: storageAPI,
    location: { pathname: '/console/db', search: '?page=2', replace: url => redirects.push(url) },
    addEventListener() {},
    removeEventListener() {},
  };
  const mocks = {
    '@/lib/config': {
      ENABLE_ROOT_COOKIE_TOKEN_SYNC: false,
      ROOT_COOKIE_DOMAIN: '',
      withBasePath: p => p,
    },
    '@/utils/cookie': { deleteCookie() {}, setRawCookie() {} },
    '@/utils/client-id': { generateClientId: () => 'tab-1' },
    '@/lib/observability': { captureError() {} },
    '@/utils/logout-redirect': { consumePendingLogoutRedirect: () => null },
    '@/lib/i18n': { getCurrentLocale: () => 'zh-Hans' },
    sonner: { toast: { error: (message, options) => notices.push({ message, options }) } },
    react: {
      useMemo: callback => callback(),
      useEffect: (callback, deps) => {
        const index = effectIndex++;
        const previous = dependencies[index];
        if (!previous || deps.some((value, i) => !Object.is(value, previous[i]))) {
          dependencies[index] = deps;
          effects.push(callback);
        }
      },
    },
    '@tanstack/react-query': { useQuery: () => ({ error: queryError }) },
    '@/i18n': { useT: () => key => key },
    'next/navigation': {
      usePathname: () => window.location.pathname,
      useRouter: () => ({ push: url => redirects.push(url) }),
    },
    'lucide-react': { AlertCircle: () => null },
    '@/components/brand/zgi-loading-screen': { ZgiLoadingScreen: () => null },
    '@/services': {},
    '@/store/workspace-store': {},
    '@/store/organization-store': {},
    '@/store/auth-store': {
      useAuthStore: {
        use: Object.fromEntries(Object.keys(auth).map(key => [key, () => auth[key]])),
        getState: () => auth,
      },
    },
    '@/lib/query-client': { queryClient: {} },
    '@/lib/auth/client-state': {
      clearSessionBoundClientState: async () => {
        sessionClears++;
      },
    },
    '@/utils/client-cache': {},
    '@/lib/auth/context-sync': {},
    '@/hooks/query-keys': { DB_KEYS: { list: () => ['dbs'], tableList: () => ['tables'] } },
    '@/hooks/query-utils': {},
    '@/utils/agent-resource-bound': {},
  };
  const modules = new Map();
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
        jsx: ts.JsxEmit.ReactJSX,
        esModuleInterop: true,
      },
    }).outputText;
    const resolveImport = name => {
      if (Object.hasOwn(mocks, name)) return mocks[name];
      if (name === './config')
        return {
          getEndpointConfig: () => ({ name: 'main', baseURL: 'http://localhost' }),
          getHttpConfig: () => ({ globalTimeout: 10000, retryAttempts: 0 }),
        };
      if (name.startsWith('@/') || name.startsWith('.')) {
        const base = name.startsWith('@/')
          ? path.join(root, name.slice(2))
          : path.resolve(path.dirname(filename), name);
        return load(base + (existsSync(base + '.ts') ? '.ts' : '.tsx'));
      }
      return require(name);
    };
    vm.runInNewContext(
      code,
      {
        module,
        exports: module.exports,
        require: resolveImport,
        window,
        console: {
          ...console,
          info: (...args) => {
            if (diagnosticFailure) throw new Error('Synthetic reporter failure');
            diagnostics.push(args);
          },
        },
        atob: globalThis.atob,
        Date,
        Error,
        Promise,
        URL: globalThis.URL,
        AbortController: globalThis.AbortController,
        TextDecoder: globalThis.TextDecoder,
        setTimeout: callback => {
          if (executeTimeouts) Promise.resolve().then(callback);
          return 1;
        },
        clearTimeout() {},
        BroadcastChannel: undefined,
      },
      { filename }
    );
    return module.exports;
  }
  const session = load('lib/auth/session-manager.ts').sessionManager;
  const { HttpClient } = load('lib/http/client.ts');
  const client = new HttpClient();
  client.getInstance().defaults.adapter = async config => {
    requests.push(config.url);
    if (config.url === '/console/api/refresh-token') {
      if (mode === 'network') throw new axios.AxiosError('Network Error', 'ERR_NETWORK', config);
      if (typeof mode === 'function') return mode(config);
      if (mode !== 200)
        throw new axios.AxiosError('Refresh failed', 'ERR_BAD_RESPONSE', config, undefined, {
          status: mode,
          data: { code: mode === 400 ? 401002 : 'unavailable' },
          config,
        });
      return {
        status: 200,
        data: { access_token: jwt(3600), refresh_token: 'new-refresh' },
        config,
      };
    }
    return { status: 200, data: { code: 0 }, config };
  };
  return {
    client,
    session,
    load,
    diagnostics,
    failDiagnostics: () => {
      diagnosticFailure = true;
    },
    mock: (name, value) => {
      mocks[name] = value;
    },
    notices,
    redirects,
    requests,
    auth,
    window,
    setMode: value => {
      mode = value;
    },
    setError: value => {
      queryError = value;
    },
    executeTimeouts: () => {
      executeTimeouts = true;
    },
    seed: () => session.setSession({ accessToken: jwt(-60), refreshToken: 'refresh' }),
    render: callback => {
      effectIndex = 0;
      callback();
    },
    commit: () => effects.splice(0).map(callback => callback()),
    get sessionClears() {
      return sessionClears;
    },
  };
}

test('opted-in GET retries a transient network failure without notifying before recovery', async () => {
  const f = fixture();
  f.session.setSession({ accessToken: jwt(3600), refreshToken: 'refresh' });
  f.executeTimeouts();
  let attempts = 0;
  f.client.getInstance().defaults.adapter = async config => {
    attempts++;
    if (attempts < 3) {
      throw new axios.AxiosError('Network Error', 'ERR_NETWORK', config);
    }
    return { status: 200, data: { code: 0 }, config };
  };

  await f.client.get('/profile', { retryAttemptsOverride: 2 });

  assert.equal(attempts, 3);
  assert.equal(f.notices.length, 0);
});

test('network retry never replays a mutation even when an override is present', async () => {
  const f = fixture();
  f.session.setSession({ accessToken: jwt(3600), refreshToken: 'refresh' });
  f.executeTimeouts();
  let attempts = 0;
  f.client.getInstance().defaults.adapter = async config => {
    attempts++;
    throw new axios.AxiosError('Network Error', 'ERR_NETWORK', config);
  };

  await assert.rejects(f.client.post('/mutation', {}, { retryAttemptsOverride: 2 }));

  assert.equal(attempts, 1);
  assert.equal(f.notices.length, 1);
});

test('profile bootstrap explicitly opts into bounded read retries', async () => {
  const f = fixture();
  let request;
  f.mock('@/lib/http/services', {
    BaseService: class {
      async request(...args) {
        request = args;
        return { code: '0', data: { id: 'profile-account' } };
      }
    },
  });
  const service = f.load('services/auth.service.ts').authenticationService;

  const profile = await service.getProfile(false);

  assert.equal(profile.id, 'profile-account');
  assert.equal(request[0], 'get');
  assert.equal(request[1], '/account/profile');
  assert.equal(request[3].retryAttemptsOverride, 2);
});

function logoutService(f, request) {
  f.mock('@/lib/http/services', {
    BaseService: class {
      request(...args) {
        return request(...args);
      }
    },
  });
  return f.load('services/auth.service.ts').authenticationService;
}

test('logout diagnostics expose only allowlisted fields and never interrupt cleanup', async () => {
  const f = fixture();
  f.seed();
  const privateValue = 'private-test-credential';
  const failure = {
    message: privateValue,
    config: { headers: { Authorization: privateValue } },
    response: {
      status: 503,
      data: { code: '123456', message: privateValue },
      headers: { 'x-request-id': '12345678-1234-1234-1234-123456789abc' },
    },
  };
  await logoutService(f, async () => {
    throw failure;
  }).logout();
  assert.equal(f.session.hasSession(), false);
  const event = f.diagnostics
    .map(([marker, data]) => [marker, JSON.parse(data)])
    .find(([, data]) => data.phase === 'request_failed');
  assert.equal(event[0], 'auth.logout.phase');
  assert.equal(event[1].session_present, true);
  assert.equal(event[1].logout_in_progress, false);
  const clearedEvent = f.diagnostics
    .map(([, data]) => JSON.parse(data))
    .find(data => data.phase === 'session_cleared');
  assert.equal(clearedEvent.session_present, false);
  assert.equal(clearedEvent.logout_in_progress, false);
  assert.equal(event[1].status, 503);
  assert.equal(event[1].code, '123456');
  assert.equal(event[1].requestId, '12345678-1234-1234-1234-123456789abc');
  assert.equal(JSON.stringify(f.diagnostics).includes(privateValue), false);
  assert.deepEqual(Object.keys(event[1]).sort(), [
    'code',
    'kind',
    'logout_in_progress',
    'phase',
    'requestId',
    'session_present',
    'status',
  ]);
  f.seed();
  f.failDiagnostics();
  await logoutService(f, async () => ({})).logout();
  assert.equal(f.session.hasSession(), false);
});

for (const serverFailure of [false, true]) {
  test(`explicit logout clears persisted session even when server failure=${serverFailure}`, async () => {
    const f = fixture();
    f.seed();
    const expectedRefresh = f.session.getRefreshToken();
    const service = logoutService(f, async (method, route, body, options) => {
      assert.equal(method, 'post');
      assert.equal(route, '/logout');
      assert.deepEqual(body, { refresh_token: expectedRefresh });
      assert.equal(options.skipAuth, true);
      assert.equal(options.retryAttemptsOverride, 0);
      if (serverFailure) throw new Error('Synthetic logout transport failure');
      return { code: 0 };
    });
    await service.logout();
    assert.equal(f.session.hasSession(), false);
    for (const key of ['auth_session_v1', 'auth_token', 'refresh_token']) {
      assert.equal(f.window.localStorage.getItem(key), null);
    }
  });
}

test('late successful refresh cannot restore a session after explicit logout', async () => {
  const f = fixture();
  f.seed();
  let finishRefresh;
  f.setMode(
    config =>
      new Promise(resolve => {
        finishRefresh = () =>
          resolve({
            status: 200,
            data: { access_token: jwt(3600), refresh_token: 'late-refresh' },
            config,
          });
      })
  );
  const refresh = f.client.get('/private').catch(error => error);
  for (let i = 0; i < 20 && !finishRefresh; i++) await Promise.resolve();
  assert.equal(typeof finishRefresh, 'function');
  await logoutService(f, async () => ({ code: 0 })).logout();
  finishRefresh();
  await refresh;
  assert.equal(f.session.hasSession(), false);
  assert.equal(f.requests.includes('/private'), false);
});

test('expired access token: concurrent reads share one successful refresh', async () => {
  const f = fixture();
  f.seed();
  await Promise.all([f.client.get('/a'), f.client.get('/b'), f.client.get('/c')]);
  assert.equal(f.requests.filter(p => p.includes('refresh-token')).length, 1);
  assert.equal(f.redirects.length, 0);
});

for (const status of [500, 503, 429, 'network']) {
  test(`refresh ${status}: preserve credentials, reject request, allow later recovery`, async () => {
    const f = fixture();
    f.seed();
    const before = f.session.getSession();
    f.setMode(status);
    await assert.rejects(f.client.get('/a'));
    assert.deepEqual(f.session.getSession(), before);
    assert.equal(f.redirects.length, 0);
    f.setMode(200);
    await f.client.get('/b');
    assert.equal(f.session.getRefreshToken(), 'new-refresh');
  });
}

test('malformed refresh response fails without destroying the session', async () => {
  const f = fixture();
  f.seed();
  f.setMode(async config => ({ status: 200, data: {}, config }));
  await assert.rejects(f.client.get('/a'), /missing access_token/);
  assert.equal(f.session.getRefreshToken(), 'refresh');
  assert.equal(f.redirects.length, 0);
});

for (const status of [400, 401, 403]) {
  test(`refresh rejected (${status}): clear session and redirect once across concurrent requests`, async () => {
    const f = fixture();
    f.seed();
    f.setMode(status);
    const results = await Promise.allSettled([f.client.get('/a'), f.client.get('/b')]);
    assert.ok(results.every(result => result.status === 'rejected'));
    assert.equal(f.session.hasSession(), false);
    await assert.rejects(f.client.get('/c'));
    assert.equal(f.redirects.length, 1);
    assert.match(f.redirects[0], /redirect=%2Fconsole%2Fdb%3Fpage%3D2/);
    assert.equal(f.notices.length, 1);
    assert.match(f.notices[0].message, /重新登录/);
  });
}

test('missing session: reject before HTTP, redirect once; public requests still succeed', async () => {
  const f = fixture();
  const results = await Promise.allSettled([f.client.get('/a'), f.client.get('/b')]);
  assert.ok(results.every(result => result.reason.code === 'ERR_AUTH_SESSION_MISSING'));
  assert.equal(f.requests.length, 0);
  assert.equal(f.redirects.length, 1);
  assert.equal(f.notices.length, 1);
  await f.client.get('/public', { skipAuth: true });
  assert.equal(f.requests.length, 1);
});

test('logout in progress: no forced redirect or session-expired notification', async () => {
  const f = fixture();
  f.load('lib/auth/logout-state.ts').setLogoutInProgress(true);
  await assert.rejects(f.client.get('/a'));
  assert.equal(f.redirects.length, 0);
  assert.equal(f.notices.length, 0);
});

test('stale refresh rejection cannot clear a newer login', async () => {
  const f = fixture();
  f.seed();
  f.setMode(async config => {
    f.session.setSession({ accessToken: jwt(3600), refreshToken: 'replacement-session' });
    throw new axios.AxiosError('Unauthorized', 'ERR_BAD_RESPONSE', config, undefined, {
      status: 401,
      data: {},
      config,
    });
  });
  await f.client.get('/a');
  assert.equal(f.session.getRefreshToken(), 'replacement-session');
  assert.equal(f.redirects.length, 0);
});

test('same-tab session removal resets auth and clears active query state', () => {
  const f = fixture();
  const { AuthProvider } = f.load('providers/auth-provider.tsx');
  f.render(() => AuthProvider({ children: null }));
  const cleanups = f.commit();
  f.session.clearSession();
  assert.equal(f.auth.isAuthenticated, false);
  assert.equal(f.sessionClears, 1);
  cleanups.forEach(cleanup => cleanup?.());
});

test('session cleanup retains only the exact public features query and its active observer', async () => {
  const f = fixture();
  const { QueryClient, QueryObserver } = require('@tanstack/react-query');
  const client = new QueryClient({ defaultOptions: { queries: { gcTime: Infinity } } });
  const keys = f.load('hooks/query-keys.ts');
  const features = { enable_email_code_login: true, is_email_setup: true, is_allow_register: true };
  client.setQueryData(keys.SYSTEM_KEYS.features(), features);
  const privateKeys = [
    ['profile'],
    ['workspace', 'A'],
    keys.SYSTEM_KEYS.settings(),
    ['system', 'features', 'private'],
  ];
  privateKeys.forEach(key => client.setQueryData(key, { secret: 'session-bound-test-data' }));
  client.getMutationCache().build(client, { mutationKey: ['create-key'] });
  const observer = new QueryObserver(client, {
    queryKey: keys.SYSTEM_KEYS.features(),
    enabled: false,
  });
  const unsubscribe = observer.subscribe(() => {});
  const originalQuery = client
    .getQueryCache()
    .find({ queryKey: keys.SYSTEM_KEYS.features(), exact: true });
  f.mock('@/lib/query-client', { queryClient: client });
  f.mock('@/hooks/query-keys', keys);
  let cachesCleared = 0;
  let workspaces = ['A'];
  let organizations = ['org-A'];
  f.mock('@/utils/client-cache', { clearAuthClientCaches: () => cachesCleared++ });
  f.mock('@/store/workspace-store', {
    useWorkspaceStore: {
      getState: () => ({
        setWorkspaces: value => {
          workspaces = value;
        },
        enterOrganizationMode() {},
      }),
    },
  });
  f.mock('@/store/organization-store', {
    useOrganizationStore: {
      getState: () => ({
        setOrganizations: value => {
          organizations = value;
        },
        setCurrentOrganization() {},
        setSwitchingOrganization() {},
      }),
    },
  });
  try {
    const { clearSessionBoundClientState } = f.load('lib/auth/client-state.ts');
    await clearSessionBoundClientState();
    await clearSessionBoundClientState(); // Duplicate sign-out notification from another tab.
    assert.equal(
      client.getQueryCache().find({ queryKey: keys.SYSTEM_KEYS.features(), exact: true }),
      originalQuery
    );
    assert.equal(originalQuery.getObserversCount(), 1);
    assert.deepEqual(observer.getCurrentResult().data, features);
    privateKeys.forEach(key => assert.equal(client.getQueryData(key), undefined));
    assert.equal(client.getMutationCache().getAll().length, 0);
    assert.equal(workspaces.length, 0);
    assert.equal(organizations.length, 0);
    assert.equal(cachesCleared, 2);
    const fresh = { ...features, is_allow_register: false };
    await client.fetchQuery({ queryKey: keys.SYSTEM_KEYS.features(), queryFn: async () => fresh });
    assert.deepEqual(
      observer.getCurrentResult().data,
      fresh,
      'the retained query must still receive refreshed public configuration'
    );
    await client.resetQueries({ queryKey: keys.SYSTEM_KEYS.features(), exact: true });
    let finishFeatures;
    const pendingFeatures = client.fetchQuery({
      queryKey: keys.SYSTEM_KEYS.features(),
      queryFn: () =>
        new Promise(resolve => {
          finishFeatures = resolve;
        }),
    });
    await clearSessionBoundClientState();
    assert.equal(
      client.getQueryCache().find({ queryKey: keys.SYSTEM_KEYS.features(), exact: true }),
      originalQuery
    );
    finishFeatures(features);
    await pendingFeatures;
    assert.deepEqual(
      observer.getCurrentResult().data,
      features,
      'a late sign-out must not cancel initial login configuration loading'
    );
  } finally {
    unsubscribe();
    client.clear();
  }
});

test('auth reset removes private state without losing known public login features', () => {
  const f = fixture();
  f.mock('@/services/auth.service', { AuthenticationService: class {} });
  f.mock('@/utils/client-cache', { clearAuthClientCaches() {} });
  f.mock('@/lib/auth/context-sync', { syncAccountContextStores() {} });
  const { useAuthStore } = f.load('store/auth-store.ts');
  const features = { enable_email_code_login: true, is_email_setup: true, is_allow_register: true };
  useAuthStore.getState().setSystemFeatures(features);
  useAuthStore.getState().setUser({ id: 'private-member' });
  useAuthStore.getState().reset({ clearSession: false });
  const state = useAuthStore.getState();
  assert.equal(state.user, null);
  assert.equal(state.isAuthenticated, false);
  assert.equal(state.sessionStatus, 'guest');
  assert.equal(state.systemFeatures, features);
});

for (const entry of ['refreshProfile', 'initializeAuth']) {
  for (const failure of ['unauthorized', 'network', 'generic']) {
    test(`${entry}: late ${failure} from an old account cannot mutate the new login`, async () => {
      const f = fixture();
      let rejectProfile;
      f.mock('@/services/auth.service', {
        AuthenticationService: class {
          getToken() {
            return f.session.getAccessToken();
          }
          getProfile() {
            return new Promise((_resolve, reject) => {
              rejectProfile = reject;
            });
          }
          async getSystemFeatures() {
            return {};
          }
        },
      });
      f.mock('@/utils/client-cache', { clearAuthClientCaches() {} });
      f.mock('@/lib/auth/context-sync', { syncAccountContextStores() {} });
      const { useAuthStore } = f.load('store/auth-store.ts');
      f.session.setSession({
        accessToken: 'old-account-access',
        refreshToken: 'old-account-refresh',
      });
      useAuthStore.getState().setUser({ id: 'old-account' });
      const pending = useAuthStore
        .getState()
        [entry]({ force: true })
        .catch(error => error);
      f.session.setSession({
        accessToken: 'new-account-access',
        refreshToken: 'new-account-refresh',
      });
      useAuthStore.getState().setUser({ id: 'new-account' });
      useAuthStore.getState().setLoading(true);
      const error =
        failure === 'unauthorized'
          ? new axios.AxiosError('Unauthorized', 'ERR_BAD_RESPONSE', undefined, undefined, {
              status: 401,
            })
          : failure === 'network'
            ? new axios.AxiosError('Network Error', 'ERR_NETWORK')
            : new Error('old profile failed');
      rejectProfile(error);
      await pending;
      const state = useAuthStore.getState();
      assert.equal(state.user?.id, 'new-account');
      assert.equal(state.isAuthenticated, true);
      assert.equal(state.sessionStatus, 'authenticated');
      assert.equal(state.isLoading, true, 'old initializer cannot finish a newer bootstrap');
      assert.equal(state.error, null);
      assert.equal(state.networkError, false);
      assert.equal(f.session.getAccessToken(), 'new-account-access');
      assert.equal(f.notices.length, 0);
    });
  }
}

test('initialization finishes loading when its own token refresh rotates credentials', async () => {
  const f = fixture();
  f.mock('@/services/auth.service', {
    AuthenticationService: class {
      getToken() {
        return f.session.getAccessToken();
      }
      async getProfile() {
        f.session.setSession(
          { accessToken: 'rotated-access', refreshToken: 'rotated-refresh' },
          { type: 'TOKEN_REFRESHED' }
        );
        return { id: 'same-account' };
      }
      async getSystemFeatures() {
        return {};
      }
    },
  });
  f.mock('@/utils/client-cache', { clearAuthClientCaches() {} });
  f.mock('@/lib/auth/context-sync', { syncAccountContextStores() {} });
  const { useAuthStore } = f.load('store/auth-store.ts');
  f.session.setSession({ accessToken: 'original-access', refreshToken: 'original-refresh' });
  await useAuthStore.getState().initializeAuth({ force: true });
  assert.equal(useAuthStore.getState().isLoading, false);
  assert.equal(f.session.getAccessToken(), 'rotated-access');
  assert.equal(f.notices.length, 0);
});

test('a current-session profile 401 still invalidates that session', async () => {
  const f = fixture();
  f.mock('@/services/auth.service', {
    AuthenticationService: class {
      getToken() {
        return f.session.getAccessToken();
      }
      async getProfile() {
        throw new axios.AxiosError('Unauthorized', 'ERR_BAD_RESPONSE', undefined, undefined, {
          status: 401,
        });
      }
    },
  });
  f.mock('@/utils/client-cache', { clearAuthClientCaches() {} });
  f.mock('@/lib/auth/context-sync', { syncAccountContextStores() {} });
  const { useAuthStore } = f.load('store/auth-store.ts');
  f.session.setSession({ accessToken: 'invalid-access', refreshToken: 'invalid-refresh' });
  await assert.rejects(useAuthStore.getState().refreshProfile());
  assert.equal(f.session.hasSession(), false);
  assert.equal(useAuthStore.getState().user, null);
  assert.equal(f.notices.length, 1);
});

test('SSE missing session uses the same redirect and localized error handling', async () => {
  const f = fixture();
  let error;
  await f.client.sse('/stream', {
    onError: value => {
      error = value;
    },
  });
  assert.equal(error.code, 'ERR_AUTH_SESSION_MISSING');
  assert.equal(f.redirects.length, 1);
  assert.match(f.load('utils/error-notifications.ts').getErrorMessage(error), /重新登录/);
});

for (const inProgress of [null, 'redirect', 'logout']) {
  test(`protected route: ${inProgress ?? 'normal'} navigation does not duplicate auth redirects`, () => {
    const f = fixture();
    f.auth.isAuthenticated = false;
    const state = f.load('lib/auth/logout-state.ts');
    if (inProgress === 'redirect') state.markAuthRedirectInProgress();
    if (inProgress === 'logout') state.setLogoutInProgress(true);
    const { ProtectedRoute } = f.load('components/auth/protected-route.tsx');
    f.render(() => ProtectedRoute({ children: null }));
    f.commit();
    assert.equal(f.redirects.length, inProgress ? 0 : 1);
  });
}

for (const [file, name, args] of [
  ['use-dbs', 'useDbsBasic', [{}]],
  ['use-db-tables', 'useDbTables', ['database']],
]) {
  test(`${name}: notify after commit only, once per error; silence auth and cancellation`, () => {
    const f = fixture();
    const hook = f.load(`hooks/db/${file}.ts`)[name];
    f.setError(new Error('Request failed'));
    f.render(() => hook(...args));
    assert.equal(f.notices.length, 0, 'render must have no toast side effect');
    f.commit();
    assert.equal(f.notices.length, 1);
    f.render(() => hook(...args));
    f.commit();
    assert.equal(f.notices.length, 1, 'unchanged error must not notify again');
    for (const code of ['ERR_AUTH_SESSION_MISSING', 'ERR_CANCELED']) {
      f.setError(Object.assign(new Error('suppressed'), { code }));
      f.render(() => hook(...args));
      f.commit();
      assert.equal(f.notices.length, 1);
    }
    f.setError(new Error('Another failure'));
    f.render(() => hook(...args));
    f.commit();
    assert.equal(f.notices.length, 2);
  });
}
