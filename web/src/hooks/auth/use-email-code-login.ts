'use client';

import { useMutation } from '@tanstack/react-query';
import { toast } from 'sonner';

import { useT } from '@/i18n';
import { clearSessionBoundClientState } from '@/lib/auth/client-state';
import { authenticationService } from '@/services/auth.service';
import type { EmailCodeLoginSendRequest, EmailCodeLoginVerifyRequest } from '@/services/types/auth';
import { useAuthStore } from '@/store/auth-store';
import { getAuthBusinessErrorMessage } from '@/utils/auth-errors';
import { normalizeToastDescription } from '@/utils/error-notifications';

export function useSendEmailLoginCode() {
  const t = useT('auth');
  return useMutation({
    mutationFn: (data: EmailCodeLoginSendRequest) => authenticationService.sendEmailLoginCode(data),
    onSuccess: () => toast.success(t('verificationCodeSent')),
    onError: error => {
      const title = t('sendCodeError');
      toast.error(title, {
        description: normalizeToastDescription(title, getAuthBusinessErrorMessage(error)),
      });
    },
  });
}

export function useEmailCodeLogin() {
  const t = useT('auth');
  return useMutation({
    mutationFn: (data: EmailCodeLoginVerifyRequest) => authenticationService.loginByEmailCode(data),
    onSuccess: async () => {
      await clearSessionBoundClientState();
      try {
        await useAuthStore.getState().initializeAuth({ force: true });
      } catch {
        // Navigation will retry session bootstrap.
      }
      toast.success(t('loginSuccess'));
    },
    onError: error => {
      const title = t('loginError');
      toast.error(title, {
        description: normalizeToastDescription(title, getAuthBusinessErrorMessage(error)),
      });
    },
  });
}
