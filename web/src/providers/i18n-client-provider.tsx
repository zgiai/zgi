'use client';

import React, {
  createContext,
  useContext,
  useState,
  useTransition,
  useCallback,
  type ReactNode,
} from 'react';
import { NextIntlClientProvider } from 'next-intl';
import { loadModules } from '@/i18n/loader';
import { getModulesForPathname } from '@/i18n/route-modules';
import type { Locale } from '@/i18n/config';
import { defaultLocale, isLanguageSwitchEnabled } from '@/lib/i18n';

type I18nMessages = Record<string, unknown>;

interface I18nContextType {
  locale: Locale;
  setLocale: (newLocale: Locale) => Promise<void>;
  isPending: boolean;
}

const I18nContext = createContext<I18nContextType | null>(null);

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) {
    throw new Error('useI18n must be used within I18nClientProvider');
  }
  return context;
}

interface I18nClientProviderProps {
  children: ReactNode;
  initialLocale: Locale;
  initialMessages: I18nMessages;
}

export function I18nClientProvider({
  children,
  initialLocale,
  initialMessages,
}: I18nClientProviderProps) {
  const [locale, setLocaleState] = useState<Locale>(initialLocale);
  const [messages, setMessages] = useState(initialMessages);
  const [isPending, startTransition] = useTransition();

  const setLocale = useCallback(
    async (newLocale: Locale) => {
      if (!isLanguageSwitchEnabled()) {
        return;
      }

      if (newLocale === locale) return;

      // Load new messages package in the background
      const pathname = typeof window !== 'undefined' ? window.location.pathname : '/';
      const newMessages = (await loadModules(
        getModulesForPathname(pathname),
        newLocale
      )) as I18nMessages;

      startTransition(() => {
        // Update cookie for future server-side renders or reloads
        const expires = new Date();
        expires.setFullYear(expires.getFullYear() + 1);
        document.cookie = `locale=${newLocale};expires=${expires.toUTCString()};path=/`;

        // Update state to trigger soft-replacement of translations
        setLocaleState(newLocale);
        setMessages(newMessages);
      });
    },
    [locale]
  );

  const resolvedLocale = isLanguageSwitchEnabled() ? locale : defaultLocale;
  const resolvedMessages = isLanguageSwitchEnabled() ? messages : initialMessages;

  return (
    <I18nContext.Provider value={{ locale: resolvedLocale, setLocale, isPending }}>
      <NextIntlClientProvider
        locale={resolvedLocale}
        messages={resolvedMessages}
        timeZone="Asia/Shanghai"
      >
        {children}
      </NextIntlClientProvider>
    </I18nContext.Provider>
  );
}
