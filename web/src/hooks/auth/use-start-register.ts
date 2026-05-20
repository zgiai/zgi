'use client';

import { useMutation } from '@tanstack/react-query';
import { authenticationService } from '@/services/auth.service';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import {
  getAuthBusinessErrorCode,
  getAuthBusinessErrorDescriptionKey,
  getAuthBusinessErrorMessage,
} from '@/utils/auth-errors';
import { normalizeToastDescription } from '@/utils/error-notifications';

export interface StartRegisterPayload {
  email: string;
  language: string;
}

export function useStartRegister() {
  const t = useT('auth');

  return useMutation({
    mutationKey: ['auth', 'start-register'],
    mutationFn: async (payload: StartRegisterPayload) => {
      return authenticationService.startRegister(payload.email, payload.language);
    },
    onSuccess: () => {
      toast.success(t('verificationCodeSent'));
    },
    onError: error => {
      const code = getAuthBusinessErrorCode(error);
      if (code === '201001' || code === '201003') {
        toast.error(t('userAlreadyExists'));
        return;
      }

      const title = t('failedToStartRegistration');
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
