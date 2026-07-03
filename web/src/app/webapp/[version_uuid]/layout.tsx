'use client';

import React from 'react';
import { LogIn, UserCheck } from 'lucide-react';
import { useParams, usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useWebAppConfig } from '@/hooks/webapp/use-webapp';
import { useMaybeMigrateUser } from '@/hooks/webapp/use-maybe-migrate-user';
import { Logo } from '@/components/logo';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import { Providers } from '@/providers';
import { useAuthStore } from '@/store/auth-store';

export default function WebappVersionLayout({ children }: { children: React.ReactNode }) {
  return (
    <Providers>
      <WebappVersionLayoutContent>{children}</WebappVersionLayoutContent>
    </Providers>
  );
}

function WebappVersionLayoutContent({ children }: { children: React.ReactNode }) {
  const { version_uuid } = useParams<{ version_uuid: string }>();
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const t = useT('webapp');
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const isAuthLoading = useAuthStore.use.isLoading();
  const isAuthInitialized = useAuthStore.use.isInitialized();
  const user = useAuthStore.use.user();
  useMaybeMigrateUser();
  const { data, isLoading } = useWebAppConfig(version_uuid);

  const meta = data?.data?.config;
  const iconType = meta?.icon_type;

  // Derive icon props consistent with AgentSidebar
  let textIcon = (meta?.title || ICON_TEXT).slice(0, 2).toUpperCase();
  let iconBackground = ICON_BG;
  let imgSrc: string | undefined = undefined;
  if (iconType === 'image') {
    imgSrc = meta?.icon_url || meta?.icon || '';
  } else if (iconType === 'text') {
    try {
      const parsed = JSON.parse(meta?.icon || '{}');
      textIcon = parsed?.icon || textIcon;
      iconBackground = parsed?.icon_background || iconBackground;
    } catch {
      // ignore parse error
    }
  } else if (meta?.icon) {
    try {
      const parsed = JSON.parse(meta.icon);
      if (parsed?.icon) textIcon = parsed.icon;
      if (parsed?.icon_background) iconBackground = parsed.icon_background;
    } catch {
      // ignore parse error
    }
  }

  const showLoginGuide = isAuthInitialized && !isAuthLoading && !isAuthenticated;
  const showSignedInStatus = isAuthInitialized && !isAuthLoading && isAuthenticated;
  const signedInName = user?.name || user?.email || t('header.signedIn');

  const handleLogin = React.useCallback(() => {
    const search = searchParams.toString();
    const currentUrl = search ? `${pathname}?${search}` : pathname;
    router.push(`/login?redirect=${encodeURIComponent(currentUrl)}`);
  }, [pathname, router, searchParams]);

  return (
    <div className="flex h-[100dvh] min-h-[100dvh] max-h-[100dvh] w-full flex-col overflow-hidden">
      {/* Webapp global header for this version */}
      <div className="w-full shrink-0 border-b bg-background/95 backdrop-blur">
        <div className="grid min-h-12 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 px-3 py-2 md:grid-cols-[1fr_auto_1fr] md:gap-3 md:px-4 md:py-1">
          <div className="hidden max-w-52 md:block">
            <Logo routerToHome={false} showName={false} />
          </div>
          <div className="flex min-w-0 items-center gap-2">
            {isLoading ? (
              <>
                <Skeleton className="h-6 w-6 rounded-md" />
                <Skeleton className="h-4 w-36" />
              </>
            ) : (
              <>
                <IconPreview
                  iconType={iconType === 'image' ? 'image' : 'text'}
                  src={iconType === 'image' ? imgSrc : ''}
                  icon={textIcon}
                  iconBackground={iconBackground}
                  editable={false}
                  size="sm"
                />
                <div className="truncate text-sm font-medium md:text-lg" title={meta?.title}>
                  {meta?.title}
                </div>
              </>
            )}
          </div>
          <div className="flex min-w-0 justify-end">
            {showLoginGuide ? (
              <div className="flex min-w-0 items-center justify-end gap-1.5 sm:gap-2">
                <span className="inline-flex h-7 shrink-0 items-center rounded-md border bg-muted/40 px-2 text-xs text-muted-foreground md:hidden">
                  {t('header.guestModeShort')}
                </span>
                <span className="hidden max-w-[min(42vw,26rem)] whitespace-normal text-right text-xs leading-4 text-muted-foreground md:inline">
                  {t('header.guestMode')}
                </span>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="h-8 shrink-0 gap-1.5 px-2.5 text-xs"
                  onClick={handleLogin}
                  title={t('header.loginHint')}
                >
                  <LogIn className="size-3.5" />
                  {t('header.login')}
                </Button>
              </div>
            ) : null}
            {showSignedInStatus ? (
              <div
                className="inline-flex min-w-0 max-w-[42vw] items-center gap-1.5 rounded-md border bg-primary/5 px-2.5 py-1.5 text-xs text-primary md:max-w-72"
                title={signedInName}
              >
                <UserCheck className="size-3.5 shrink-0" />
                <span className="shrink-0">{t('header.signedIn')}</span>
                <span className="hidden min-w-0 truncate text-primary/80 sm:inline">
                  {signedInName}
                </span>
              </div>
            ) : null}
          </div>
        </div>
      </div>

      {/* Children pages */}
      <div className="grow min-h-0 w-full overflow-hidden">{children}</div>
    </div>
  );
}
