'use client';

import { useMutation } from '@tanstack/react-query';
import { authenticationService } from '@/services/auth.service';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { useAuthStore } from '@/store/auth-store';
import { clearSessionBoundClientState } from '@/lib/auth/client-state';
import {
  getAuthBusinessErrorDescriptionKey,
  getAuthBusinessErrorMessage,
} from '@/utils/auth-errors';
import { normalizeToastDescription } from '@/utils/error-notifications';

export function useConsumeCasdoorTicket() {
  const t = useT('auth');
  const logPrefix = '[SSO Mutation]';

  return useMutation({
    mutationKey: ['auth', 'casdoor', 'consume-ticket'],
    mutationFn: async (ticket: string) => {
      console.info(logPrefix, 'mutationFn start', {
        ticketPreview: `${ticket.slice(0, 8)}...`,
      });
      return authenticationService.consumeCasdoorTicket({ ticket });
    },
    onSuccess: async data => {
      console.info(logPrefix, 'onSuccess', {
        hasAccessToken: Boolean(data?.access_token),
        hasRefreshToken: Boolean(data?.refresh_token),
        userId: data?.user?.id,
      });

      try {
        await clearSessionBoundClientState();
        console.info(logPrefix, 'session-bound client state cleared');
        console.info(logPrefix, 'initializeAuth start');
        await useAuthStore.getState().initializeAuth({ force: true });
        console.info(logPrefix, 'initializeAuth done');
      } catch (error) {
        console.error(logPrefix, 'initializeAuth failed', error);
        void error;
      }

      console.info(logPrefix, 'show login success toast');
      toast.success(t('loginSuccess'));
    },
    onError: error => {
      console.error(logPrefix, 'onError', error);
      const title = t('ssoLoginFailed');
      const descriptionKey = getAuthBusinessErrorDescriptionKey(error);
      const description = descriptionKey ? t(descriptionKey) : getAuthBusinessErrorMessage(error);
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    },
  });
}
