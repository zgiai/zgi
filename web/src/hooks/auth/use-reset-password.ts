'use client';

import { useMutation } from '@tanstack/react-query';
import { authenticationService } from '@/services/auth.service';
import type { ResetPasswordRequest } from '@/services/types/auth';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { useAuthStore } from '@/store/auth-store';
import { clearSessionBoundClientState } from '@/lib/auth/client-state';
import { sessionManager } from '@/lib/auth/session-manager';
import {
  getAuthBusinessErrorDescriptionKey,
  getAuthBusinessErrorMessage,
} from '@/utils/auth-errors';
import { normalizeToastDescription } from '@/utils/error-notifications';

export function useResetPassword() {
  const t = useT('auth');

  return useMutation({
    mutationKey: ['auth', 'reset-password'],
    mutationFn: async (payload: ResetPasswordRequest) => {
      return authenticationService.resetPassword({
        email: payload.email,
        new_password: payload.password,
        password_confirm: payload.password_confirm,
        token: payload.token,
        language: payload.language,
      });
    },
    onSuccess: async result => {
      if (result?.data?.access_token) {
        sessionManager.setSession(
          {
            accessToken: result.data.access_token,
            refreshToken: result.data.refresh_token,
          },
          { type: 'SIGNED_IN' }
        );
        await clearSessionBoundClientState();
        try {
          await useAuthStore.getState().initializeAuth({ force: true });
        } catch {
          // Ignore bootstrap failures and let subsequent navigation retry.
        }
      }
      toast.success(t('passwordResetSuccess'));
    },
    onError: error => {
      const title = t('passwordResetFailed');
      const descriptionKey = getAuthBusinessErrorDescriptionKey(error, {
        context: 'resetPassword',
      });
      const description = descriptionKey ? t(descriptionKey) : getAuthBusinessErrorMessage(error);
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    },
  });
}
