'use client';

import { useCallback } from 'react';
import { useT } from '@/i18n';
import { translateProviderName } from '@/utils/provider/meta';

type ProviderTranslateFn = ((
  key: string,
  values?: Record<string, string | number | Date>
) => string) & {
  has?: (key: string) => boolean;
};

/**
 * Hook that converts provider ids or paths into localized display names.
 */
export function useProviderI18n() {
  const t = useT('aiProviders');
  const translateKey = useCallback<ProviderTranslateFn>(
    (key: string, values?: Record<string, string | number | Date>) => {
      const translated = t(key as never, values);
      return translated;
    },
    [t]
  );

  translateKey.has = (key: string) => {
    const maybeHas = (t as { has?: (messageKey: string) => boolean }).has;
    if (typeof maybeHas === 'function') {
      return maybeHas(key);
    }

    try {
      const translated = t(key as never);
      return translated !== key && !translated.startsWith('providers.');
    } catch {
      return false;
    }
  };

  return useCallback(
    (provider?: string | null, fallbackName?: string | null) =>
      translateProviderName(provider, { fallbackName, t: translateKey }),
    [translateKey]
  );
}
