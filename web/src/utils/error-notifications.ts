/**
 * Simplified error notification utilities for HTTP client.
 * Renders cataloged Pre application codes with localized title, action hint,
 * error_id, and a retry entry when the code is retryable.
 */
import { toast, type Action } from 'sonner';
import { AxiosError } from 'axios';
import type { ReactNode } from 'react';
import { getCurrentLocale } from '@/lib/i18n';
import zhHans from '@/i18n/modules/common/zh-Hans';
import enUS from '@/i18n/modules/common/en-US';
import {
  applicationErrorCodeFromValue,
  applicationErrorCopy,
  isApplicationErrorCode,
} from '@/services/application-error-i18n';

const isClient = (): boolean => typeof window !== 'undefined';
const getIsZhHans = (): boolean => getCurrentLocale() === 'zh-Hans';

export type ToastVariant = 'default' | 'destructive' | 'success' | 'warning' | 'info';

interface ToastInput {
  title: ReactNode;
  description?: ReactNode;
  variant?: ToastVariant;
  duration?: number;
  id?: string | number;
  action?: Action;
}

interface NormalizeToastDescriptionOptions {
  duplicateDescriptions?: Array<ReactNode | null | undefined>;
}

const normalizeComparableToastText = (value: ReactNode | null | undefined): string | null => {
  if (value === null || value === undefined || typeof value === 'boolean') {
    return null;
  }

  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'bigint') {
    const text = String(value).trim().replace(/\s+/g, ' ');
    return text.length > 0 ? text.toLowerCase() : null;
  }

  return null;
};

/**
 * Hide toast descriptions that are empty or repeat the title.
 */
export function normalizeToastDescription(
  title: ReactNode,
  description: ReactNode | null | undefined,
  options: NormalizeToastDescriptionOptions = {}
): ReactNode | undefined {
  const descriptionText = normalizeComparableToastText(description);
  if (!descriptionText) {
    return undefined;
  }

  const titleText = normalizeComparableToastText(title);
  if (titleText && descriptionText === titleText) {
    return undefined;
  }

  const duplicates = options.duplicateDescriptions ?? [];
  const hasDuplicate = duplicates.some(candidate => {
    const candidateText = normalizeComparableToastText(candidate);
    return Boolean(candidateText && candidateText === descriptionText);
  });

  return hasDuplicate ? undefined : description;
}

const safeToast = (props: ToastInput): void => {
  if (!isClient()) return;

  const { title, variant, description, duration, id, action } = props;
  const shared = { description, duration, id, action };

  switch (variant) {
    case 'success':
      toast.success(title, shared);
      break;
    case 'destructive':
      toast.error(title, shared);
      break;
    case 'warning':
      toast.warning(title, shared);
      break;
    case 'info':
      toast.info(title, shared);
      break;
    default:
      toast(title, shared);
  }
};

// Error types for categorization
export enum ErrorType {
  NETWORK = 'network',
  AUTH = 'auth',
  PERMISSION = 'permission',
  VALIDATION = 'validation',
  SERVER = 'server',
  UNKNOWN = 'unknown',
}

export interface ErrorInfo {
  status?: number;
  message?: string;
  data?: unknown;
  url?: string;
  method?: string;
  type?: ErrorType;
}

/**
 * ApplicationErrorPresentation is the code-first rendering of a Pre API error.
 * The browser consumes Pre's projected code, never the internal ErrorInfo or
 * the raw transport text.
 */
export interface ApplicationErrorPresentation {
  code?: string;
  title: string;
  description?: string;
  errorId?: string;
  retryable: boolean;
}

function getCommonMessages() {
  return getIsZhHans() ? zhHans : enUS;
}

function recordValue(value: unknown, key: string): unknown {
  return value && typeof value === 'object' ? (value as Record<string, unknown>)[key] : undefined;
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim().length > 0 ? value.trim() : undefined;
}

function extractErrorId(error: unknown): string | undefined {
  const data = recordValue(recordValue(error, 'response'), 'data');
  const nestedError = recordValue(data, 'error');
  return (
    stringValue(recordValue(data, 'error_id')) ??
    stringValue(recordValue(nestedError, 'error_id'))
  );
}

function extractRetryable(error: unknown): boolean {
  const data = recordValue(recordValue(error, 'response'), 'data');
  return recordValue(data, 'retryable') === true;
}

function isNetworkFailure(error: unknown): boolean {
  if (!(error instanceof AxiosError)) {
    return false;
  }
  if (error.response) {
    return error.response.status === 0;
  }
  return (
    error.code === 'NETWORK_ERROR' ||
    error.code === 'ERR_NETWORK' ||
    error.code === 'ECONNABORTED' ||
    error.message === 'Network Error'
  );
}

/**
 * Get the code-first presentation for an error, or null when the error has no
 * cataloged code and no network/status classification. Callers must not echo
 * the raw backend message for a recognized product domain.
 */
export function getApplicationErrorPresentation(error: unknown): ApplicationErrorPresentation | null {
  if (isAuthSessionError(error)) {
    return { title: getCommonMessages().requestErrors.sessionExpired, retryable: false };
  }

  const code = applicationErrorCodeFromValue(error) ?? undefined;
  const errorId = extractErrorId(error);
  const serverRetryable = extractRetryable(error);
  const copy = code ? applicationErrorCopy(code) : null;
  if (code && copy) {
    return {
      code,
      title: copy.title,
      description: copy.action,
      errorId,
      retryable: serverRetryable || copy.retryable,
    };
  }

  if (code && isApplicationErrorCode(code)) {
    // Known product domain, unknown code: render a safe generic title instead
    // of leaking the backend message.
    return { code, title: getCommonMessages().requestErrors.generic, errorId, retryable: serverRetryable };
  }

  if (isNetworkFailure(error)) {
    return { title: getNetworkErrorMessage(), errorId, retryable: false };
  }

  return null;
}

function formatApplicationError(presentation: ApplicationErrorPresentation): string {
  const parts = [presentation.title];
  if (presentation.description) {
    parts.push(presentation.description);
  }
  if (presentation.errorId) {
    parts.push(`${getCommonMessages().requestErrors.errorId}: ${presentation.errorId}`);
  }
  return parts.join('\n');
}

/**
 * Get network error message based on current locale
 */
function getNetworkErrorMessage(): string {
  return getIsZhHans() ? zhHans.networkError : enUS.networkError;
}

function normalizeBackendMessage(
  message: string | null | undefined,
  status?: number
): string | null {
  const commonMessages = getCommonMessages();
  const normalized = message?.trim().toLowerCase();

  if (!normalized) {
    if (status === 401) return commonMessages.requestErrors.sessionExpired;
    if (status === 403) return commonMessages.requestErrors.forbidden;
    if (status === 404) return commonMessages.requestErrors.notFound;
    if (status === 429) return commonMessages.requestErrors.rateLimited;
    if (status && status >= 500) return commonMessages.requestErrors.serverBusy;
    return commonMessages.requestErrors.generic;
  }

  if (normalized.includes('password validation failed')) {
    return commonMessages.requestErrors.passwordValidation;
  }
  if (normalized.includes('network error')) {
    return commonMessages.networkError;
  }
  if (normalized.includes('internal server error')) {
    return commonMessages.requestErrors.serverBusy;
  }
  if (normalized.includes('timeout')) {
    return commonMessages.requestErrors.timeout;
  }
  if (normalized.includes('forbidden') || normalized.includes('access denied')) {
    return commonMessages.requestErrors.forbidden;
  }
  if (normalized.includes('not found')) {
    return commonMessages.requestErrors.notFound;
  }

  return message as string;
}

const NETWORK_ERROR_TOAST_ID = 'network-error';
const NETWORK_ERROR_DEDUPE_MS = 5_000;
let lastNetworkErrorAt = 0;

export class ErrorNotificationService {
  static showSessionExpired(): void {
    safeToast({
      title: getCommonMessages().requestErrors.sessionExpired,
      variant: 'destructive',
      duration: 5000,
      id: 'auth-session-expired',
    });
  }

  /**
   * Show network error with i18n support
   */
  static showNetworkError(): void {
    if (!isClient()) return;

    const now = Date.now();
    if (now - lastNetworkErrorAt < NETWORK_ERROR_DEDUPE_MS) {
      return;
    }
    lastNetworkErrorAt = now;

    safeToast({
      title: getNetworkErrorMessage(),
      variant: 'destructive',
      duration: 5000,
      id: NETWORK_ERROR_TOAST_ID,
    });
  }

  /**
   * Show validation error (used by icon-input component)
   */
  static showValidationError(errorInfo: ErrorInfo): void {
    if (!isClient()) return;
    safeToast({
      title: errorInfo.message || 'Validation Error',
      variant: 'destructive',
      duration: 5000,
    });
  }

  /**
   * Show generic error with custom message
   */
  static showError(message: string): void {
    if (!isClient()) return;
    safeToast({
      title: message,
      variant: 'destructive',
      duration: 5000,
    });
  }

  /**
   * Show a code-first API error toast. The title and action hint come from the
   * catalog; error_id is shown for support and a retry action is offered when
   * the code is retryable.
   */
  static showApiError(
    error: unknown,
    options: { onRetry?: () => void; id?: string | number } = {}
  ): void {
    if (!isClient()) return;

    const presentation = getApplicationErrorPresentation(error);
    const title = presentation?.title || getCommonMessages().requestErrors.generic;
    const descriptionLines: string[] = [];
    if (presentation?.description) {
      descriptionLines.push(presentation.description);
    }
    if (presentation?.errorId) {
      descriptionLines.push(
        `${getCommonMessages().requestErrors.errorId}: ${presentation.errorId}`
      );
    }
    const action =
      presentation?.retryable && options.onRetry
        ? {
            label: getCommonMessages().requestErrors.retry,
            onClick: options.onRetry,
          }
        : undefined;

    safeToast({
      title,
      description: descriptionLines.length > 0 ? descriptionLines.join('\n') : undefined,
      variant: 'destructive',
      duration: 5000,
      id: options.id,
      action,
    });
  }
}

/**
 * Extract a user-safe error message. Cataloged codes are rendered by code;
 * everything else keeps the legacy network/status/backend-message behavior.
 */
export function getErrorMessage(error: unknown): string {
  const presentation = getApplicationErrorPresentation(error);
  if (presentation) {
    return formatApplicationError(presentation);
  }

  if (error instanceof AxiosError) {
    if (
      error.code === 'NETWORK_ERROR' ||
      error.code === 'ERR_NETWORK' ||
      error.code === 'ECONNABORTED' ||
      error.message === 'Network Error'
    ) {
      return getNetworkErrorMessage();
    }

    const respMsg = (error.response?.data as { message?: string } | undefined)?.message;
    return (
      normalizeBackendMessage(respMsg || error.message, error.response?.status) ||
      getCommonMessages().requestErrors.generic
    );
  }
  if (error instanceof Error) {
    return normalizeBackendMessage(error.message) || getCommonMessages().requestErrors.generic;
  }
  if (typeof error === 'string') {
    return normalizeBackendMessage(error) || getCommonMessages().requestErrors.generic;
  }
  return getCommonMessages().requestErrors.generic;
}

export function isAuthSessionError(error: unknown): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    (error as { code?: string }).code === 'ERR_AUTH_SESSION_MISSING'
  );
}

/**
 * Check if error was caused by user-initiated cancel/abort
 */
export function isCanceledRequestError(error: unknown): boolean {
  if (!error) return false;
  if (typeof error === 'string') {
    return /\babort(?:ed)?\b/i.test(error) || /\bcancell?ed\b/i.test(error);
  }
  const err = error as { code?: string; name?: string; message?: string } | undefined;
  if (!err) return false;
  if (err.code === 'ERR_CANCELED') return true;
  if (err.name === 'CanceledError' || err.name === 'AbortError') return true;
  const msg = err.message || '';
  return /\babort(?:ed)?\b/i.test(msg) || /\bcancell?ed\b/i.test(msg);
}

export const {
  showNetworkError,
  showValidationError,
  showError,
  showApiError,
} = ErrorNotificationService;
