'use client';

import { useMutation } from '@tanstack/react-query';
import { accountService } from '@/services/account.service';
import { useLocale } from '@/hooks/use-locale';
import type { Locale } from '@/lib/i18n';
import { useAuthStore } from '@/store/auth-store';
import { sessionManager } from '@/lib/auth/session-manager';
import { clearProfileClientCache } from '@/utils/client-cache';

export function useUpdateInterfaceLanguage() {
  const { locale, setLocale } = useLocale();

  return useMutation({
    mutationFn: async (newLocale: Locale) => {
      return accountService.updateInterfaceLanguage(newLocale);
    },
    onMutate: async (newLocale: Locale) => {
      const previous = locale;
      await setLocale(newLocale);
      return { previous } as const;
    },
    onSuccess: async () => {
      // Ensure next profile fetch hits network instead of stale client cache
      clearProfileClientCache();
      // Refresh store profile (will write fresh local cache inside service)
      await useAuthStore.getState().refreshProfile();
      sessionManager.broadcastProfileUpdated();
    },
    onError: async (_error, _newLocale, context) => {
      const prev = context?.previous;
      if (prev) {
        try {
          await setLocale(prev as Locale);
        } catch {
          void 0;
        }
      }
    },
  });
}
