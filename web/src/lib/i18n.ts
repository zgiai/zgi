import { DEFAULT_LOCALE, ENABLE_LANG_SWITCH } from '@/lib/config';
import { locales, localeMapping, type Locale } from '@/i18n/config';
export { useT, getT, type UnifiedTranslations } from '@/i18n/translations';

export type { Locale };

/**
 * Normalize a locale string to a valid Locale value
 * Handles shorthand values like "zh" or "en" and invalid values
 * @param value - The locale string to normalize
 * @param fallback - The fallback locale (default: 'zh-Hans')
 * @returns A valid Locale value
 */
export function normalizeLocale(value: string | undefined, fallback: Locale = 'zh-Hans'): Locale {
  if (!value) return fallback;

  const trimmed = value.trim();

  // Check if it's already a valid locale (exact match)
  if (locales.includes(trimmed as Locale)) {
    return trimmed as Locale;
  }

  // Check mapping (case-insensitive)
  const normalized = trimmed.toLowerCase();
  const mapped = localeMapping[normalized];
  if (mapped) return mapped;

  // Fallback to default
  return fallback;
}

export const defaultLocale: Locale = normalizeLocale(DEFAULT_LOCALE);

// Check if language switching is enabled (client-side safe)
export function isLanguageSwitchEnabled(): boolean {
  if (typeof window === 'undefined') {
    // Server-side: check environment variable
    return ENABLE_LANG_SWITCH;
  }
  // Client-side: check environment variable
  return ENABLE_LANG_SWITCH;
}

// Get locale labels
export function getLocaleLabel(locale: Locale): string {
  const labels = {
    'zh-Hans': '中文',
    'en-US': 'English',
  };
  return labels[locale];
}

// Validate locale
export function isValidLocale(locale: string): locale is Locale {
  return locales.includes(locale as Locale);
}

// Get current locale from client-side storage
export function getCurrentLocale(): Locale {
  if (typeof window === 'undefined') {
    return defaultLocale;
  }

  // Check if language switching is enabled
  if (!isLanguageSwitchEnabled()) {
    return defaultLocale;
  }

  // Get from cookie
  const savedLocale = document.cookie
    .split('; ')
    .find(row => row.startsWith('locale='))
    ?.split('=')[1] as Locale;

  return savedLocale && locales.includes(savedLocale) ? savedLocale : defaultLocale;
}
