'use client';

import { useMutation } from '@tanstack/react-query';
import { authenticationService } from '@/services/auth.service';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { useAuthStore } from '@/store/auth-store';
import { clearSessionBoundClientState } from '@/lib/auth/client-state';
import { setLogoutInProgress } from '@/lib/auth/logout-state';
import { queryClient, setQueryClientQueriesEnabled } from '@/lib/query-client';
import {
  getAuthBusinessErrorDescriptionKey,
  getAuthBusinessErrorMessage,
} from '@/utils/auth-errors';
import { normalizeToastDescription } from '@/utils/error-notifications';

export function useLogout() {
  const t = useT('auth');

  return useMutation({
    mutationKey: ['auth', 'logout'],
    retry: false,
    onMutate: async () => {
      setLogoutInProgress(true);
      useAuthStore.getState().setLoggingOut(true);
      setQueryClientQueriesEnabled(false);
      await queryClient.cancelQueries({ type: 'active' });
    },
    mutationFn: async () => {
      return authenticationService.logout();
    },
    onSettled: async (_data, error) => {
      try {
        await clearSessionBoundClientState();
        useAuthStore.getState().reset({ clearSession: false });
        if (!error) {
          toast.success(t('logoutSuccess'));
        }
      } finally {
        setLogoutInProgress(false);
        useAuthStore.getState().setLoggingOut(false);
        setQueryClientQueriesEnabled(true);
      }
    },
    onError: error => {
      const title = t('logoutFailed');
      const descriptionKey = getAuthBusinessErrorDescriptionKey(error);
      const description = descriptionKey ? t(descriptionKey) : getAuthBusinessErrorMessage(error);
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    },
  });
}
