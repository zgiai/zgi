'use client';

import React, {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  useTransition,
  useCallback,
  type ReactNode,
} from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { NextIntlClientProvider } from 'next-intl';
import { flushSync } from 'react-dom';
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

function getMissingRouteModules(messages: I18nMessages, pathname: string) {
  return getModulesForPathname(pathname).filter(
    module => !Object.prototype.hasOwnProperty.call(messages, module)
  );
}

function isDashboardPathname(pathname: string): boolean {
  return pathname.split('/').filter(Boolean).includes('dashboard');
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
  const router = useRouter();
  const [locale, setLocaleState] = useState<Locale>(initialLocale);
  const [messages, setMessages] = useState(initialMessages);
  const [isPending, startTransition] = useTransition();
  const messagesRef = useRef(messages);
  const routeMessageLoadsRef = useRef(new Map<string, Promise<I18nMessages>>());
  const languageSwitchEnabled = isLanguageSwitchEnabled();
  const resolvedLocale = languageSwitchEnabled ? locale : defaultLocale;
  const isRouteMessagesReady = hasRouteMessages(messages, pathname);

  useEffect(() => {
    messagesRef.current = messages;
  }, [messages]);

  const mergeMessages = useCallback(
    (routeMessages: I18nMessages, options?: { sync?: boolean }) => {
      const updateMessages = () => {
        setMessages(currentMessages => {
          const nextMessages = {
            ...currentMessages,
            ...routeMessages,
          };
          messagesRef.current = nextMessages;
          return nextMessages;
        });
      };

      if (options?.sync) {
        flushSync(updateMessages);
        return;
      }

      startTransition(updateMessages);
    },
    [startTransition]
  );

  const ensureRouteMessages = useCallback(
    async (targetPathname: string, options?: { sync?: boolean }) => {
      const missingModules = getMissingRouteModules(messagesRef.current, targetPathname);

      if (missingModules.length === 0) {
        return;
      }

      const loadKey = `${resolvedLocale}:${missingModules.join('|')}`;
      let loadPromise = routeMessageLoadsRef.current.get(loadKey);

      if (!loadPromise) {
        loadPromise = loadModules(missingModules, resolvedLocale) as Promise<I18nMessages>;
        routeMessageLoadsRef.current.set(loadKey, loadPromise);
      }

      const routeMessages = await loadPromise;
      mergeMessages(routeMessages, options);
    },
    [mergeMessages, resolvedLocale]
  );

  useEffect(() => {
    if (isRouteMessagesReady) {
      return;
    }

    void ensureRouteMessages(pathname);
  }, [ensureRouteMessages, isRouteMessagesReady, pathname]);

  useEffect(() => {
    function getInternalLink(event: MouseEvent | FocusEvent) {
      const target = event.target;

      if (!(target instanceof Element)) {
        return null;
      }

      const anchor = target.closest<HTMLAnchorElement>('a[href]');

      if (!anchor) {
        return null;
      }

      if (anchor.target && anchor.target !== '_self') {
        return null;
      }

      if (anchor.hasAttribute('download')) {
        return null;
      }

      const url = new URL(anchor.href);

      if (url.origin !== window.location.origin) {
        return null;
      }

      const internalHref = `${url.pathname}${url.search}${url.hash}`;
      const currentHref = `${window.location.pathname}${window.location.search}${window.location.hash}`;

      if (internalHref === currentHref) {
        return null;
      }

      const rawHref = anchor.getAttribute('href');

      return {
        href: rawHref && rawHref.startsWith('/') ? rawHref : internalHref,
        hardHref: anchor.href,
        pathname: url.pathname,
      };
    }

    function preloadRouteMessages(event: MouseEvent | FocusEvent) {
      const link = getInternalLink(event);

      if (!link || hasRouteMessages(messagesRef.current, link.pathname)) {
        return;
      }

      void ensureRouteMessages(link.pathname);
    }

    function handleInternalLinkClick(event: MouseEvent) {
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      ) {
        return;
      }

      const link = getInternalLink(event);
      const needsRouteMessages = link
        ? !hasRouteMessages(messagesRef.current, link.pathname)
        : false;
      const shouldUseDocumentNavigation = link ? isDashboardPathname(link.pathname) : false;

      if (!link || (!needsRouteMessages && !shouldUseDocumentNavigation)) {
        return;
      }

      event.preventDefault();
      event.stopImmediatePropagation();

      void ensureRouteMessages(link.pathname, { sync: true }).then(() => {
        if (shouldUseDocumentNavigation) {
          window.location.assign(link.hardHref);
          return;
        }

        router.push(link.href);
      });
    }

    document.addEventListener('pointerover', preloadRouteMessages, true);
    document.addEventListener('focusin', preloadRouteMessages, true);
    document.addEventListener('click', handleInternalLinkClick, true);

    return () => {
      document.removeEventListener('pointerover', preloadRouteMessages, true);
      document.removeEventListener('focusin', preloadRouteMessages, true);
      document.removeEventListener('click', handleInternalLinkClick, true);
    };
  }, [ensureRouteMessages, router]);

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
        {children}
      </NextIntlClientProvider>
    </I18nContext.Provider>
  );
}
