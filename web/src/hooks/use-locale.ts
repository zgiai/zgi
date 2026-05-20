import { useI18n } from '@/providers/i18n-client-provider';
import { defaultLocale, isLanguageSwitchEnabled } from '@/lib/i18n';

export function useLocale() {
  const { locale, setLocale, isPending } = useI18n();

  return {
    locale,
    setLocale,
    isPending,
    isEnabled: isLanguageSwitchEnabled(),
    defaultLocale,
  };
}
