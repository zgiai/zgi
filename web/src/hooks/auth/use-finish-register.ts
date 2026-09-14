'use client';

import { useMutation } from '@tanstack/react-query';
import { authenticationService } from '@/services/auth.service';
import type { RegisterFinishRequest } from '@/services/types/auth';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { useAuthStore } from '@/store/auth-store';
import { clearSessionBoundClientState } from '@/lib/auth/client-state';
import { sessionManager } from '@/lib/auth/session-manager';
import { getRegistrationErrorDescription } from '@/utils/auth-errors';
import { normalizeToastDescription } from '@/utils/error-notifications';

export function useFinishRegister() {
  const t = useT('auth');

  return useMutation({
    mutationKey: ['auth', 'finish-register'],
    mutationFn: async (payload: RegisterFinishRequest) => {
      return authenticationService.finishRegister(payload);
    },
    onSuccess: async result => {
      sessionManager.setSession(
        {
          accessToken: result.access_token,
          refreshToken: result.refresh_token,
        },
        { type: 'SIGNED_IN' }
      );
      await clearSessionBoundClientState();
      try {
        await useAuthStore.getState().initializeAuth({ force: true });
      } catch {
        // Ignore bootstrap failures and let subsequent navigation retry.
      }
      toast.success(t('registerSuccess'));
    },
    onError: (error, variables) => {
      const title = t('registrationFailed');
      const errorDescription = getRegistrationErrorDescription(error, variables.invite_token);
      const description = errorDescription.key ? t(errorDescription.key) : errorDescription.message;
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    },
  });
}
