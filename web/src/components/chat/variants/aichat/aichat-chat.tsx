'use client';

import {
  createElement,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
} from 'react';
import { createPortal } from 'react-dom';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { useStore } from 'zustand';
import { ArrowDown, MessageSquarePlus, PanelLeft, Settings2 } from 'lucide-react';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import type { AIChatController } from '@/components/chat/controllers/aichat-controller';
import type { ConversationSummary } from '@/components/chat/controllers/types';
import {
  selectActiveConversation,
  selectActiveMessagePagination,
  selectActiveMessages,
  selectBranchNavigationByMessageId,
  selectDisplayMessageIds,
  selectDisplayMessages,
  selectIsRecoveringMessages,
  selectIsLoadingOlderMessages,
  selectIsStopping,
} from '@/components/chat/controllers/aichat/selectors';
import { Sidebar } from '@/components/chat/variants/common/sidebar';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { Button } from '@/components/ui/button';
import {
  useAIChatSkillPreference,
  useAIChatSkills,
  useUpdateAIChatSkillPreference,
} from '@/hooks/aichat/use-aichat-skills';
import { useLocale } from '@/hooks/use-locale';
import { useIsMobile } from '@/hooks/use-mobile';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { useWorkspaceStore } from '@/store/workspace-store';
import type { AIChatMessage, AIChatMessageFile } from '@/services/types/aichat';
import {
  buildChatMessageTopology,
  buildChatMessageTopologyKey,
  type ChatMessageTopology,
} from '@/components/chat/utils/message-tree';
import { AIChatHeader } from '@/components/chat/variants/aichat/chat-header';
import { AIChatHomeView } from '@/components/chat/variants/aichat/home-view';
import { AIChatInputArea } from '@/components/chat/variants/aichat/input-area';
import { AIChatMessageList } from '@/components/chat/variants/aichat/message-list';
import {
  buildAIChatSkillDisplayMap,
  isHiddenSystemSkill,
} from '@/components/chat/variants/aichat/skill-display';
import { AIChatSkillPreferenceDialog } from '@/components/chat/variants/aichat/skill-preference-dialog';
import { useAIChatScroll } from '@/components/chat/variants/aichat/use-aichat-scroll';
import {
  getAIChatMessageErrorInput,
  resolveAIChatErrorMessage,
} from '@/components/chat/variants/aichat/error-utils';
import {
  WorkflowBillingToastAction,
  workflowBillingToastClassNames,
} from '@/components/workflow/common/workflow-billing-toast-action';
import { AICHAT_SIDEBAR_BG_IMAGE } from '@/lib/config';
import {
  MAX_AICHAT_BRANCHES,
  type AIChatModelValue,
  type AIChatSuggestion,
} from '@/components/chat/variants/aichat/types';

export { AIChatMessageBubble } from '@/components/chat/variants/aichat/message-bubble';
export type { AIChatModelValue } from '@/components/chat/variants/aichat/types';

interface AIChatShellProps {
  controller: AIChatController;
  modelSelectorValue: AIChatModelValue;
  onModelChange: (value: ModelSelectorValue) => void;
  variant?: 'full' | 'embedded';
  showModelSelector?: boolean;
  requireModel?: boolean;
  showMemoryToggle?: boolean;
  forcedUseMemory?: boolean;
  enableUpload?: boolean;
  showFileLibraryPicker?: boolean;
  homeBrand?: React.ReactNode;
  homeTitle?: string;
  homeDescription?: string;
  suggestions?: string[];
  inputPlaceholder?: string;
  embeddedConversationMode?: 'none' | 'drawer';
  embeddedConversationControlsMode?: 'internal' | 'external';
  embeddedConversationControlsClassName?: string;
  embeddedConversationControlsPortalId?: string;
  renderEmbeddedConversationControls?: (controls: {
    openConversations: () => void;
    startNewConversation: () => void;
    isHome: boolean;
  }) => React.ReactNode;
  showAssistantModelMeta?: boolean;
  surface?: 'aichat' | 'agent-draft' | 'agent-webapp';
  themeColor?: string;
}

const CHAT_THEME_PRIMARY: Record<string, string> = {
  blue: '217 91% 60%',
  emerald: '160 84% 39%',
  violet: '262 83% 58%',
  rose: '346 77% 50%',
  amber: '38 92% 50%',
  slate: '215 20% 45%',
};

/**
 * @component AIChatShell
 * @category Feature
 * @status Stable
 * @description Standalone console AIChat interface backed by /console/api/aichat.
 * @usage Use only for /console/work/chat
 * @example
 * <AIChatShell controller={controller} modelSelectorValue={value} onModelChange={setValue} />
 */
export function AIChatShell({
  controller,
  modelSelectorValue,
  onModelChange,
  variant = 'full',
  showModelSelector = true,
  requireModel = true,
  showMemoryToggle = true,
  forcedUseMemory,
  enableUpload = true,
  showFileLibraryPicker = true,
  homeBrand,
  homeTitle,
  homeDescription,
  suggestions: configuredSuggestions,
  inputPlaceholder,
  embeddedConversationMode = 'none',
  embeddedConversationControlsMode = 'internal',
  embeddedConversationControlsClassName,
  embeddedConversationControlsPortalId,
  renderEmbeddedConversationControls,
  showAssistantModelMeta = true,
  surface = 'aichat',
  themeColor,
}: AIChatShellProps) {
  const router = useRouter();
  const t = useT('webapp');
  const tGlobal = useT();
  const { locale } = useLocale();
  const isMobile = useIsMobile();
  const isEmbedded = variant === 'embedded';
  const showEmbeddedConversationDrawer = isEmbedded && embeddedConversationMode === 'drawer';
  const themeStyle = useMemo<CSSProperties | undefined>(() => {
    const primary = themeColor ? CHAT_THEME_PRIMARY[themeColor] : undefined;
    return primary ? ({ '--primary': primary } as CSSProperties) : undefined;
  }, [themeColor]);
  const [input, setInput] = useState('');
  const [editingMessageId, setEditingMessageId] = useState<string | null>(null);
  const [editingQuery, setEditingQuery] = useState('');
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const [externalControlsPortal, setExternalControlsPortal] = useState<HTMLElement | null>(null);
  const [skillPreferenceOpen, setSkillPreferenceOpen] = useState(false);
  const [draftSkillPreferenceIds, setDraftSkillPreferenceIds] = useState<string[]>([]);
  const [inputAreaHeight, setInputAreaHeight] = useState(160);
  const topologyRef = useRef<{ key: string; topology: ChatMessageTopology } | null>(null);
  const lastErrorToastRef = useRef<string | null>(null);

  const conversations = useStore(controller.store, state => state.conversations);
  const activeConversationId = useStore(controller.store, state => state.activeConversationId);
  const activeConversation = useStore(controller.store, selectActiveConversation);
  const activeMessages = useStore(controller.store, selectActiveMessages);
  const activeMessagePagination = useStore(controller.store, selectActiveMessagePagination);
  const isLoadingMessages = useStore(controller.store, state => state.isLoadingMessages);
  const isLoadingOlderMessages = useStore(controller.store, selectIsLoadingOlderMessages);
  const isRecoveringMessages = useStore(controller.store, selectIsRecoveringMessages);
  const isStopping = useStore(controller.store, selectIsStopping);
  const isSending = useStore(controller.store, state => state.isSending);
  const streamingByMessageId = useStore(controller.store, state => state.streamingByMessageId);
  const error = useStore(controller.store, state => state.error);
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const organizationRole = useWorkspaceStore.use.permissionState().organizationRole;
  const isBillingAdmin = organizationRole === 'owner' || organizationRole === 'admin';
  const enableAIChatSkillPreference = !isEmbedded && surface !== 'agent-draft' && surface !== 'agent-webapp';
  const { data: availableSkills = [] } = useAIChatSkills();
  const { data: skillPreference, isLoading: isLoadingSkillPreference } =
    useAIChatSkillPreference({ enabled: enableAIChatSkillPreference });
  const updateSkillPreference = useUpdateAIChatSkillPreference();
  const skillDisplayById = useMemo(
    () => buildAIChatSkillDisplayMap(availableSkills, locale),
    [availableSkills, locale]
  );
  const aichatConfigurableSkills = useMemo(
    () =>
      availableSkills.filter(skill => {
        if (!skill.enabled || skill.status === 'invalid') return false;
        if (isHiddenSystemSkill(skill.skill_id)) return false;
        const callers = skill.supported_callers ?? [];
        return callers.length === 0 || callers.includes('aichat');
      }),
    [availableSkills]
  );

  useEffect(() => {
    if (!enableAIChatSkillPreference || !skillPreference) return;
    setDraftSkillPreferenceIds(skillPreference.enabled_skill_ids ?? []);
  }, [enableAIChatSkillPreference, skillPreference]);

  const messageTopologyKey = useMemo(
    () => buildChatMessageTopologyKey(activeMessages),
    [activeMessages]
  );
  const messageTopology = useMemo(() => {
    if (topologyRef.current?.key === messageTopologyKey) {
      return topologyRef.current.topology;
    }

    const topology = buildChatMessageTopology(activeMessages);
    topologyRef.current = { key: messageTopologyKey, topology };
    return topology;
  }, [activeMessages, messageTopologyKey]);
  const displayMessageIds = useMemo(
    () => selectDisplayMessageIds(activeConversation, messageTopology),
    [activeConversation, messageTopology]
  );
  const messages = useMemo(
    () => selectDisplayMessages(displayMessageIds, activeMessages),
    [activeMessages, displayMessageIds]
  );
  const branchNavigationByMessageId = useMemo(
    () => selectBranchNavigationByMessageId(displayMessageIds, messageTopology),
    [displayMessageIds, messageTopology]
  );
  const isHome = !activeConversationId && messages.length === 0 && !isSending;
  const modelMissing = requireModel && !modelSelectorValue.model;
  const {
    bottomRef,
    scrollViewportRef,
    handleMessagesScroll,
    isAutoFollowPaused,
    resumeAutoFollow,
  } = useAIChatScroll({
    messages,
    activeMessagePagination,
    isLoadingMessages,
    isLoadingOlderMessages,
    isSending,
    loadOlderMessages: controller.loadOlderMessages,
  });
  const hasActiveStreamingMessage = useMemo(
    () =>
      Object.values(streamingByMessageId).some(
        streaming =>
          streaming.status === 'streaming' && streaming.conversation_id === activeConversationId
      ),
    [activeConversationId, streamingByMessageId]
  );
  const showResumeScrollButton = isAutoFollowPaused && (isSending || hasActiveStreamingMessage);

  useEffect(() => {
    if (!error) {
      lastErrorToastRef.current = null;
      return;
    }

    const matchingErrorMessage = [...activeMessages]
      .reverse()
      .find(message => message.status === 'error' && message.error === error);
    const errorInput = matchingErrorMessage
      ? getAIChatMessageErrorInput(matchingErrorMessage)
      : { message: error };
    const resolvedError = resolveAIChatErrorMessage(
      (key, values) => tGlobal(key as never, values),
      errorInput,
      {
        isAdmin: isBillingAdmin,
        workspaceId: currentWorkspace?.id,
      }
    );
    const toastKey = `${resolvedError.code ?? 'unknown'}:${error}`;

    if (lastErrorToastRef.current === toastKey) {
      return;
    }

    lastErrorToastRef.current = toastKey;
    const toastFn = resolvedError.isBilling ? toast.warning : toast.error;
    toastFn(resolvedError.title || resolvedError.description, {
      id: resolvedError.code ? `aichat-billing-${resolvedError.code}` : undefined,
      description: resolvedError.title ? resolvedError.description : undefined,
      classNames: resolvedError.isBilling ? workflowBillingToastClassNames : undefined,
      action:
        isBillingAdmin && resolvedError.href && resolvedError.actionLabel
          ? createElement(WorkflowBillingToastAction, {
              label: resolvedError.actionLabel,
              onClick: () => router.push(resolvedError.href as string),
            })
          : undefined,
    });
  }, [activeMessages, currentWorkspace?.id, error, isBillingAdmin, router, tGlobal]);

  const conversationSummaries = useMemo<ConversationSummary[]>(
    () =>
      conversations.map(conversation => ({
        id: conversation.id,
        conversationId: conversation.id,
        title: conversation.title,
        dialogueCount: conversation.dialogue_count,
        updatedAt: conversation.updated_at * 1000,
        status: conversation.status,
        metadata: {
          source: conversation.source,
          current_leaf_message_id: conversation.current_leaf_message_id,
          runtime_status: conversation.runtime_status,
          active_message_id: conversation.active_message_id,
          isRecovering:
            conversation.id === activeConversationId
              ? isRecoveringMessages
              : conversation.runtime_status === 'streaming',
          isStopping: conversation.id === activeConversationId ? isStopping : false,
        },
      })),
    [activeConversationId, conversations, isRecoveringMessages, isStopping]
  );

  const suggestions = useMemo<AIChatSuggestion[]>(() => {
    if (configuredSuggestions) {
      return configuredSuggestions
        .map(text => text.trim())
        .filter(Boolean)
        .slice(0, 6)
        .map((text, index) => ({ text, key: `configured-${index}` }));
    }

    return [
      { text: t('chat.suggestions.email'), key: 'email' },
      { text: t('chat.suggestions.meeting'), key: 'meeting' },
      { text: t('chat.suggestions.report'), key: 'report' },
      { text: t('chat.suggestions.polish'), key: 'polish' },
    ];
  }, [configuredSuggestions, t]);

  const canReplaceRootMessage = useCallback(
    (message: AIChatMessage) => {
      const conversation = activeConversation;
      return Boolean(
        conversation &&
          conversation.runtime_status === 'idle' &&
          conversation.dialogue_count === 1 &&
          !message.parent_id &&
          conversation.current_leaf_message_id === message.id &&
          (message.status === 'completed' ||
            message.status === 'stopped' ||
            message.status === 'error') &&
          activeMessages.length === 1
      );
    },
    [activeConversation, activeMessages.length]
  );

  const handleToggleSidebar = useCallback(() => {
    if (isMobile) {
      setMobileSidebarOpen(true);
      return;
    }
    setSidebarOpen(value => !value);
  }, [isMobile]);

  const handleSend = useCallback(
    (files: AIChatMessageFile[] = [], useMemory = false) => {
      const query = input.trim();
      if (!query || isSending) return;
      if (requireModel && !modelSelectorValue.model) {
        toast.error(t('consoleChat.modelRequired'));
        return;
      }

      setInput('');
      void controller.send({
        query,
        files,
        model: {
          provider: modelSelectorValue.provider,
          model: modelSelectorValue.model,
          parameters: modelSelectorValue.params,
        },
        useMemory: forcedUseMemory ?? useMemory,
      });
    },
    [controller, forcedUseMemory, input, isSending, modelSelectorValue, requireModel, t]
  );

  const handleRegenerate = useCallback(
    (message: AIChatMessage) => {
      const branchCount = branchNavigationByMessageId.get(message.id)?.total ?? 1;
      const canReplaceRoot = canReplaceRootMessage(message);
      if (!canReplaceRoot && (!message.parent_id || branchCount >= MAX_AICHAT_BRANCHES)) return;
      if (requireModel && !modelSelectorValue.model) {
        toast.error(t('consoleChat.modelRequired'));
        return;
      }

      void controller.regenerate(message.id, {
        provider: modelSelectorValue.provider,
        model: modelSelectorValue.model,
        parameters: modelSelectorValue.params,
      });
    },
    [branchNavigationByMessageId, canReplaceRootMessage, controller, modelSelectorValue, requireModel, t]
  );

  const handleEditStart = useCallback(
    (message: AIChatMessage) => {
      const branchCount = branchNavigationByMessageId.get(message.id)?.total ?? 1;
      const canReplaceRoot = canReplaceRootMessage(message);
      if (!canReplaceRoot && (!message.parent_id || branchCount >= MAX_AICHAT_BRANCHES)) return;
      setEditingMessageId(message.id);
      setEditingQuery(message.query);
    },
    [branchNavigationByMessageId, canReplaceRootMessage]
  );

  const handleEditCancel = useCallback(() => {
    setEditingMessageId(null);
    setEditingQuery('');
  }, []);

  const handleEditSubmit = useCallback(
    (message: AIChatMessage) => {
      const query = editingQuery.trim();
      const branchCount = branchNavigationByMessageId.get(message.id)?.total ?? 1;
      const canReplaceRoot = canReplaceRootMessage(message);
      if (
        !query ||
        isSending ||
        (!canReplaceRoot && (!message.parent_id || branchCount >= MAX_AICHAT_BRANCHES))
      ) {
        return;
      }
      if (requireModel && !modelSelectorValue.model) {
        toast.error(t('consoleChat.modelRequired'));
        return;
      }

      setEditingMessageId(null);
      setEditingQuery('');
      if (canReplaceRoot) {
        void controller.replaceRootMessage({
          messageId: message.id,
          query,
          model: {
            provider: modelSelectorValue.provider,
            model: modelSelectorValue.model,
            parameters: modelSelectorValue.params,
          },
        });
        return;
      }

      void controller.send({
        query,
        parentId: message.parent_id,
        model: {
          provider: modelSelectorValue.provider,
          model: modelSelectorValue.model,
          parameters: modelSelectorValue.params,
        },
        useMemory: Boolean(message.metadata?.use_memory),
      });
    },
    [
      branchNavigationByMessageId,
      canReplaceRootMessage,
      controller,
      editingQuery,
      isSending,
      modelSelectorValue,
      requireModel,
      t,
    ]
  );

  const handleSwitchBranch = useCallback(
    (messageId: string) => {
      setEditingMessageId(null);
      setEditingQuery('');
      controller.switchBranch(messageId);
    },
    [controller]
  );

  const handleNewChat = useCallback(() => {
    if (isHome) {
      toast.info(t('chat.alreadyInDraft'));
      setMobileSidebarOpen(false);
      return;
    }
    controller.startNew();
    setMobileSidebarOpen(false);
  }, [controller, isHome, t]);

  const handleSelectConversation = useCallback(
    (id: string) => {
      void controller.select(id);
      setMobileSidebarOpen(false);
    },
    [controller]
  );

  const handleDeleteConversation = useCallback(
    (id: string) => {
      void controller.remove(id);
      setMobileSidebarOpen(false);
    },
    [controller]
  );

  const handleRenameConversation = useCallback(
    async (id: string, title: string) => {
      await controller.rename(id, title);
    },
    [controller]
  );

  const handleToggleSkillPreference = useCallback((skillId: string, checked: boolean) => {
    setDraftSkillPreferenceIds(current =>
      checked ? Array.from(new Set([...current, skillId])) : current.filter(id => id !== skillId)
    );
  }, []);

  const handleSaveSkillPreference = useCallback(() => {
    updateSkillPreference.mutate(
      { payload: { enabled_skill_ids: draftSkillPreferenceIds } },
      {
        onSuccess: () => {
          setSkillPreferenceOpen(false);
          toast.success(t('consoleChat.skillPreferences.saved'));
        },
        onError: error => {
          toast.error(
            error instanceof Error
              ? error.message
              : t('consoleChat.skillPreferences.saveFailed')
          );
        },
      }
    );
  }, [draftSkillPreferenceIds, t, updateSkillPreference]);

  const embeddedConversationControls = useMemo(() => {
    if (!showEmbeddedConversationDrawer) return null;
    const controls = {
      openConversations: () => setMobileSidebarOpen(true),
      startNewConversation: handleNewChat,
      isHome,
    };
    if (renderEmbeddedConversationControls) {
      return renderEmbeddedConversationControls(controls);
    }
    return (
      <div className="flex items-center gap-1 rounded-full border bg-background/90 p-1 shadow-sm backdrop-blur">
        <Button
          variant="ghost"
          isIcon
          className="size-8 text-muted-foreground"
          onClick={controls.openConversations}
          title={t('consoleChat.toggleSidebar')}
        >
          <PanelLeft className="size-4" />
        </Button>
        <Button
          variant="ghost"
          isIcon
          className="size-8 text-muted-foreground"
          onClick={controls.startNewConversation}
          title={t('chat.newConversation')}
        >
          <MessageSquarePlus className="size-4" />
        </Button>
      </div>
    );
  }, [handleNewChat, isHome, renderEmbeddedConversationControls, showEmbeddedConversationDrawer, t]);

  useEffect(() => {
    if (
      embeddedConversationControlsMode !== 'external' ||
      !embeddedConversationControlsPortalId ||
      typeof document === 'undefined'
    ) {
      setExternalControlsPortal(null);
      return;
    }
    setExternalControlsPortal(document.getElementById(embeddedConversationControlsPortalId));
  }, [embeddedConversationControlsMode, embeddedConversationControlsPortalId]);

  return (
    <div className="flex h-full w-full overflow-hidden bg-background" style={themeStyle}>
      {!isEmbedded ? (
        <div className="hidden md:block">
          <Sidebar
            activeId={activeConversationId}
            conversations={conversationSummaries}
            isOpen={sidebarOpen}
            isHome={isHome}
            onNewChat={handleNewChat}
            onSelect={handleSelectConversation}
            onDelete={handleDeleteConversation}
            onRename={handleRenameConversation}
            backgroundImage={AICHAT_SIDEBAR_BG_IMAGE}
          />
        </div>
      ) : null}

      <main className="relative flex min-w-0 flex-1 flex-col overflow-hidden bg-background">
        {!isEmbedded ? (
          <AIChatHeader
            isMobile={isMobile}
            isHome={isHome}
            title={activeConversation?.title || t('consoleChat.title')}
            onToggleSidebar={handleToggleSidebar}
            onStartNew={handleNewChat}
            rightAction={enableAIChatSkillPreference ? (
              <Button
                variant="ghost"
                isIcon
                className="size-8 text-muted-foreground"
                onClick={() => setSkillPreferenceOpen(true)}
                title={t('consoleChat.skillPreferences.action')}
              >
                <Settings2 className="size-4" />
              </Button>
            ) : undefined}
          />
        ) : null}

        {showEmbeddedConversationDrawer && embeddedConversationControlsMode === 'internal' ? (
          <div
            className={cn(
              'absolute z-30',
              embeddedConversationControlsClassName ?? 'left-3 top-3'
            )}
          >
            {embeddedConversationControls}
          </div>
        ) : null}

        {externalControlsPortal && embeddedConversationControls
          ? createPortal(embeddedConversationControls, externalControlsPortal)
          : null}

        <AIChatMessageList
          messages={messages}
          activeConversation={activeConversation}
          activeMessageCount={activeMessages.length}
          branchNavigationByMessageId={branchNavigationByMessageId}
          isLoadingMessages={isLoadingMessages}
          isLoadingOlderMessages={isLoadingOlderMessages}
          isSending={isSending}
          streamingByMessageId={streamingByMessageId}
          skillDisplayById={skillDisplayById}
          editingMessageId={editingMessageId}
          editingQuery={editingQuery}
          bottomRef={bottomRef}
          scrollViewportRef={scrollViewportRef}
          bottomSpacerHeight={Math.max(inputAreaHeight + 72, 180)}
          onScroll={handleMessagesScroll}
          onRegenerate={handleRegenerate}
          onSwitchBranch={handleSwitchBranch}
          onEditStart={handleEditStart}
          onEditChange={setEditingQuery}
          onEditCancel={handleEditCancel}
          onEditSubmit={handleEditSubmit}
          showAssistantModelMeta={showAssistantModelMeta}
        />

        <AIChatHomeView
          isVisible={isHome && !isLoadingMessages}
          suggestions={suggestions}
          onSelectSuggestion={setInput}
          brand={homeBrand}
          title={homeTitle}
          description={homeDescription}
        />

        {showResumeScrollButton ? (
          <Button
            type="button"
            size="sm"
            variant="secondary"
            className="absolute left-1/2 z-30 -translate-x-1/2 rounded-full border bg-background/95 px-3 shadow-lg backdrop-blur"
            style={{ bottom: Math.max(inputAreaHeight + 18, 96) }}
            onClick={resumeAutoFollow}
          >
            <ArrowDown className="mr-1.5 size-4" />
            {t('consoleChat.resumeAutoScroll')}
          </Button>
        ) : null}

        <AIChatInputArea
          isHome={isHome}
          isLoadingMessages={isLoadingMessages}
          input={input}
          modelSelectorValue={modelSelectorValue}
          modelMissing={modelMissing}
          isSending={isSending}
          isStopping={isStopping}
          onInputChange={setInput}
          onSend={handleSend}
          onStop={controller.stop}
          onModelChange={onModelChange}
          onHeightChange={setInputAreaHeight}
          showModelSelector={showModelSelector}
          showMemoryToggle={showMemoryToggle}
          enableUpload={enableUpload}
          showFileLibraryPicker={showFileLibraryPicker}
          inputPlaceholder={inputPlaceholder}
        />
      </main>

      {!isEmbedded || showEmbeddedConversationDrawer ? (
        <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
          <SheetContent side="left" className="max-w-none p-0 sm:max-w-sm" showClose={false}>
            <SheetTitle className="sr-only">{t('chat.conversations')}</SheetTitle>
            <Sidebar
              activeId={activeConversationId}
              conversations={conversationSummaries}
              isOpen
              isHome={isHome}
              className="w-full border-r-0"
              onNewChat={handleNewChat}
              onSelect={handleSelectConversation}
              onDelete={handleDeleteConversation}
              onRename={handleRenameConversation}
              backgroundImage={AICHAT_SIDEBAR_BG_IMAGE}
              onClose={() => setMobileSidebarOpen(false)}
            />
          </SheetContent>
        </Sheet>
      ) : null}

      {enableAIChatSkillPreference ? (
        <AIChatSkillPreferenceDialog
          open={skillPreferenceOpen}
          locale={locale}
          skills={aichatConfigurableSkills}
          selectedSkillIds={draftSkillPreferenceIds}
          isLoading={isLoadingSkillPreference}
          isSaving={updateSkillPreference.isPending}
          onOpenChange={setSkillPreferenceOpen}
          onToggleSkill={handleToggleSkillPreference}
          onSave={handleSaveSkillPreference}
        />
      ) : null}
    </div>
  );
}
