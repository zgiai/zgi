'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Link from 'next/link';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { Bot, Brain, ListTodo, Settings2 } from 'lucide-react';
import { useStore } from 'zustand';
import { toast } from 'sonner';
import {
  AIChatShell,
  createDigitalHumanRuntimeTransport,
  useChatRuntimeController,
} from '@/components/chat';
import {
  resolveConversationRouteSync,
  shouldStartNewConversationForRoute,
  type ConversationRouteHandoff,
} from '@/components/chat/runtime/conversation-route-handoff';
import { isConversationRouteRestoring } from '@/components/chat/runtime/conversation-route-state';
import { isDraftAIChatConversationId } from '@/components/chat/utils/aichat-message';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import {
  DigitalHumanTaskPanel,
  type DigitalHumanTaskPanelClient,
} from '@/components/digital-human/digital-human-task-panel';
import { useDigitalHuman } from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';

const conversationParam = 'conversation';

export function DigitalHumanChat({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const detail = useDigitalHuman(id).data?.data;
  const permissions = useAccountPermissions();
  const canRun = permissions.hasPermission('digital_human.run');
  const [mobileTasksOpen, setMobileTasksOpen] = useState(false);
  const [taskRefreshKey, setTaskRefreshKey] = useState(0);
  const taskRefreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const published = detail?.published;
  const transport = useMemo(() => createDigitalHumanRuntimeTransport(id), [id]);
  const taskClient = useMemo<DigitalHumanTaskPanelClient>(
    () => ({
      list: params => digitalHumanService.listMyTasks(id, params),
      create: request => digitalHumanService.createMyTask(id, request),
      get: taskId => digitalHumanService.getMyTask(id, taskId),
      update: (taskId, request) => digitalHumanService.updateMyTask(id, taskId, request),
      action: (taskId, action, revision) =>
        digitalHumanService.applyMyTaskAction(id, taskId, action, revision),
      runs: (taskId, page) => digitalHumanService.listMyTaskRuns(id, taskId, page),
      activities: (taskId, page) => digitalHumanService.listMyTaskActivities(id, taskId, page),
    }),
    [id]
  );
  const controller = useChatRuntimeController({
    transport,
    requireModel: false,
    memoryMutationMode: 'disabled',
  });
  const initController = controller.init;
  const startNewConversation = controller.startNew;
  const activeConversationId = useStore(controller.store, state => state.activeConversationId);
  const initialConversationId = searchParams.get(conversationParam);
  const initControllerRef = useRef(initController);
  const startNewConversationRef = useRef(startNewConversation);
  const lastInitializedConversationIdRef = useRef<string | null | undefined>(undefined);
  const routeHandoffRef = useRef<ConversationRouteHandoff | undefined>(undefined);
  const isRestoringConversationRoute = isConversationRouteRestoring(
    initialConversationId,
    activeConversationId
  );

  const replaceConversationParam = useCallback(
    (conversationId?: string | null, nullMode: 'new-chat' | 'draft-persistence' = 'new-chat') => {
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
    initControllerRef.current = initController;
    startNewConversationRef.current = startNewConversation;
  }, [initController, startNewConversation]);

  useEffect(() => {
    if (lastInitializedConversationIdRef.current === initialConversationId) return;
    lastInitializedConversationIdRef.current = initialConversationId;
    routeHandoffRef.current = initialConversationId
      ? { conversationId: initialConversationId, mode: 'selection' }
      : {
          conversationId: null,
          mode: isDraftAIChatConversationId(activeConversationId)
            ? 'draft-persistence'
            : 'new-chat',
        };
    initControllerRef.current(initialConversationId);
    if (
      shouldStartNewConversationForRoute(
        initialConversationId,
        activeConversationId,
        isDraftAIChatConversationId(activeConversationId)
      )
    ) {
      startNewConversationRef.current();
    }
  }, [activeConversationId, initialConversationId]);

  useEffect(() => {
    const decision = resolveConversationRouteSync({
      activeConversationId,
      currentConversationId: initialConversationId,
      routeHandoff: routeHandoffRef.current,
      activeConversationIsDraft: isDraftAIChatConversationId(activeConversationId),
    });
    routeHandoffRef.current = decision.routeHandoff;

    if (decision.action.type === 'replace') {
      replaceConversationParam(decision.action.conversationId);
    } else if (decision.action.type === 'clear') {
      replaceConversationParam(null);
    }
  }, [activeConversationId, initialConversationId, replaceConversationParam]);

  const selectConversation = useCallback(
    (conversationId: string) => {
      if (!conversationId) return;
      if (conversationId === initialConversationId) {
        initControllerRef.current(conversationId);
        return;
      }
      replaceConversationParam(conversationId);
    },
    [initialConversationId, replaceConversationParam]
  );

  const handleStartNewConversation = useCallback(() => {
    startNewConversationRef.current();
    replaceConversationParam(null, 'draft-persistence');
  }, [replaceConversationParam]);

  const modelSelectorValue = useMemo(
    () => ({
      provider: '',
      model: published?.default_model ?? '',
      params: {},
    }),
    [published?.default_model]
  );

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

  if (!published) {
    return (
      <div className="mx-auto flex h-full max-w-5xl items-center justify-center px-4 py-10 md:px-8">
        <div className="max-w-lg rounded-2xl border bg-background p-8 text-center shadow-sm">
          <div className="mx-auto flex size-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
            <Bot className="size-6" />
          </div>
          <h2 className="mt-5 text-xl font-semibold">{t('chat.publishTitle')}</h2>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            {t('configuration.notPublished')}
          </p>
          <Button className="mt-6" asChild>
            <Link href={`/console/digital-humans/${id}/configuration?section=identity`}>
              <Settings2 className="mr-2 size-4" />
              {t('chat.openSettings')}
            </Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="relative flex h-full min-h-0 overflow-hidden bg-background">
      <div className="min-w-0 flex-1">
        <AIChatShell
          controller={controller}
          modelSelectorValue={modelSelectorValue}
          onModelChange={() => undefined}
          beforeSend={() => {
            if (published) return true;
            toast.error(t('configuration.notPublished'));
            return false;
          }}
          showModelSelector={false}
          variant="full"
          requireModel={false}
          showMemoryToggle={false}
          forcedUseMemory={false}
          enableUpload={published?.file_upload_policy == null || published.file_upload_policy.allowed_file_types.length > 0}
          fileUploadPolicy={published?.file_upload_policy}
          uploadScope={{ type: 'console' }}
          showFileLibraryPicker={false}
          allowWorkspaceSwitch={false}
          enableMessageReplay
          showAssistantModelMeta={false}
          surface="digital-human"
          runtimeSurface="external_page_chat"
          defaultSidebarOpen
          routeConversationId={initialConversationId}
          isConversationTransitioning={isRestoringConversationRoute}
          conversationListTitle={t('chat.conversations')}
          conversationListDescription={t('chat.conversationListDescription')}
          homeBrand={
            <div className="flex size-14 items-center justify-center rounded-2xl bg-primary/10 text-primary shadow-sm">
              <Bot className="size-7" />
            </div>
          }
          homeTitle={t('chat.homeTitle', { name: detail?.definition.name ?? t('title') })}
          homeDescription={t('chat.homeDescription')}
          suggestions={[
            t('chat.suggestions.introduce'),
            t('chat.suggestions.capabilities'),
            t('chat.suggestions.plan'),
          ]}
          inputPlaceholder={t('chat.placeholder')}
          onSelectConversation={selectConversation}
          onStartNewConversation={handleStartNewConversation}
          headerRightAction={
            <Button variant="ghost" size="sm" asChild>
              <Link
                href={`/profile/memory?consumer_type=digital_human&consumer_id=${encodeURIComponent(id)}`}
              >
                <Brain className="mr-2 size-4" />
                {t('chat.manageMemory')}
              </Link>
            </Button>
          }
          onTaskMutation={handleTaskMutation}
        />
      </div>
      {canRun ? (
        <div className="hidden w-[22rem] shrink-0 xl:block">
          <DigitalHumanTaskPanel client={taskClient} compact refreshKey={taskRefreshKey} />
        </div>
      ) : null}
      {canRun ? (
        <Button
          variant="outline"
          size="sm"
          className="absolute bottom-4 right-4 z-10 gap-2 shadow-md xl:hidden"
          onClick={() => setMobileTasksOpen(true)}
        >
          <ListTodo className="size-4" />
          {t('tasks.myTitle')}
        </Button>
      ) : null}
      {canRun ? (
        <Sheet open={mobileTasksOpen} onOpenChange={setMobileTasksOpen}>
          <SheetContent side="right" className="w-full max-w-none p-0 sm:max-w-lg">
            <SheetTitle className="sr-only">{t('tasks.myTitle')}</SheetTitle>
            <DigitalHumanTaskPanel client={taskClient} compact refreshKey={taskRefreshKey} />
          </SheetContent>
        </Sheet>
      ) : null}
    </div>
  );
}
