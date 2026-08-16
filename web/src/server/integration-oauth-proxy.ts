const MAX_REQUEST_BODY_BYTES = 1024 * 1024;
const UPSTREAM_TIMEOUT_MS = 30_000;

interface IntegrationFlowProxyDefinition {
  upstreamPrefix: string;
  errorCodePrefix: string;
  label: string;
  serviceLabel: string;
  allowCallback: boolean;
}

const oauthProxyDefinition: IntegrationFlowProxyDefinition = {
  upstreamPrefix: '/console/api/integrations/oauth',
  errorCodePrefix: 'integration_oauth_proxy',
  label: 'OAuth',
  serviceLabel: 'Integration OAuth',
  allowCallback: true,
};

const providerInstallProxyDefinition: IntegrationFlowProxyDefinition = {
  upstreamPrefix: '/console/api/integrations/provider-install',
  errorCodePrefix: 'integration_provider_install_proxy',
  label: 'Provider installation',
  serviceLabel: 'Integration provider installation',
  allowCallback: false,
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
  if (!validIntegrationFlowProxyRoute(method, path, definition.allowCallback)) {
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
    return new Response(upstream.body, {
      status: upstream.status,
      statusText: upstream.statusText,
      headers: buildResponseHeaders(upstream),
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
