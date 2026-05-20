'use client';

import { useMutation } from '@tanstack/react-query';
import { authenticationService } from '@/services/auth.service';
import type { RegisterVerifyRequest } from '@/services/types/auth';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import {
  getAuthBusinessErrorCode,
  getAuthBusinessErrorDescriptionKey,
  getAuthBusinessErrorMessage,
} from '@/utils/auth-errors';
import { normalizeToastDescription } from '@/utils/error-notifications';

const VERIFY_CODE_ERROR_LIMIT = 5;
const VERIFY_CODE_ERROR_CODE = '401002';
const VERIFY_CODE_ERROR_WINDOW_MS = 30 * 60 * 1000;
const forgotPasswordVerifyFailureCounts = new Map<
  string,
  { count: number; lastFailedAt: number }
>();

function normalizeVerifyEmail(email: string): string {
  return email.trim().toLowerCase();
}

function incrementVerifyFailureCount(email: string): number {
  const normalizedEmail = normalizeVerifyEmail(email);
  if (!normalizedEmail) {
    return 0;
  }

  const now = Date.now();
  const current = forgotPasswordVerifyFailureCounts.get(normalizedEmail);
  const currentCount =
    current && now - current.lastFailedAt < VERIFY_CODE_ERROR_WINDOW_MS ? current.count : 0;
  const nextCount = currentCount + 1;
  forgotPasswordVerifyFailureCounts.set(normalizedEmail, {
    count: nextCount,
    lastFailedAt: now,
  });
  return nextCount;
}

function clearVerifyFailureCount(email: string): void {
  const normalizedEmail = normalizeVerifyEmail(email);
  if (!normalizedEmail) {
    return;
  }

  forgotPasswordVerifyFailureCounts.delete(normalizedEmail);
}

export function useVerifyForgotPassword() {
  const t = useT('auth');

  return useMutation({
    mutationKey: ['auth', 'verify-forgot-password'],
    mutationFn: async (payload: RegisterVerifyRequest) => {
      return authenticationService.verifyForgotPassword(payload);
    },
    onSuccess: (_result, variables) => {
      clearVerifyFailureCount(variables.email);
      toast.success(t('verificationCodeSent'));
    },
    onError: (error, variables) => {
      const title = t('passwordResetFailed');
      const descriptionKey = getAuthBusinessErrorDescriptionKey(error, {
        context: 'verification',
      });
      const code = getAuthBusinessErrorCode(error);
      const description = descriptionKey ? t(descriptionKey) : getAuthBusinessErrorMessage(error);
      const failedCount =
        code === VERIFY_CODE_ERROR_CODE ? incrementVerifyFailureCount(variables.email) : 0;
      const failedCountDescription =
        failedCount >= VERIFY_CODE_ERROR_LIMIT
          ? t('verificationCodeErrorLimitReachedHint', { max: VERIFY_CODE_ERROR_LIMIT })
          : failedCount > 0
            ? t('verificationCodeErrorRemainingHint', {
                remaining: VERIFY_CODE_ERROR_LIMIT - failedCount,
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
