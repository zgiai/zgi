import {
  isSensitiveReporterKey,
  sanitizeReporterRecord,
  sanitizeReporterString,
  sanitizeReporterUrl,
} from './sanitize';

interface SentryEventLike {
  message?: string;
  tags?: Record<string, unknown>;
  extra?: Record<string, unknown>;
  contexts?: Record<string, Record<string, unknown> | undefined>;
  request?: {
    url?: string;
    data?: unknown;
    cookies?: unknown;
    query_string?: unknown;
    headers?: Record<string, unknown>;
  };
  user?: { id?: string; [key: string]: unknown };
  breadcrumbs?: Array<
    | {
        message?: string;
        data?: Record<string, unknown>;
      }
    | undefined
  >;
  exception?: { values?: Array<{ value?: string } | undefined> };
}

const ALLOWED_REQUEST_HEADERS = new Set([
  'accept',
  'content-length',
  'content-type',
  'user-agent',
  'x-request-id',
]);

/** Applies the ZGI privacy boundary to automatic and adapter-created Sentry events. */
export function sanitizeSentryEvent<T>(event: T): T {
  if (!event || typeof event !== 'object') return event;
  const mutable = event as SentryEventLike;

  if (mutable.message) mutable.message = sanitizeReporterString(mutable.message);
  mutable.tags = sanitizeReporterRecord(mutable.tags);
  mutable.extra = sanitizeReporterRecord(mutable.extra);

  if (mutable.contexts) {
    for (const [name, context] of Object.entries(mutable.contexts)) {
      mutable.contexts[name] = sanitizeReporterRecord(context);
    }
  }

  for (const breadcrumb of mutable.breadcrumbs || []) {
    if (!breadcrumb) continue;
    if (breadcrumb.message) breadcrumb.message = sanitizeReporterString(breadcrumb.message);
    breadcrumb.data = sanitizeReporterRecord(breadcrumb.data);
  }

  for (const exception of mutable.exception?.values || []) {
    if (exception?.value) exception.value = sanitizeReporterString(exception.value);
  }

  if (mutable.request) {
    if (mutable.request.url) {
      mutable.request.url = sanitizeReporterUrl(sanitizeReporterString(mutable.request.url));
    }
    delete mutable.request.data;
    delete mutable.request.cookies;
    delete mutable.request.query_string;
    const headers: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(mutable.request.headers || {})) {
      if (ALLOWED_REQUEST_HEADERS.has(key.toLowerCase()) && !isSensitiveReporterKey(key)) {
        headers[key] = typeof value === 'string' ? sanitizeReporterString(value) : value;
      }
    }
    mutable.request.headers = headers;
  }

  mutable.user = mutable.user?.id ? { id: mutable.user.id } : undefined;
  return event;
}
