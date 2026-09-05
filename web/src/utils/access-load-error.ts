export type AccessLoadErrorKind =
  | 'session'
  | 'forbidden'
  | 'notFound'
  | 'server'
  | 'network'
  | 'unknown';

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === 'object'
    ? (value as Record<string, unknown>)
    : undefined;
}

// Never render raw error messages, response bodies or request headers. They can
// contain credentials and internal database/provider details.
export function describeAccessLoadError(error: unknown): {
  kind: AccessLoadErrorKind;
  status?: number;
  code?: string;
  requestId?: string;
} {
  const source = record(error);
  const response = record(source?.response);
  const data = record(response?.data);
  const headers = record(response?.headers);
  const rawStatus = response?.status;
  const status =
    typeof rawStatus === 'number' &&
    Number.isInteger(rawStatus) &&
    rawStatus >= 100 &&
    rawStatus <= 599
      ? rawStatus
      : undefined;
  const rawCode = data?.code;
  const code =
    (typeof rawCode === 'string' || typeof rawCode === 'number') &&
    /^\d{1,12}$/.test(String(rawCode))
      ? String(rawCode)
      : undefined;
  const rawRequestId = headers?.['x-request-id'];
  const requestId =
    typeof rawRequestId === 'string' &&
    /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i.test(rawRequestId)
      ? rawRequestId
      : undefined;

  let kind: AccessLoadErrorKind = 'unknown';
  if (
    status === 401 ||
    (status === 400 && code === '212012') ||
    source?.code === 'ERR_AUTH_SESSION_MISSING'
  ) {
    kind = 'session';
  } else if (status === 403) {
    kind = 'forbidden';
  } else if (status === 404) {
    kind = 'notFound';
  } else if (status !== undefined && status >= 500) {
    kind = 'server';
  } else if (
    !response &&
    ['ERR_NETWORK', 'NETWORK_ERROR', 'ECONNABORTED', 'ETIMEDOUT'].includes(String(source?.code))
  ) {
    kind = 'network';
  }
  return { kind, status, code, requestId };
}

const PERMISSION_CLIENT_CODES = new Set([
  'ERR_NETWORK',
  'NETWORK_ERROR',
  'ECONNABORTED',
  'ETIMEDOUT',
  'ERR_CANCELED',
  'ERR_AUTH_SESSION_MISSING',
  'ERR_AUTH_REFRESH_STALE',
  'ERR_BUSINESS',
  'ERR_BAD_REQUEST',
  'ERR_BAD_RESPONSE',
  'ERR_PERMISSION_RESPONSE_EMPTY',
]);

export function describePermissionLoadError(error: unknown) {
  const source = record(error);
  const clientCode =
    typeof source?.code === 'string' && PERMISSION_CLIENT_CODES.has(source.code)
      ? source.code
      : 'unknown';
  return { ...describeAccessLoadError(error), clientCode };
}

/** Observe the failed read without logging the Axios request, body or message. */
export async function observePermissionLoad<T>(
  load: () => Promise<T>,
  report: (diagnostic: ReturnType<typeof describePermissionLoadError>) => void
): Promise<T> {
  try {
    const result = await load();
    if (result === undefined) {
      // React Query rejects undefined data too; make this boundary diagnosable.
      throw Object.assign(new Error('Workspace permissions response is missing'), {
        code: 'ERR_PERMISSION_RESPONSE_EMPTY',
      });
    }
    return result;
  } catch (error) {
    try {
      report(describePermissionLoadError(error));
    } catch {
      // An unavailable reporter must not replace the original request error.
    }
    throw error;
  }
}
