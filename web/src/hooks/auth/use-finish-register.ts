'use client';

import { useMutation } from '@tanstack/react-query';
import { authenticationService } from '@/services/auth.service';
import type { RegisterFinishRequest } from '@/services/types/auth';
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
    onError: error => {
      const title = t('registrationFailed');
      const descriptionKey = getAuthBusinessErrorDescriptionKey(error, {
        context: 'register',
      });
      const description = descriptionKey ? t(descriptionKey) : getAuthBusinessErrorMessage(error);
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    },
  });
}
