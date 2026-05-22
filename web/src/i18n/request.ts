import { getRequestConfig } from 'next-intl/server';
import { cookies, headers } from 'next/headers';
import { DEFAULT_LOCALE, ENABLE_LANG_SWITCH } from '@/lib/config';
import { locales, defaultLocale, localeMapping, type Locale } from './config';
import { loadModules } from './loader';
import { getModulesForPathname } from './route-modules';

export default getRequestConfig(async () => {
  const headerStore = await headers();
  const pathname = headerStore.get('x-zgi-pathname') || '/';
  const modules = getModulesForPathname(pathname);
  const fallbackLocale =
    (localeMapping[DEFAULT_LOCALE] ??
      localeMapping[DEFAULT_LOCALE.toLowerCase()] ??
      (locales.includes(DEFAULT_LOCALE as Locale) ? (DEFAULT_LOCALE as Locale) : undefined) ??
      defaultLocale) as Locale;

  // When language switching is disabled, always use the configured default locale.
  if (!ENABLE_LANG_SWITCH) {
    return {
      locale: fallbackLocale,
      messages: await loadModules(modules, fallbackLocale),
    };
  }

  // Try to get locale from cookie first
  const cookieStore = await cookies();
  const localeCookie = cookieStore.get('locale')?.value as Locale | undefined;

  // Determine locale with fallback logic
  let locale: Locale = fallbackLocale;

  if (localeCookie && locales.includes(localeCookie)) {
    locale = localeCookie;
  } else {
    // Try Accept-Language header
    const acceptLanguage = headerStore.get('accept-language');

    if (acceptLanguage) {
      // Parse Accept-Language header (e.g., "zh-CN,zh;q=0.9,en;q=0.8")
      const languages = acceptLanguage.split(',').map(lang => lang.split(';')[0].trim());

      for (const lang of languages) {
        // Try exact match first, then language-only match
        const mappedLocale = localeMapping[lang] ?? localeMapping[lang.toLowerCase()];
        if (mappedLocale) {
          locale = mappedLocale;
          break;
        }
        const langOnly = lang.split('-')[0].toLowerCase();
        if (localeMapping[langOnly]) {
          locale = localeMapping[langOnly];
          break;
        }
      }
    }
  }

  return {
    locale,
    messages: await loadModules(modules, locale),
  };
});

export { locales };
export type { Locale as LanguageValue };
