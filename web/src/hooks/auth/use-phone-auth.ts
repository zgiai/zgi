import { useMutation } from '@tanstack/react-query';
import { toast } from 'sonner';
import { authService } from '@/services/auth.service';
import {
  getAuthBusinessErrorDescriptionKey,
  getAuthBusinessErrorMessage,
} from '@/utils/auth-errors';
import { normalizeToastDescription } from '@/utils/error-notifications';
import type {
  PhoneCheckRequest,
  PhoneCheckResponse,
  PhoneCodeRequest,
  PhoneVerifyRequest,
  PhoneRegisterRequest,
  PhoneLoginRequest,
} from '@/services/types/auth';
import { useT } from '@/i18n';
import { useAuthStore } from '@/store/auth-store';
import { clearSessionBoundClientState } from '@/lib/auth/client-state';

type PhoneAuthMessageKey =
  | 'verificationCodeSent'
  | 'phoneNotRegistered'
  | 'sendCodeError'
  | 'userAlreadyExists';

export function usePhoneCheck(options?: {
  successMessageKey?: PhoneAuthMessageKey;
  errorMessageKey?: PhoneAuthMessageKey;
  silentSuccess?: boolean;
  onSuccess?: (data: PhoneCheckResponse) => void;
}) {
  const t = useT('auth');
  return useMutation({
    mutationFn: (data: PhoneCheckRequest) => authService.checkPhone(data),
    onSuccess: data => {
      options?.onSuccess?.(data);
      if (!options?.silentSuccess) {
        toast.success(t(options?.successMessageKey || 'verificationCodeSent'));
      }
    },
    onError: error => {
      const title = t(options?.errorMessageKey || 'sendCodeError');
      const description = getAuthBusinessErrorMessage(error);
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    },
  });
}

export function usePhoneCode() {
  const t = useT('auth');
  return useMutation({
    mutationFn: (data: PhoneCodeRequest) => authService.sendPhoneCode(data),
    onSuccess: () => {
      toast.success(t('verificationCodeSent'));
    },
    onError: error => {
      const title = t('sendCodeError');
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

export function usePhoneVerify() {
  const t = useT('auth');
  return useMutation({
    mutationFn: (data: PhoneVerifyRequest) => authService.verifyPhoneCode(data),
    onSuccess: () => {
      toast.success(t('phoneVerified'));
    },
    onError: error => {
      const title = t('verificationError');
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

export function usePhoneRegister() {
  const t = useT('auth');
  return useMutation({
    mutationFn: (data: PhoneRegisterRequest) => authService.phoneRegister(data),
    onSuccess: async () => {
      await clearSessionBoundClientState();
      try {
        await useAuthStore.getState().initializeAuth({ force: true });
      } catch {
        // Ignore bootstrap failures here and let subsequent navigation retry.
      }
      toast.success(t('registrationSuccess'));
    },
    onError: error => {
      const title = t('registrationError');
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

export function usePhoneLogin() {
  const t = useT('auth');
  return useMutation({
    mutationFn: (data: PhoneLoginRequest) => authService.phoneLogin(data),
    onSuccess: async () => {
      await clearSessionBoundClientState();
      try {
        await useAuthStore.getState().initializeAuth({ force: true });
      } catch {
        // Ignore bootstrap failures here and let subsequent navigation retry.
      }
      toast.success(t('loginSuccess'));
    },
    onError: error => {
      const title = t('loginError');
      const descriptionKey = getAuthBusinessErrorDescriptionKey(error, {
        context: 'login',
      });
      const description = descriptionKey ? t(descriptionKey) : getAuthBusinessErrorMessage(error);
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    },
  });
}
