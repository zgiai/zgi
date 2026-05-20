'use client';

import { useMutation } from '@tanstack/react-query';
import { authenticationService } from '@/services/auth.service';
import type { RegisterVerifyRequest } from '@/services/types/auth';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import {
  getAuthBusinessErrorDescriptionKey,
  getAuthBusinessErrorMessage,
} from '@/utils/auth-errors';
import { normalizeToastDescription } from '@/utils/error-notifications';

export function useVerifyRegister() {
  const t = useT('auth');

  return useMutation({
    mutationKey: ['auth', 'verify-register'],
    mutationFn: async (payload: RegisterVerifyRequest) => {
      return authenticationService.verifyRegister(payload);
    },
    onSuccess: () => {
      toast.success(t('verificationCodeSent'));
    },
    onError: error => {
      const title = t('registrationFailed');
      const descriptionKey = getAuthBusinessErrorDescriptionKey(error, {
        context: 'verification',
      });
      const description = descriptionKey ? t(descriptionKey) : getAuthBusinessErrorMessage(error);
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    },
  });
}
