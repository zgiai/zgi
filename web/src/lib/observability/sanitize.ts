import type { ZGIEvent } from './types';

const REDACTED = '[REDACTED]';
const TRUNCATED = '[TRUNCATED]';
const MAX_STRING_LENGTH = 8192;
const MAX_DEPTH = 6;
const MAX_ITEMS = 64;

const SENSITIVE_VALUE_PATTERNS = [
  /\b(?:bearer|basic)\s+[a-z0-9._~+/=-]+/gi,
  /\bsk-[a-z0-9_-]{8,}\b/gi,
  /\b(?:api[_-]?key|access_token|refresh_token|password|secret)=[^\s&]+/gi,
];
const ABSOLUTE_URL_PATTERN = /https?:\/\/[^\s]+/gi;

const SENSITIVE_KEYS = [
  'password',
  'passwd',
  'secret',
  'api_key',
  'apikey',
  'authorization',
  'cookie',
  'credential',
  'private_key',
  'access_token',
  'refresh_token',
  'ip_address',
  'ipaddress',
  'client_ip',
  'x_forwarded_for',
  'x_real_ip',
  'remote_addr',
  'node_inputs',
  'prompt',
  'request_body',
  'response_body',
  'response_text',
  'raw_sql',
];

function normalizeKey(key: string): string {
  return key.toLowerCase().replaceAll('-', '_').replaceAll('.', '_');
}

export function isSensitiveReporterKey(key: string): boolean {
  const normalized = normalizeKey(key);
  return normalized === 'sql' || SENSITIVE_KEYS.some(item => normalized.includes(item));
}

function isReporterUrlKey(key: string): boolean {
  const normalized = normalizeKey(key);
  return normalized === 'url' || normalized.endsWith('_url');
}

export function sanitizeReporterUrl(value: string): string {
  const separator = value.search(/[?#]/);
  return separator >= 0 ? value.slice(0, separator) : value;
}

export function sanitizeReporterString(value: string): string {
  const redactedSecrets = SENSITIVE_VALUE_PATTERNS.reduce(
    (current, pattern) => current.replace(pattern, REDACTED),
    value
  );
  const redacted = redactedSecrets.replace(ABSOLUTE_URL_PATTERN, sanitizeReporterUrl);
  return redacted.length <= MAX_STRING_LENGTH
    ? redacted
    : `${redacted.slice(0, MAX_STRING_LENGTH)}...${TRUNCATED}`;
}

function sanitizeValue(value: unknown, depth: number): unknown {
  if (depth >= MAX_DEPTH) return TRUNCATED;
  if (typeof value === 'string') return sanitizeReporterString(value);
  if (value === null || typeof value !== 'object') return value;
  if (Array.isArray(value)) {
    return value.slice(0, MAX_ITEMS).map(item => sanitizeValue(item, depth + 1));
  }
  if (value instanceof Date) return value.toISOString();
  if (value instanceof Error) return sanitizeReporterString(value.message);
  return sanitizeReporterRecord(value as Record<string, unknown>, depth + 1);
}

export function sanitizeReporterRecord(
  values: Record<string, unknown> | undefined,
  depth = 0
): Record<string, unknown> | undefined {
  if (!values) return undefined;
  const result: Record<string, unknown> = {};
  const entries = Object.entries(values);
  for (const [key, value] of entries.slice(0, MAX_ITEMS)) {
    if (!key) continue;
    if (isSensitiveReporterKey(key)) {
      result[key] = REDACTED;
    } else if (typeof value === 'string' && isReporterUrlKey(key)) {
      result[key] = sanitizeReporterUrl(sanitizeReporterString(value));
    } else {
      result[key] = sanitizeValue(value, depth);
    }
  }
  if (entries.length > MAX_ITEMS) result['zgi.truncated'] = true;
  return result;
}

function sanitizeReporterError(error: unknown): unknown {
  if (!(error instanceof Error)) return sanitizeValue(error, 0);

  const sanitized = new Error(sanitizeReporterString(error.message));
  sanitized.name = sanitizeReporterString(error.name);
  if (error.stack) sanitized.stack = sanitizeReporterString(error.stack);
  return sanitized;
}

export function sanitizeZGIEvent(event: ZGIEvent): ZGIEvent {
  const rawTags = sanitizeReporterRecord(event.tags as Record<string, unknown> | undefined);
  const tags: Record<string, string | number | boolean> = {};
  for (const [key, value] of Object.entries(rawTags || {})) {
    if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
      tags[key] = value;
    }
  }

  return {
    ...event,
    name: sanitizeReporterString(event.name.trim() || 'zgi.event'),
    error: event.error === undefined ? undefined : sanitizeReporterError(event.error),
    tags: Object.keys(tags).length > 0 ? tags : undefined,
    attributes: sanitizeReporterRecord(event.attributes),
  };
}
