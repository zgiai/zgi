'use client';

import { useCallback, useEffect, useMemo, useRef } from 'react';
import { AlertCircle, LogIn, UserCheck } from 'lucide-react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useStore } from 'zustand';
import Chat, { createDigitalHumanWebAppTransport, useAIChatController } from '@/components/chat';
import type { OpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';
import {
  resolveConversationRouteSync,
  shouldStartNewConversationForRoute,
  type ConversationRouteHandoff,
} from '@/components/chat/runtime/conversation-route-handoff';
import { isConversationRouteRestoring } from '@/components/chat/runtime/conversation-route-state';
import { isDraftAIChatConversationId } from '@/components/chat/utils/aichat-message';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useDigitalHumanWebAppConfig } from '@/hooks/digital-human/use-digital-human-webapp';
import { useT } from '@/i18n';
import { ICON_BG } from '@/lib/config';
import type { DigitalHumanWebAppConfig } from '@/services/types/digital-human';
import { useAuthStore } from '@/store/auth-store';

const conversationParam = 'convId';

function resolveIcon(config: DigitalHumanWebAppConfig) {
  let icon = config.name.slice(0, 2).toUpperCase();
  let iconBackground = config.theme_color || ICON_BG;
  if (config.icon_type === 'text' && config.icon) {
    try {
      const parsed = JSON.parse(config.icon) as { icon?: string; icon_background?: string };
      icon = parsed.icon || icon;
      iconBackground = parsed.icon_background || iconBackground;
    } catch {
      // Older records may contain a plain text icon.
      icon = config.icon.slice(0, 2) || icon;
    }
  }
  return {
    iconType: config.icon_type,
    icon,
    iconBackground,
    src: config.icon_type === 'image' ? config.icon_url || config.icon : '',
  } as const;
}

export function DigitalHumanWebApp({ webAppId }: { webAppId: string }) {
  const t = useT('digitalHumans.publicWebApp');
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const configQuery = useDigitalHumanWebAppConfig(webAppId);
  const config = configQuery.data?.data;
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const isAuthLoading = useAuthStore.use.isLoading();
  const isAuthInitialized = useAuthStore.use.isInitialized();
  const transport = useMemo(() => createDigitalHumanWebAppTransport(webAppId), [webAppId]);
  const controller = useAIChatController({ transport, requireModel: false });
  const activeConversationId = useStore(controller.store, state => state.activeConversationId);
  const routeConversationId = searchParams.get(conversationParam);
  const initControllerRef = useRef(controller.init);
  const startNewConversationRef = useRef(controller.startNew);
  const lastInitializedConversationIdRef = useRef<string | null | undefined>(undefined);
  const routeHandoffRef = useRef<ConversationRouteHandoff | undefined>(undefined);
  const canChat = Boolean(
    config && isAuthInitialized && !isAuthLoading && (!config.login_required || isAuthenticated)
  );

  const replaceConversationRoute = useCallback(
    (conversationId: string | null, nullMode: 'new-chat' | 'draft-persistence' = 'new-chat') => {
      const params = new URLSearchParams(searchParams.toString());
      routeHandoffRef.current = conversationId
        ? { conversationId, mode: 'selection' }
        : { conversationId: null, mode: nullMode };
      if (conversationId) params.set(conversationParam, conversationId);
      else params.delete(conversationParam);
      const query = params.toString();
      router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams]
  );

  useEffect(() => {
    initControllerRef.current = controller.init;
    startNewConversationRef.current = controller.startNew;
  }, [controller.init, controller.startNew]);

  useEffect(() => {
    if (!canChat || lastInitializedConversationIdRef.current === routeConversationId) return;
    lastInitializedConversationIdRef.current = routeConversationId;
    routeHandoffRef.current = routeConversationId
      ? { conversationId: routeConversationId, mode: 'selection' }
      : {
          conversationId: null,
          mode: isDraftAIChatConversationId(activeConversationId)
            ? 'draft-persistence'
            : 'new-chat',
        };
    initControllerRef.current(routeConversationId);
    if (
      shouldStartNewConversationForRoute(
        routeConversationId,
        activeConversationId,
        isDraftAIChatConversationId(activeConversationId)
      )
    ) {
      startNewConversationRef.current();
    }
  }, [activeConversationId, canChat, routeConversationId]);

  useEffect(() => {
    if (!canChat) return;
    const decision = resolveConversationRouteSync({
      activeConversationId,
      currentConversationId: routeConversationId,
      routeHandoff: routeHandoffRef.current,
      activeConversationIsDraft: isDraftAIChatConversationId(activeConversationId),
    });
    routeHandoffRef.current = decision.routeHandoff;
    if (decision.action.type === 'replace') {
      replaceConversationRoute(decision.action.conversationId);
    } else if (decision.action.type === 'clear') {
      replaceConversationRoute(null);
    }
  }, [activeConversationId, canChat, replaceConversationRoute, routeConversationId]);

  const handleLogin = useCallback(() => {
    const query = searchParams.toString();
    const currentURL = query ? `${pathname}?${query}` : pathname;
    router.push(`/login?redirect=${encodeURIComponent(currentURL)}`);
  }, [pathname, router, searchParams]);

  if (configQuery.isLoading || !isAuthInitialized || isAuthLoading) {
    return <DigitalHumanWebAppLoading />;
  }
  if (configQuery.error || !config) {
    return (
      <div className="flex h-[100dvh] items-center justify-center bg-muted/20 px-4">
        <div className="max-w-md rounded-2xl border bg-background p-8 text-center shadow-sm">
          <AlertCircle className="mx-auto size-10 text-destructive/70" />
          <h1 className="mt-4 text-lg font-semibold">{t('unavailableTitle')}</h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            {t('unavailableDescription')}
          </p>
        </div>
      </div>
    );
  }

  const icon = resolveIcon(config);
  if (config.login_required && !isAuthenticated) {
    return (
      <div className="flex h-[100dvh] flex-col bg-muted/20">
        <PublicHeader config={config} icon={icon} onLogin={handleLogin} />
        <main className="flex min-h-0 flex-1 items-center justify-center px-4">
          <div className="flex max-w-md flex-col items-center rounded-2xl border bg-background p-8 text-center shadow-sm">
            <IconPreview {...icon} editable={false} size="lg" />
            <h1 className="mt-5 text-lg font-semibold">{t('loginRequiredTitle')}</h1>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              {t('loginRequiredDescription')}
            </p>
            <Button className="mt-5 gap-2" onClick={handleLogin}>
              <LogIn className="size-4" />
              {t('login')}
            </Button>
          </div>
        </main>
      </div>
    );
  }

  const routeRestoring = isConversationRouteRestoring(routeConversationId, activeConversationId);
  const openingGuideBrand: OpeningGuideBrand = {
    title: config.name,
    iconType: icon.iconType,
    icon: icon.icon,
    iconBackground: icon.iconBackground,
    iconSrc: icon.src,
  };
  const modelValue = { provider: '', model: '', params: {} };
  const uploadScope = { type: 'digital-human-webapp' as const, webAppId };

  return (
    <div className="flex h-[100dvh] min-h-0 flex-col overflow-hidden bg-muted/20">
      <PublicHeader config={config} icon={icon} onLogin={handleLogin} />
      <main className="min-h-0 flex-1 md:p-3">
        <div className="h-full min-h-0 overflow-hidden bg-background md:rounded-xl md:border md:shadow-sm">
          {routeRestoring ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              {t('loadingConversation')}
            </div>
          ) : (
            <Chat
              mode="aichat"
              controller={controller}
              modelSelectorValue={modelValue}
              onModelChange={() => undefined}
              beforeSend={() => true}
              variant="full"
              showModelSelector={false}
              requireModel={false}
              showMemoryToggle={false}
              forcedUseMemory={false}
              enableUpload={config.file_upload_enabled && isAuthenticated}
              uploadScope={uploadScope}
              showFileLibraryPicker={false}
              allowWorkspaceSwitch={false}
              suggestions={config.suggested_questions}
              inputPlaceholder={config.input_placeholder || t('inputPlaceholder')}
              embeddedConversationMode="drawer"
              onSelectConversation={conversationId => replaceConversationRoute(conversationId)}
              onStartNewConversation={() => {
                startNewConversationRef.current();
                replaceConversationRoute(null, 'draft-persistence');
              }}
              showAssistantModelMeta={false}
              enableMessageReplay={false}
              surface="digital-human"
              runtimeSurface="external_page_chat"
              openingGuideBrand={openingGuideBrand}
              homeTitle={config.home_title || t('homeTitle', { name: config.name })}
              homeDescription={config.opening_statement || config.description}
              themeColor={config.theme_color || undefined}
            />
          )}
        </div>
      </main>
    </div>
  );
}

function PublicHeader({
  config,
  icon,
  onLogin,
}: {
  config: DigitalHumanWebAppConfig;
  icon: ReturnType<typeof resolveIcon>;
  onLogin: () => void;
}) {
  const t = useT('digitalHumans.publicWebApp');
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const user = useAuthStore.use.user();
  return (
    <header className="flex min-h-14 shrink-0 items-center justify-between gap-3 border-b bg-background/95 px-3 backdrop-blur md:px-5">
      <div className="flex min-w-0 items-center gap-2.5">
        <IconPreview {...icon} editable={false} size="sm" />
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold md:text-base">{config.name}</div>
          <div className="hidden truncate text-xs text-muted-foreground sm:block">
            {config.description}
          </div>
        </div>
      </div>
      {isAuthenticated ? (
        <div className="inline-flex max-w-[45vw] items-center gap-1.5 rounded-md border bg-primary/5 px-2.5 py-1.5 text-xs text-primary">
          <UserCheck className="size-3.5 shrink-0" />
          <span className="truncate">{user?.name || user?.email || t('signedIn')}</span>
        </div>
      ) : (
        <Button variant="outline" size="sm" className="gap-1.5" onClick={onLogin}>
          <LogIn className="size-3.5" />
          {t('login')}
        </Button>
      )}
    </header>
  );
}

function DigitalHumanWebAppLoading() {
  return (
    <div className="flex h-[100dvh] flex-col bg-muted/20">
      <div className="flex h-14 items-center gap-3 border-b bg-background px-4">
        <Skeleton className="size-8 rounded-lg" />
        <Skeleton className="h-4 w-40" />
      </div>
      <div className="min-h-0 flex-1 p-3">
        <Skeleton className="h-full w-full rounded-xl" />
      </div>
    </div>
  );
}
