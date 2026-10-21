'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, ListTodo, LogIn, RefreshCw, UserCheck } from 'lucide-react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useStore } from 'zustand';
import Chat, { createDigitalHumanWebAppTransport, useAIChatController } from '@/components/chat';
import {
  DigitalHumanTaskPanel,
  type DigitalHumanTaskPanelClient,
} from '@/components/digital-human/digital-human-task-panel';
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
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { useDigitalHumanWebAppConfig } from '@/hooks/digital-human/use-digital-human-webapp';
import { useT } from '@/i18n';
import { ICON_BG } from '@/lib/config';
import type { DigitalHumanWebAppConfig } from '@/services/types/digital-human';
import { useAuthStore } from '@/store/auth-store';
import { digitalHumanWebAppService } from '@/services/digital-human-webapp.service';
import { sessionManager } from '@/lib/auth/session-manager';

const conversationParam = 'convId';

function resolveIcon(config: DigitalHumanWebAppConfig) {
  let icon = config.name.slice(0, 2).toUpperCase();
  let iconBackground = ICON_BG;
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
  const taskT = useT('digitalHumans');
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const configQuery = useDigitalHumanWebAppConfig(webAppId);
  const config = configQuery.data?.data;
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const isAuthLoading = useAuthStore.use.isLoading();
  const isAuthInitialized = useAuthStore.use.isInitialized();
  const authNetworkError = useAuthStore.use.networkError();
  const initializeAuth = useAuthStore.use.initializeAuth();
  // Anonymous public pages must not wait for the console auth bootstrap. When
  // an account session already exists, however, wait for it to be restored so
  // the first conversation is never created under an anonymous subject.
  const hasAccountSession = sessionManager.hasSession();
  const [mobileTasksOpen, setMobileTasksOpen] = useState(false);
  const [taskRefreshKey, setTaskRefreshKey] = useState(0);
  const [configLoadTimedOut, setConfigLoadTimedOut] = useState(false);
  const [authWaitTimedOut, setAuthWaitTimedOut] = useState(false);
  const taskRefreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const transport = useMemo(() => createDigitalHumanWebAppTransport(webAppId), [webAppId]);
  const modelValue = useMemo(() => ({ provider: '', model: '', params: {} }), []);
  const taskClient = useMemo<DigitalHumanTaskPanelClient>(
    () => ({
      list: params => digitalHumanWebAppService.listTasks(webAppId, params),
      create: request => digitalHumanWebAppService.createTask(webAppId, request),
      get: taskId => digitalHumanWebAppService.getTask(webAppId, taskId),
      update: (taskId, request) => digitalHumanWebAppService.updateTask(webAppId, taskId, request),
      action: (taskId, action, revision) =>
        digitalHumanWebAppService.applyTaskAction(webAppId, taskId, action, revision),
      runs: (taskId, page) => digitalHumanWebAppService.listTaskRuns(webAppId, taskId, page),
      activities: (taskId, page) => digitalHumanWebAppService.listTaskActivities(webAppId, taskId, page),
      reports: {
        list: taskId => digitalHumanWebAppService.listTaskReports(webAppId, taskId),
        review: (reportId, request) => digitalHumanWebAppService.reviewTaskReport(webAppId, reportId, request),
        readFile: (reportId, fileId, signal) => digitalHumanWebAppService.downloadTaskReportFile(webAppId, reportId, fileId, signal),
      },
    }),
    [webAppId]
  );
  const controller = useAIChatController({ transport, requireModel: false });
  const handleTaskMutation = useCallback(() => {
    if (taskRefreshTimerRef.current) clearTimeout(taskRefreshTimerRef.current);
    taskRefreshTimerRef.current = setTimeout(() => {
      setTaskRefreshKey(value => value + 1);
      taskRefreshTimerRef.current = null;
    }, 250);
  }, []);

  useEffect(
    () => () => {
      if (taskRefreshTimerRef.current) clearTimeout(taskRefreshTimerRef.current);
    },
    []
  );
  const activeConversationId = useStore(controller.store, state => state.activeConversationId);
  const routeConversationId = searchParams.get(conversationParam);
  const initControllerRef = useRef(controller.init);
  const startNewConversationRef = useRef(controller.startNew);
  const lastInitializedConversationIdRef = useRef<string | null | undefined>(undefined);
  const routeHandoffRef = useRef<ConversationRouteHandoff | undefined>(undefined);
  const authResolutionRequired = Boolean(config?.login_required || hasAccountSession);
  const authReady =
    !authResolutionRequired || (isAuthInitialized && !isAuthLoading);
  const canChat = Boolean(config && authReady && (!config.login_required || isAuthenticated));

  // A stale account session must not be allowed to hold the public page in a
  // skeleton forever. The auth client has its own request timeout, but this
  // page also needs a visible recovery path when a bootstrap promise is
  // interrupted before that timeout can settle.
  useEffect(() => {
    const waitingForConfig = configQuery.isLoading;
    if (!waitingForConfig) {
      setConfigLoadTimedOut(false);
      return;
    }

    const timeoutID = window.setTimeout(() => setConfigLoadTimedOut(true), 10_000);
    return () => window.clearTimeout(timeoutID);
  }, [configQuery.isLoading]);

  useEffect(() => {
    const waitingForAuth =
      authResolutionRequired && (!isAuthInitialized || isAuthLoading) && !authNetworkError;
    if (!waitingForAuth) {
      setAuthWaitTimedOut(false);
      return;
    }

    const timeoutID = window.setTimeout(() => setAuthWaitTimedOut(true), 10_000);
    return () => window.clearTimeout(timeoutID);
  }, [authNetworkError, authResolutionRequired, isAuthInitialized, isAuthLoading]);

  const retryAuth = useCallback(() => {
    setAuthWaitTimedOut(false);
    void initializeAuth({ force: true });
  }, [initializeAuth]);
  const suggestedQuestions = useMemo(() => {
    if (config?.suggested_questions?.length) return config.suggested_questions;
    return [
      taskT('chat.suggestions.introduce'),
      taskT('chat.suggestions.capabilities'),
      taskT('chat.suggestions.plan'),
    ];
  }, [config?.suggested_questions, taskT]);

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

  if (configQuery.isLoading) {
    if (configLoadTimedOut) {
      return (
        <DigitalHumanWebAppLoadError
          title={t('loadTimeoutTitle')}
          description={t('loadTimeoutDescription')}
          onRetry={() => {
            setConfigLoadTimedOut(false);
            void configQuery.refetch();
          }}
        />
      );
    }
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

  // Login-gated pages and pages opened with an existing account session must
  // wait for auth restoration. Anonymous pages can render immediately while
  // the global auth provider finishes its non-blocking bootstrap.
  if (config.login_required || hasAccountSession) {
    if (authNetworkError) {
      return (
        <DigitalHumanWebAppLoadError
          title={t('authLoadErrorTitle')}
          description={t('authLoadErrorDescription')}
          onRetry={retryAuth}
        />
      );
    }
    if (!isAuthInitialized || isAuthLoading) {
      if (authWaitTimedOut) {
        return (
          <DigitalHumanWebAppLoadError
            title={t('authLoadTimeoutTitle')}
            description={t('authLoadTimeoutDescription')}
            onRetry={retryAuth}
          />
        );
      }
      return <DigitalHumanWebAppLoading />;
    }
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
  const uploadScope = { type: 'digital-human-webapp' as const, webAppId };

  return (
    <div className="flex h-[100dvh] min-h-0 flex-col overflow-hidden bg-[radial-gradient(circle_at_top,_hsl(var(--primary)/0.08),_transparent_42%),hsl(var(--muted)/0.28)]">
      <PublicHeader config={config} icon={icon} onLogin={handleLogin} />
      <main className="mx-auto min-h-0 w-full max-w-none flex-1 px-2 py-2 md:px-3 md:py-3 lg:px-4">
        <div className="relative flex h-full min-h-0 overflow-hidden rounded-xl border border-border/70 bg-background/95 shadow-[0_18px_60px_-30px_hsl(var(--foreground)/0.35)] backdrop-blur md:rounded-2xl">
          {routeRestoring ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              {t('loadingConversation')}
            </div>
          ) : (
            <>
              <div className="min-w-0 flex-1 bg-background">
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
          fileUploadPolicy={config.file_upload_policy}
                  uploadScope={uploadScope}
                  showFileLibraryPicker={false}
                  allowWorkspaceSwitch={false}
                  suggestions={suggestedQuestions}
                  inputPlaceholder={config.input_placeholder || t('inputPlaceholder')}
                  embeddedConversationMode="drawer"
                  onSelectConversation={conversationId => replaceConversationRoute(conversationId)}
                  onStartNewConversation={() => {
                    startNewConversationRef.current();
                    replaceConversationRoute(null, 'draft-persistence');
                  }}
                  showAssistantModelMeta={false}
                  showSkillEventDetails={false}
                  enableMessageReplay
                  messageDensity="compact"
                  surface="digital-human"
                  runtimeSurface="external_page_chat"
                  onTaskMutation={handleTaskMutation}
                  openingGuideBrand={openingGuideBrand}
                  homeTitle={config.home_title || t('homeTitle', { name: config.name })}
                  homeDescription={config.opening_statement || config.description}
                />
              </div>
              {isAuthenticated ? (
                <div className="hidden w-[21rem] max-w-[35vw] shrink-0 border-l border-border/70 bg-card/70 xl:block">
                  <DigitalHumanTaskPanel client={taskClient} compact refreshKey={taskRefreshKey} />
                </div>
              ) : null}
              {isAuthenticated ? (
                <>
                  <Button
                    variant="outline"
                    size="sm"
                    className="absolute bottom-4 right-4 z-10 gap-2 shadow-md xl:hidden"
                    onClick={() => setMobileTasksOpen(true)}
                  >
                    <ListTodo className="size-4" />
                    {taskT('tasks.myTitle')}
                  </Button>
                  <Sheet open={mobileTasksOpen} onOpenChange={setMobileTasksOpen}>
                    <SheetContent side="right" className="w-full max-w-none p-0 sm:max-w-lg">
                      <SheetTitle className="sr-only">{taskT('tasks.myTitle')}</SheetTitle>
                      <DigitalHumanTaskPanel
                        client={taskClient}
                        compact
                        refreshKey={taskRefreshKey}
                      />
                    </SheetContent>
                  </Sheet>
                </>
              ) : null}
            </>
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

function DigitalHumanWebAppLoadError({
  title,
  description,
  onRetry,
}: {
  title: string;
  description: string;
  onRetry: () => void;
}) {
  const t = useT('digitalHumans.publicWebApp');
  return (
    <div className="flex h-[100dvh] items-center justify-center bg-muted/20 px-4">
      <div className="max-w-md rounded-2xl border bg-background p-8 text-center shadow-sm">
        <AlertCircle className="mx-auto size-10 text-destructive/70" />
        <h1 className="mt-4 text-lg font-semibold">{title}</h1>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">{description}</p>
        <Button className="mt-5 gap-2" onClick={onRetry}>
          <RefreshCw className="size-4" />
          {t('retry')}
        </Button>
      </div>
    </div>
  );
}
