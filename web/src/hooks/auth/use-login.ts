'use client';

import { useMutation } from '@tanstack/react-query';
import { authenticationService } from '@/services/auth.service';
import type { LoginRequest } from '@/services/types/auth';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { useAuthStore } from '@/store/auth-store';
import { clearSessionBoundClientState } from '@/lib/auth/client-state';
import {
  getAuthBusinessErrorCode,
  getAuthBusinessErrorData,
  getAuthBusinessErrorDescriptionKey,
  getAuthBusinessErrorMessage,
} from '@/utils/auth-errors';
import { normalizeToastDescription } from '@/utils/error-notifications';

const LOGIN_PASSWORD_ERROR_CODE = '201017';
const LOGIN_RATE_LIMIT_CODE = '201018';
const ACCOUNT_NOT_FOUND_SPECIAL_CODE = 'account_not_found';
const LOGIN_PASSWORD_ERROR_LIMIT = 5;
const LOGIN_PASSWORD_ERROR_WINDOW_MS = 30 * 60 * 1000;
const loginFailureCounts = new Map<string, { count: number; lastFailedAt: number }>();

function normalizeLoginEmail(email: string): string {
  return email.trim().toLowerCase();
}

function incrementLoginFailureCount(email: string): number {
  const normalizedEmail = normalizeLoginEmail(email);
  if (!normalizedEmail) {
    return 0;
  }

  const now = Date.now();
  const current = loginFailureCounts.get(normalizedEmail);
  const currentCount =
    current && now - current.lastFailedAt < LOGIN_PASSWORD_ERROR_WINDOW_MS ? current.count : 0;
  const nextCount = currentCount + 1;
  loginFailureCounts.set(normalizedEmail, {
    count: nextCount,
    lastFailedAt: now,
  });
  return nextCount;
}

function clearLoginFailureCount(email: string): void {
  const normalizedEmail = normalizeLoginEmail(email);
  if (!normalizedEmail) {
    return;
  }

  loginFailureCounts.delete(normalizedEmail);
}

export function useLogin() {
  const t = useT('auth');

  return useMutation({
    mutationKey: ['auth', 'login'],
    mutationFn: async (payload: LoginRequest) => {
      return authenticationService.login(payload);
    },
    onSuccess: async (_result, variables) => {
      clearLoginFailureCount(variables.email);
      await clearSessionBoundClientState();
      try {
        await useAuthStore.getState().initializeAuth({ force: true });
      } catch {
        // Ignore bootstrap failures here and let subsequent navigation retry.
      }
      toast.success(t('loginSuccess'));
    },
    onError: (error, variables) => {
      const title = t('loginError');
      const descriptionKey = getAuthBusinessErrorDescriptionKey(error, {
        context: 'login',
      });
      const code = getAuthBusinessErrorCode(error);
      const errorData = getAuthBusinessErrorData(error);
      if (
        code === ACCOUNT_NOT_FOUND_SPECIAL_CODE &&
        typeof errorData === 'string' &&
        errorData.length > 0
      ) {
        return;
      }

      const description =
        code === LOGIN_RATE_LIMIT_CODE
          ? t('loginPasswordErrorLockedHint')
          : descriptionKey
            ? t(descriptionKey)
            : getAuthBusinessErrorMessage(error);
      const failedCount =
        code === LOGIN_PASSWORD_ERROR_CODE ? incrementLoginFailureCount(variables.email) : 0;
      const failedCountDescription =
        failedCount >= LOGIN_PASSWORD_ERROR_LIMIT
          ? t('loginPasswordErrorLimitReachedHint', { max: LOGIN_PASSWORD_ERROR_LIMIT })
          : failedCount > 0
            ? t('loginPasswordErrorRemainingHint', {
                remaining: LOGIN_PASSWORD_ERROR_LIMIT - failedCount,
              })
            : undefined;
      toast.error(title, {
        description: normalizeToastDescription(
          title,
          [description, failedCountDescription].filter(Boolean).join('\n')
        ),
      });
    },
  });
}
