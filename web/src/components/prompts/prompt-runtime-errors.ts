'use client';

import { getErrorMessage } from '@/utils/error-notifications';

function normalizeErrorText(error: unknown): string {
  if (error instanceof Error) {
    return error.message || '';
  }
  if (typeof error === 'string') {
    return error;
  }
  return '';
}

export function getPromptRuntimeErrorMessage(
  error: unknown,
  modelLabel: string | undefined,
  isAdmin: boolean,
  t: (key: string, values?: Record<string, string | number>) => string
) {
  const rawMessage = normalizeErrorText(error);
  const normalized = rawMessage.toLowerCase();
  const fallback = getErrorMessage(error);
  const model = modelLabel || t('messages.currentModelFallback');

  if (
    normalized.includes('overdue-payment') ||
    normalized.includes('arrearage') ||
    normalized.includes('good standing')
  ) {
    return {
      kind: 'billing' as const,
      message: t('messages.providerBillingIssue', { model }),
      hint: isAdmin
        ? t('messages.providerBillingHintAdmin')
        : t('messages.providerBillingHintMember'),
      details: rawMessage,
    };
  }

  if (normalized.includes('access denied') || normalized.includes('forbidden')) {
    return {
      kind: 'access' as const,
      message: t('messages.providerAccessDenied', { model }),
      hint: isAdmin
        ? t('messages.providerAccessHintAdmin')
        : t('messages.providerAccessHintMember'),
      details: rawMessage,
    };
  }

  return {
    kind: 'generic' as const,
    message: fallback,
    hint: '',
    details: rawMessage && rawMessage !== fallback ? rawMessage : '',
  };
}
