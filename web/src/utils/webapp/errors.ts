export const WEBAPP_OFFLINE_ERROR_CODE = '204008';
export const WEBAPP_OFFLINE_EVENT = 'webapp:offline';

interface ErrorLikeRecord {
  code?: unknown;
  errorCode?: unknown;
  message?: unknown;
  businessError?: {
    code?: unknown;
    message?: unknown;
  };
  response?: {
    data?: {
      code?: unknown;
      errorCode?: unknown;
      message?: unknown;
      errorMessage?: unknown;
    };
  };
}

/**
 * @util getWebAppErrorCode
 * @description Extracts backend business codes from Axios errors, SSE errors, and raw payloads.
 */
export function getWebAppErrorCode(error: unknown): string | undefined {
  if (!error || typeof error !== 'object') return undefined;

  const record = error as ErrorLikeRecord;
  const code =
    record.businessError?.code ??
    record.response?.data?.code ??
    record.response?.data?.errorCode ??
    record.code ??
    record.errorCode;

  return typeof code === 'string' || typeof code === 'number' ? String(code) : undefined;
}

/**
 * @util isWebAppOfflineError
 * @description Identifies the public WebApp offline business error.
 */
export function isWebAppOfflineError(error: unknown): boolean {
  return getWebAppErrorCode(error) === WEBAPP_OFFLINE_ERROR_CODE;
}

/**
 * @util emitWebAppOffline
 * @description Broadcasts that the current public WebApp became unavailable.
 */
export function emitWebAppOffline(): void {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new CustomEvent(WEBAPP_OFFLINE_EVENT));
}
