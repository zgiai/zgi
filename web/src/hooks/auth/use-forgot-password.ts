'use client';

import { useMutation } from '@tanstack/react-query';
import { authenticationService } from '@/services/auth.service';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import {
  getAuthBusinessErrorDescriptionKey,
  getAuthBusinessErrorMessage,
} from '@/utils/auth-errors';
import { normalizeToastDescription } from '@/utils/error-notifications';

export interface ForgotPasswordPayload {
  email: string;
  language?: string;
}

export function useForgotPassword() {
  const t = useT('auth');

  return useMutation({
    mutationKey: ['auth', 'forgot-password'],
    mutationFn: async (payload: ForgotPasswordPayload) => {
      return authenticationService.forgotPassword(payload.email, payload.language);
    },
    onSuccess: () => {
      toast.success(t('codeSent'));
    },
    onError: error => {
      const title = t('errorSendingRecovery');
      const descriptionKey = getAuthBusinessErrorDescriptionKey(error, {
        context: 'forgotPassword',
      });
      const description = descriptionKey ? t(descriptionKey) : getAuthBusinessErrorMessage(error);
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    },
  });
}
