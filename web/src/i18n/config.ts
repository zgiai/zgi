import { LANGUAGES, type LanguageValue } from '@/lib/constants';

// Supported locales
export const locales = LANGUAGES.map(lang => lang.value);
export type Locale = LanguageValue;

// Default locale
export const defaultLocale: Locale = 'zh-Hans';

// Available languages as a record for easy access
export const localeNames: Record<Locale, string> = LANGUAGES.reduce(
  (acc, lang) => {
    acc[lang.value] = lang.label;
    return acc;
  },
  {} as Record<Locale, string>
);

// Mapping for Accept-Language header or other automated detection
export const localeMapping: Record<string, Locale> = {
  zh: 'zh-Hans',
  'zh-CN': 'zh-Hans',
  'zh-cn': 'zh-Hans',
  en: 'en-US',
  'en-US': 'en-US',
  'en-us': 'en-US',
};
