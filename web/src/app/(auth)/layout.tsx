'use client';

import { useState, useEffect, type PropsWithChildren } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { useSetupStatus } from '@/hooks';
import { LanguageSwitcher } from '@/components/common/language-switcher';
import {
  APP_NAME,
  AUTH_TITLE_LINE1_EN,
  AUTH_TITLE_LINE1_ZH,
  AUTH_TITLE_LINE2_EN,
  AUTH_TITLE_LINE2_ZH,
  AUTH_DESCRIPTION_EN,
  AUTH_DESCRIPTION_ZH,
  AUTH_BG_IMAGE,
  DISABLE_AUTHENTICATED_REDIRECT_ON_AUTH_PAGES,
  HIDE_AUTH_LEFT_PANEL,
  SINGLE_SSO_PROVIDER,
  withBasePathIfInternal,
} from '@/lib/config';
import { sessionManager } from '@/lib/auth/session-manager';
import { useLocale } from 'next-intl';
import { cn } from '@/lib/utils';
import { Logo } from '@/components/logo';
import { useT } from '@/i18n';
import { Icons } from '@/components/ui/icons';
import { useAuthStore } from '@/store/auth-store';
import {
  buildSsoStartUrl,
  resolveSingleSsoRedirectTarget,
  shouldAutoRedirectToSingleSso,
} from '@/utils/auth-sso';
import { consumePendingLogoutRedirect } from '@/utils/logout-redirect';

/**
 * Shared layout for all authentication related pages
 * This creates a consistent experience for login, register, password reset, etc.
 */
export default function AuthLayout({ children }: PropsWithChildren) {
  const router = useRouter();
  const pathname = usePathname();
  const locale = useLocale();
  const t = useT().auth;
  const { isInitialized, isLoading } = useSetupStatus();
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const authLoading = useAuthStore.use.isLoading();
  const authInitialized = useAuthStore.use.isInitialized();
  const [mounted, setMounted] = useState(false);
  const [singleSsoRedirectUrl, setSingleSsoRedirectUrl] = useState<string | null>(null);

  // Select promotional text based on locale
  const isZh = locale === 'zh-Hans' || locale === 'zh-CN' || locale?.startsWith('zh');
  const titleLine1 = isZh ? AUTH_TITLE_LINE1_ZH : AUTH_TITLE_LINE1_EN;
  const titleLine2 = isZh ? AUTH_TITLE_LINE2_ZH : AUTH_TITLE_LINE2_EN;
  const description = isZh ? AUTH_DESCRIPTION_ZH : AUTH_DESCRIPTION_EN;

  useEffect(() => {
    setMounted(true);
  }, []);

  useEffect(() => {
    if (isLoading) return;
    if (!isInitialized && pathname !== '/init') {
      router.replace('/init');
    }
  }, [isInitialized, isLoading, pathname, router]);

  useEffect(() => {
    if (isLoading || !isInitialized || authLoading || !authInitialized) {
      return;
    }
    if (DISABLE_AUTHENTICATED_REDIRECT_ON_AUTH_PAGES) {
      return;
    }
    const shouldRedirectAuthenticatedUser =
      pathname.endsWith('/login') ||
      pathname.endsWith('/login/') ||
      pathname.endsWith('/register') ||
      pathname.endsWith('/register/') ||
      pathname.endsWith('/forgot-password') ||
      pathname.endsWith('/forgot-password/');

    if (!shouldRedirectAuthenticatedUser) {
      return;
    }
    if (isAuthenticated) {
      if (consumePendingLogoutRedirect()) {
        return;
      }

      sessionManager.syncRootCookiesForCurrentSession();
      const params = new URLSearchParams(window.location.search);
      const redirectUrl = withBasePathIfInternal(params.get('redirect') || '/console');
      window.location.replace(redirectUrl);
    }
  }, [authInitialized, authLoading, isAuthenticated, isInitialized, isLoading, pathname, router]);

  useEffect(() => {
    if (isLoading || !isInitialized) {
      setSingleSsoRedirectUrl(null);
      return;
    }
    if (!SINGLE_SSO_PROVIDER || !shouldAutoRedirectToSingleSso(pathname)) {
      setSingleSsoRedirectUrl(null);
      return;
    }

    const params = new URLSearchParams(window.location.search);
    const redirectTarget = resolveSingleSsoRedirectTarget(pathname, params);
    setSingleSsoRedirectUrl(buildSsoStartUrl(SINGLE_SSO_PROVIDER, redirectTarget));
  }, [isInitialized, isLoading, pathname]);

  useEffect(() => {
    if (!singleSsoRedirectUrl) {
      return;
    }

    window.location.replace(singleSsoRedirectUrl);
  }, [singleSsoRedirectUrl]);

  return (
    <div className="flex min-h-svh bg-background overflow-hidden">
      {/* --- Left Art Panel (Desktop Only) --- */}
      <div
        className={cn(
          'relative overflow-hidden bg-muted/30 border-r',
          HIDE_AUTH_LEFT_PANEL ? 'hidden' : 'hidden lg:flex lg:w-1/2'
        )}
      >
        {/* Custom Background Image or Generative Tech Background */}
        {AUTH_BG_IMAGE ? (
          <div
            className="absolute inset-0 bg-cover bg-center bg-no-repeat"
            style={{ backgroundImage: `url(${AUTH_BG_IMAGE})` }}
          />
        ) : (
          <div className="absolute inset-0 pointer-events-none overflow-hidden">
            {/* Light Orbs */}
            <div className="absolute top-[-10%] left-[-10%] w-[60%] h-[60%] rounded-full bg-primary/5 blur-[120px] animate-pulse-subtle" />
            <div className="absolute bottom-[-15%] right-[-5%] w-[50%] h-[50%] rounded-full bg-primary/10 blur-[100px]" />
            <div className="absolute top-[30%] left-[20%] w-[30%] h-[30%] rounded-full bg-primary/5 blur-[80px]" />

            {/* Minimal Grid Overlay */}
            <div
              className="absolute inset-0 opacity-[0.03]"
              style={{
                backgroundImage: `linear-gradient(var(--border) 1px, transparent 1px), linear-gradient(90deg, var(--border) 1px, transparent 1px)`,
                backgroundSize: '40px 40px',
              }}
            />

            {/* Floating Accents */}
            <div className="absolute top-[20%] right-[15%] w-1 h-20 bg-linear-to-b from-primary/20 to-transparent rounded-full" />
            <div className="absolute bottom-[30%] left-[10%] w-20 h-1 bg-linear-to-r from-primary/20 to-transparent rounded-full" />
          </div>
        )}

        {/* Content Container - Only show when no custom background image */}
        {!AUTH_BG_IMAGE && (
          <div className="relative z-10 flex flex-col justify-center h-full px-16 xl:px-24">
            <div
              className={cn(
                'flex items-center gap-4 mb-12',
                mounted ? 'animate-in fade-in slide-in-from-left-8 duration-700' : 'opacity-0'
              )}
            >
              <div className="p-2 rounded-2xl bg-background shadow-premium border glass-panel">
                <Logo showName={false} routerToHome={false} />
              </div>
            </div>

            <div className="space-y-8">
              <h1
                className={cn(
                  'text-4xl xl:text-5xl font-bold leading-tight tracking-tight',
                  mounted
                    ? 'animate-in fade-in slide-in-from-left-10 duration-700 delay-100'
                    : 'opacity-0'
                )}
              >
                {titleLine1} <br />
                <span className="text-primary">{titleLine2}</span>
              </h1>
              <p
                className={cn(
                  'text-lg text-muted-foreground max-w-md leading-relaxed',
                  mounted
                    ? 'animate-in fade-in slide-in-from-left-12 duration-700 delay-200'
                    : 'opacity-0'
                )}
              >
                {description}
              </p>
            </div>
          </div>
        )}
      </div>

      {/* --- Right Panel (Form Area) --- */}
      <div className="flex-1 flex flex-col relative overflow-y-auto">
        {/* Responsive Header (Logo on Mobile, Switcher on both) */}
        <div className="flex items-center justify-between p-8 z-20 shrink-0">
          <div
            className={cn('pointer-events-auto w-36', HIDE_AUTH_LEFT_PANEL ? 'block' : 'lg:hidden')}
          >
            <Logo showName={false} routerToHome={false} />
          </div>
          <div className="pointer-events-auto ml-auto">
            <LanguageSwitcher />
          </div>
        </div>

        {/* Form Container */}
        <div className="flex-1 flex flex-col items-center justify-center p-6 md:p-12">
          <div
            className={cn(
              'w-full max-w-[400px]',
              mounted ? 'animate-in fade-in slide-in-from-bottom-4 duration-700' : 'opacity-0'
            )}
          >
            {singleSsoRedirectUrl ? (
              <div className="glass-panel rounded-3xl border bg-card/85 px-8 py-10 text-center shadow-premium">
                <div className="mx-auto mb-5 flex h-16 w-16 items-center justify-center rounded-full bg-primary/10 text-primary">
                  <Icons.Spinner className="size-7 animate-spin" />
                </div>
                <p className="text-xl font-semibold">{t('signInWithSSO')}</p>
                <p className="mt-3 text-sm text-muted-foreground">{t('ssoProcessing')}</p>
              </div>
            ) : (
              children
            )}
          </div>
        </div>

        {/* Footer */}
        <div className="py-8 text-center text-xs text-muted-foreground opacity-60">
          &copy; {new Date().getFullYear()} {APP_NAME}. All rights reserved.
        </div>
      </div>
    </div>
  );
}
