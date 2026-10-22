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
import { cn } from '@/lib/utils';
import { digitalHumanService } from '@/services/digital-human.service';
import { inboxService } from '@/services/inbox.service';
import type { DigitalHumanTaskPendingChange } from '@/services/types/digital-human';

const conversationParam = 'conversation';

export function DigitalHumanChat({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  // `task` opens one of the reader's tasks in the panel, e.g. from a report.
  const focusTaskId = searchParams.get('task');
  const detail = useDigitalHuman(id).data?.data;
  const permissions = useAccountPermissions();
  const canRun = permissions.hasPermission('digital_human.run');
  const [mobileTasksOpen, setMobileTasksOpen] = useState(false);
  const [tasksWide, setTasksWide] = useState(false);
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
      reports: {
        list: taskId => digitalHumanService.listTaskReports(id, { task_id: taskId }, true),
        feed: params => digitalHumanService.listTaskReports(id, params, true),
        review: (reportId, request) => digitalHumanService.reviewTaskReport(id, reportId, request, true),
        readFile: (reportId, fileId, signal) => digitalHumanService.downloadTaskReportFile(id, reportId, fileId, true, signal),
        viewed: reportId => void inboxService.markRead({ source_type: 'digital_human_task_report', source_id: reportId }).catch(() => undefined),
      },
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

  // Brings a pending task approval into view once its conversation shows it.
  // The approval may be older than the first page of history, so older pages
  // are loaded until it appears. It may also sit on a branch the reader has
  // since switched away from, so that branch is selected first; its
  // confirmation form needs it current.
  const [approvalTarget, setApprovalTarget] = useState<DigitalHumanTaskPendingChange | null>(
    null
  );
  const controllerStore = controller.store;
  const switchBranch = controller.switchBranch;
  const loadOlderMessages = controller.loadOlderMessages;
  useEffect(() => {
    if (!approvalTarget) return;
    const { conversation_id: conversationId, message_id: messageId } = approvalTarget;
    let attempts = 0;
    let olderPagesLoaded = 0;
    let branchSelected = false;
    const timer = window.setInterval(() => {
      attempts += 1;
      const target = document.querySelector<HTMLElement>(
        `[data-chat-message-id="${CSS.escape(messageId)}"]`
      );
      if (target) {
        target.scrollIntoView({ behavior: 'smooth', block: 'center' });
      } else if (!branchSelected) {
        const state = controllerStore.getState();
        const loaded = state.messagesByConversation[conversationId]?.some(
          message => message.id === messageId
        );
        const ready = state.activeConversationId === conversationId && !state.isLoadingMessages;
        if (ready && loaded && !state.isSending) {
          branchSelected = true;
          switchBranch(messageId);
        } else if (
          ready &&
          !loaded &&
          olderPagesLoaded < 10 &&
          state.messagePaginationByConversation[conversationId]?.hasMore &&
          !state.loadingOlderByConversation[conversationId]
        ) {
          // Each loaded page restarts the wait for the approval to render.
          olderPagesLoaded += 1;
          attempts = 0;
          void loadOlderMessages(conversationId);
        }
      }
      if (target || attempts >= 40) {
        window.clearInterval(timer);
        setApprovalTarget(null);
      }
    }, 150);
    return () => window.clearInterval(timer);
  }, [approvalTarget, controllerStore, loadOlderMessages, switchBranch]);

  const openPendingChange = useCallback(
    (change: DigitalHumanTaskPendingChange) => {
      selectConversation(change.conversation_id);
      setApprovalTarget(change);
    },
    [selectConversation]
  );

  // `approval` names the message of a pending confirmation or question, e.g.
  // from a waiting run's drawer; it is handled once and then dropped.
  const approvalMessageId = searchParams.get('approval');
  useEffect(() => {
    if (!approvalMessageId || !initialConversationId) return;
    openPendingChange({ conversation_id: initialConversationId, message_id: approvalMessageId });
    const params = new URLSearchParams(window.location.search);
    params.delete('approval');
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  }, [approvalMessageId, initialConversationId, openPendingChange, pathname, router]);

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
            <div className="flex items-center gap-1">
              {canRun ? (
                // Below xl the task panel is a sheet. A floating button sat
                // under the composer on phones, so it lives in the header.
                <Button
                  variant="ghost"
                  size="sm"
                  className="xl:hidden"
                  onClick={() => setMobileTasksOpen(true)}
                  aria-label={t('tasks.myTitle')}
                  title={t('tasks.myTitle')}
                >
                  <ListTodo className="size-4 sm:mr-2" />
                  <span className="hidden sm:inline">{t('tasks.myTitle')}</span>
                </Button>
              ) : null}
              <Button variant="ghost" size="sm" asChild>
                <Link
                  href={`/profile/memory?consumer_type=digital_human&consumer_id=${encodeURIComponent(id)}`}
                >
                  <Brain className="mr-2 size-4" />
                  {t('chat.manageMemory')}
                </Link>
              </Button>
            </div>
          }
          onTaskMutation={handleTaskMutation}
        />
      </div>
      {canRun ? (
        // The panel widens while a task or report is open beside the chat.
        <div
          className={cn(
            'hidden shrink-0 border-l transition-[width] duration-200 ease-out motion-reduce:transition-none xl:block',
            tasksWide ? 'w-[36rem] 2xl:w-[40rem]' : 'w-[24rem] 2xl:w-[26rem]'
          )}
        >
          <DigitalHumanTaskPanel
            client={taskClient}
            compact
            refreshKey={taskRefreshKey}
            onOpenPendingChange={openPendingChange}
            focusTaskId={focusTaskId}
            onWideChange={setTasksWide}
          />
        </div>
      ) : null}
      {canRun ? (
        <Sheet open={mobileTasksOpen} onOpenChange={setMobileTasksOpen}>
          <SheetContent side="right" className="w-full max-w-none p-0 sm:max-w-lg">
            <SheetTitle className="sr-only">{t('tasks.myTitle')}</SheetTitle>
            <DigitalHumanTaskPanel
              client={taskClient}
              compact
              refreshKey={taskRefreshKey}
              onOpenPendingChange={change => {
                setMobileTasksOpen(false);
                openPendingChange(change);
              }}
            />
          </SheetContent>
        </Sheet>
      ) : null}
    </div>
  );
}
