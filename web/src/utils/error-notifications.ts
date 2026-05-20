/**
 * Simplified error notification utilities for HTTP client.
 * Only handles network errors with i18n support.
 * Other errors are thrown directly with backend messages.
 */
import { toast } from 'sonner';
import { AxiosError } from 'axios';
import type { ReactNode } from 'react';
import { getCurrentLocale } from '@/lib/i18n';
import zhHans from '@/i18n/modules/common/zh-Hans';
import enUS from '@/i18n/modules/common/en-US';

const isClient = (): boolean => typeof window !== 'undefined';
const getIsZhHans = (): boolean => getCurrentLocale() === 'zh-Hans';

export type ToastVariant = 'default' | 'destructive' | 'success' | 'warning' | 'info';

interface ToastInput {
  title: ReactNode;
  description?: ReactNode;
  variant?: ToastVariant;
  duration?: number;
  id?: string | number;
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

  const { title, variant, description, duration, id } = props;

  switch (variant) {
    case 'success':
      toast.success(title, { description, duration, id });
      break;
    case 'destructive':
      toast.error(title, { description, duration, id });
      break;
    case 'warning':
      toast.warning(title, { description, duration, id });
      break;
    case 'info':
      toast.info(title, { description, duration, id });
      break;
    default:
      toast(title, { description, duration, id });
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
 * Get network error message based on current locale
 */
function getNetworkErrorMessage(): string {
  return getIsZhHans() ? zhHans.networkError : enUS.networkError;
}

function getCommonMessages() {
  return getIsZhHans() ? zhHans : enUS;
}

function normalizeBackendMessage(message: string | null | undefined, status?: number): string | null {
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
}

/**
 * Extract error message from unknown error types
 */
export function getErrorMessage(error: unknown): string {
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

export const { showNetworkError, showValidationError, showError } = ErrorNotificationService;
