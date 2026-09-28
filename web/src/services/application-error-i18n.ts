import { getCurrentLocale } from '@/lib/i18n';
import zhHans from '@/i18n/modules/common/zh-Hans';
import enUS from '@/i18n/modules/common/en-US';

/**
 * Application (Pre) error codes shown in streaming and timeline surfaces.
 * The browser only ever sees Pre's projected code; this module resolves the
 * matching localized title and action hint from the common error catalog.
 */

// Codes the catalog marks retryable. The server-provided retryable flag wins
// when present; this keeps local rendering correct while older responses omit it.
export const RETRYABLE_APPLICATION_ERROR_CODES = new Set<string>([
  'billing.prededuct.failed',
  'billing.settle.failed',
  'llm.provider.none_available',
  'llm.provider.unavailable',
  'llm.invocation.failed',
  'llm.provider.timeout',
  'llm.provider.rate_limited',
  'llm.rate_limit.exceeded',
  'video.task.failed',
  'video.upstream.timeout',
  'video.runtime.unavailable',
  'video.asset.save_failed',
  'image.task.timeout',
  'image.upstream.failed',
  'image.asset.save_failed',
  'image.runtime.failed',
  'music.task.failed',
]);

const APPLICATION_CODE_PREFIXES = ['billing.', 'llm.', 'video.', 'image.', 'music.'];

export interface ApplicationErrorCopy {
  title: string;
  action: string;
  retryable: boolean;
}

interface CodeMessage {
  title: string;
  action: string;
}

function codeMessages(): Record<string, CodeMessage | undefined> {
  const messages = getCurrentLocale() === 'en-US' ? enUS : zhHans;
  return messages.requestErrors.codes as Record<string, CodeMessage | undefined>;
}

function recordValue(value: unknown, key: string): unknown {
  return value && typeof value === 'object' ? (value as Record<string, unknown>)[key] : undefined;
}

function stringValue(value: unknown): string | null {
  return typeof value === 'string' && value.trim().length > 0 ? value.trim() : null;
}

/**
 * Extract a stable application code from a raw value (a string) or an axios
 * error-like object. It never reads the raw message.
 */
export function applicationErrorCodeFromValue(value: unknown): string | null {
  if (typeof value === 'string') {
    const normalized = value.trim();
    return normalized.length > 0 ? normalized : null;
  }
  const data = recordValue(recordValue(value, 'response'), 'data');
  const nestedError = recordValue(data, 'error');
  return (
    stringValue(recordValue(data, 'code')) ??
    stringValue(recordValue(data, 'error_code')) ??
    stringValue(recordValue(nestedError, 'code')) ??
    stringValue(recordValue(value, 'code'))
  );
}

export function isApplicationErrorCode(value: unknown): boolean {
  const code = applicationErrorCodeFromValue(value);
  return code ? APPLICATION_CODE_PREFIXES.some(prefix => code.startsWith(prefix)) : false;
}

/**
 * Resolve localized copy for an application code. Returns null for unknown
 * codes so callers can fall back without echoing raw text.
 */
export function applicationErrorCopy(code: string): ApplicationErrorCopy | null {
  const normalizedCode = code.replace(/\./g, '__');
  const entry = codeMessages()[normalizedCode];
  if (!entry) {
    return null;
  }
  return {
    title: entry.title,
    action: entry.action,
    retryable: RETRYABLE_APPLICATION_ERROR_CODES.has(code),
  };
}
