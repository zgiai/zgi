'use client';

import React, {
  createContext,
  useContext,
  useEffect,
  useState,
  useTransition,
  useCallback,
  type ReactNode,
} from 'react';
import { usePathname } from 'next/navigation';
import { NextIntlClientProvider } from 'next-intl';
import { loadModules } from '@/i18n/loader';
import { getModulesForPathname } from '@/i18n/route-modules';
import type { Locale } from '@/i18n/config';
import { defaultLocale, isLanguageSwitchEnabled } from '@/lib/i18n';

type I18nMessages = Record<string, unknown>;

function hasRouteMessages(messages: I18nMessages, pathname: string): boolean {
  const modules = getModulesForPathname(pathname);

  return modules.every(module =>
    Object.prototype.hasOwnProperty.call(messages, module)
  );
}

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
  const pathname = usePathname() || '/';
  const [locale, setLocaleState] = useState<Locale>(initialLocale);
  const [messages, setMessages] = useState(initialMessages);
  const [isPending, startTransition] = useTransition();
  const languageSwitchEnabled = isLanguageSwitchEnabled();
  const resolvedLocale = languageSwitchEnabled ? locale : defaultLocale;
  const isRouteMessagesReady = hasRouteMessages(messages, pathname);

  useEffect(() => {
    if (isRouteMessagesReady) {
      return;
    }

    let isCancelled = false;

    async function syncRouteMessages() {
      const routeMessages = (await loadModules(
        getModulesForPathname(pathname),
        resolvedLocale
      )) as I18nMessages;

      if (isCancelled) {
        return;
      }

      startTransition(() => {
        setMessages(currentMessages => ({
          ...currentMessages,
          ...routeMessages,
        }));
      });
    }

    void syncRouteMessages();

    return () => {
      isCancelled = true;
    };
  }, [isRouteMessagesReady, pathname, resolvedLocale]);

  const setLocale = useCallback(
    async (newLocale: Locale) => {
      if (!languageSwitchEnabled) {
        return;
      }

      if (newLocale === locale) return;

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
    [languageSwitchEnabled, locale, pathname]
  );

  return (
    <I18nContext.Provider value={{ locale: resolvedLocale, setLocale, isPending }}>
      <NextIntlClientProvider
        locale={resolvedLocale}
        messages={messages}
        timeZone="Asia/Shanghai"
      >
        {isRouteMessagesReady ? children : null}
      </NextIntlClientProvider>
    </I18nContext.Provider>
  );
}
