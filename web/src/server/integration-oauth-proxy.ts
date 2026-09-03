const MAX_REQUEST_BODY_BYTES = 1024 * 1024;
const UPSTREAM_TIMEOUT_MS = 30_000;

interface IntegrationFlowProxyDefinition {
  upstreamPrefix: string;
  errorCodePrefix: string;
  label: string;
  serviceLabel: string;
  bufferResponse?: boolean;
  validRoute: (method: string, path: readonly string[]) => boolean;
}

const oauthProxyDefinition: IntegrationFlowProxyDefinition = {
  upstreamPrefix: '/console/api/integrations/oauth',
  errorCodePrefix: 'integration_oauth_proxy',
  label: 'OAuth',
  serviceLabel: 'Integration OAuth',
  validRoute: (method, path) => validIntegrationFlowProxyRoute(method, path, true),
};

const providerInstallProxyDefinition: IntegrationFlowProxyDefinition = {
  upstreamPrefix: '/console/api/integrations/provider-install',
  errorCodePrefix: 'integration_provider_install_proxy',
  label: 'Provider installation',
  serviceLabel: 'Integration provider installation',
  validRoute: (method, path) => validIntegrationFlowProxyRoute(method, path, false),
};

const wecomCallbackProxyDefinition: IntegrationFlowProxyDefinition = {
  upstreamPrefix: '/console/api/integrations/wecom',
  errorCodePrefix: 'integration_wecom_callback_proxy',
  label: 'WeCom callback',
  serviceLabel: 'Integration WeCom callback',
  // WeCom's callback verifier uses a legacy HTTP client. Return a fixed-length
  // response instead of forwarding the upstream stream with chunked encoding.
  bufferResponse: true,
  validRoute: validWeComCallbackProxyRoute,
};

const forwardedRequestHeaders = [
  'accept',
  'accept-language',
  'authorization',
  'content-type',
  'cookie',
  'referer',
  'user-agent',
  'x-request-id',
] as const;

const excludedResponseHeaders = new Set([
  'connection',
  'content-encoding',
  'content-length',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'set-cookie',
  'te',
  'trailer',
  'transfer-encoding',
  'upgrade',
]);

type IntegrationFlowProxyMethod = 'GET' | 'POST';

function proxyError(status: number, code: string, message: string): Response {
  return Response.json(
    { code, message },
    {
      status,
      headers: {
        'cache-control': 'no-store',
      },
    }
  );
}

function validIntegrationFlowProxyRoute(
  method: string,
  path: readonly string[],
  allowCallback: boolean
): boolean {
  if (allowCallback && method === 'GET' && path.length === 1 && path[0] === 'callback') {
    return true;
  }
  if (method === 'POST' && path.length === 1 && path[0] === 'flows') {
    return true;
  }
  if (method === 'GET' && path.length === 2 && path[0] === 'flows' && validOAuthFlowID(path[1])) {
    return true;
  }
  return (
    method === 'POST' &&
    path.length === 3 &&
    path[0] === 'flows' &&
    validOAuthFlowID(path[1]) &&
    path[2] === 'cancel'
  );
}

function validOAuthFlowID(value: string | undefined): boolean {
  return typeof value === 'string' && /^[A-Za-z0-9_-]{32,256}$/.test(value);
}

function validWeComCallbackProxyRoute(method: string, path: readonly string[]): boolean {
  if (method !== 'GET' && method !== 'POST') return false;
  if (path.length === 2 && path[0] === 'install' && path[1] === 'callback') {
    return method === 'GET';
  }
  if (path.length === 2 && path[0] === 'suite' && path[1] === 'events') {
    return true;
  }
  return (
    path.length === 3 &&
    path[0] === 'data' &&
    path[1] === 'events' &&
    /^[A-Za-z0-9_-]{1,128}$/.test(path[2] ?? '')
  );
}

function resolveUpstreamBaseURL(requestURL: URL): URL | null {
  const configured =
    process.env.INTEGRATION_API_INTERNAL_URL?.trim() || process.env.API_URL?.trim() || '';
  if (!configured) return null;

  try {
    const upstream = new URL(configured);
    if (
      (upstream.protocol !== 'http:' && upstream.protocol !== 'https:') ||
      upstream.username ||
      upstream.password ||
      upstream.search ||
      upstream.hash ||
      (upstream.pathname !== '' && upstream.pathname !== '/')
    ) {
      return null;
    }
    if (upstream.origin === requestURL.origin) {
      return null;
    }
    return upstream;
  } catch {
    return null;
  }
}

function buildUpstreamHeaders(request: Request, requestURL: URL): Headers {
  const headers = new Headers();
  for (const name of forwardedRequestHeaders) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  headers.set('x-forwarded-host', requestURL.host);
  headers.set('x-forwarded-proto', requestURL.protocol.replace(':', ''));
  return headers;
}

function buildResponseHeaders(upstream: Response): Headers {
  const headers = new Headers();
  upstream.headers.forEach((value, name) => {
    if (!excludedResponseHeaders.has(name.toLowerCase())) {
      headers.append(name, value);
    }
  });

  const upstreamHeaders = upstream.headers as Headers & {
    getSetCookie?: () => string[];
  };
  const cookies = upstreamHeaders.getSetCookie?.() ?? [];
  if (cookies.length > 0) {
    for (const cookie of cookies) headers.append('set-cookie', cookie);
  } else {
    const cookie = upstream.headers.get('set-cookie');
    if (cookie) headers.append('set-cookie', cookie);
  }
  headers.set('cache-control', 'no-store');
  return headers;
}

async function readRequestBody(request: Request): Promise<ArrayBuffer | undefined> {
  if (request.method === 'GET' || request.method === 'HEAD') return undefined;

  const declaredLength = Number.parseInt(request.headers.get('content-length') ?? '', 10);
  if (Number.isFinite(declaredLength) && declaredLength > MAX_REQUEST_BODY_BYTES) {
    throw new RangeError('request body is too large');
  }
  const body = await request.arrayBuffer();
  if (body.byteLength > MAX_REQUEST_BODY_BYTES) {
    throw new RangeError('request body is too large');
  }
  return body;
}

async function proxyIntegrationFlowRequest(
  request: Request,
  rawPath: readonly string[],
  definition: IntegrationFlowProxyDefinition
): Promise<Response> {
  const method = request.method.toUpperCase() as IntegrationFlowProxyMethod;
  const path = rawPath.map(segment => segment.trim());
  if (!definition.validRoute(method, path)) {
    return proxyError(
      404,
      `${definition.errorCodePrefix}_not_found`,
      `${definition.label} proxy route not found`
    );
  }

  const requestURL = new URL(request.url);
  const upstreamBaseURL = resolveUpstreamBaseURL(requestURL);
  if (!upstreamBaseURL) {
    return proxyError(
      503,
      `${definition.errorCodePrefix}_unavailable`,
      `${definition.serviceLabel} proxy is not configured`
    );
  }

  const upstreamURL = new URL(
    `${definition.upstreamPrefix}/${path.map(encodeURIComponent).join('/')}`,
    upstreamBaseURL
  );
  upstreamURL.search = requestURL.search;

  let body: ArrayBuffer | undefined;
  try {
    body = await readRequestBody(request);
  } catch (error) {
    if (error instanceof RangeError) {
      return proxyError(
        413,
        `${definition.errorCodePrefix}_request_too_large`,
        `${definition.label} proxy request is too large`
      );
    }
    return proxyError(
      400,
      `${definition.errorCodePrefix}_invalid_request`,
      `${definition.label} proxy request is invalid`
    );
  }

  try {
    const upstream = await fetch(upstreamURL, {
      method,
      headers: buildUpstreamHeaders(request, requestURL),
      body,
      cache: 'no-store',
      redirect: 'manual',
      signal: AbortSignal.timeout(UPSTREAM_TIMEOUT_MS),
    });
    const headers = buildResponseHeaders(upstream);
    if (definition.bufferResponse) {
      const responseBody = await upstream.arrayBuffer();
      headers.set('content-length', String(responseBody.byteLength));
      return new Response(responseBody, {
        status: upstream.status,
        statusText: upstream.statusText,
        headers,
      });
    }
    return new Response(upstream.body, {
      status: upstream.status,
      statusText: upstream.statusText,
      headers,
    });
  } catch {
    return proxyError(
      502,
      `${definition.errorCodePrefix}_upstream_unavailable`,
      `${definition.serviceLabel} service is unavailable`
    );
  }
}

export function proxyIntegrationOAuthRequest(
  request: Request,
  rawPath: readonly string[]
): Promise<Response> {
  return proxyIntegrationFlowRequest(request, rawPath, oauthProxyDefinition);
}

export function proxyIntegrationProviderInstallRequest(
  request: Request,
  rawPath: readonly string[]
): Promise<Response> {
  return proxyIntegrationFlowRequest(request, rawPath, providerInstallProxyDefinition);
}

export function proxyIntegrationWeComCallbackRequest(
  request: Request,
  rawPath: readonly string[]
): Promise<Response> {
  return proxyIntegrationFlowRequest(request, rawPath, wecomCallbackProxyDefinition);
}
