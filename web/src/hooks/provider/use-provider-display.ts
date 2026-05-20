'use client';

import { useLocale } from 'next-intl';
import type { ProviderItem } from '@/services/types/provider';
import { useProviderI18n } from './use-provider-i18n';
import { resolveProviderDisplayInfo, type ProviderDisplayInfo } from '@/utils/provider/meta';

export function useProviderDisplay(provider?: ProviderItem): ProviderDisplayInfo {
  const locale = useLocale();
  const getProviderName = useProviderI18n();

  return resolveProviderDisplayInfo(provider, { locale, getProviderName });
}
