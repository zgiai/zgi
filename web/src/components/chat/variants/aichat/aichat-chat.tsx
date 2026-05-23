'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { toast } from 'sonner';
import { useStore } from 'zustand';
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
import { useAIChatSkills } from '@/hooks/aichat/use-aichat-skills';
import { useLocale } from '@/hooks/use-locale';
import { useIsMobile } from '@/hooks/use-mobile';
import { useT } from '@/i18n/translations';
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
import { buildAIChatSkillDisplayMap } from '@/components/chat/variants/aichat/skill-display';
import { useAIChatScroll } from '@/components/chat/variants/aichat/use-aichat-scroll';
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
}

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
}: AIChatShellProps) {
  const t = useT('webapp');
  const { locale } = useLocale();
  const isMobile = useIsMobile();
  const [input, setInput] = useState('');
  const [editingMessageId, setEditingMessageId] = useState<string | null>(null);
  const [editingQuery, setEditingQuery] = useState('');
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
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
  const { data: availableSkills = [] } = useAIChatSkills();
  const skillDisplayById = useMemo(
    () => buildAIChatSkillDisplayMap(availableSkills, locale),
    [availableSkills, locale]
  );

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
  const modelMissing = !modelSelectorValue.model;
  const { bottomRef, scrollViewportRef, handleMessagesScroll } = useAIChatScroll({
    messages,
    activeMessagePagination,
    isLoadingMessages,
    isLoadingOlderMessages,
    isSending,
    loadOlderMessages: controller.loadOlderMessages,
  });

  useEffect(() => {
    if (!error) {
      lastErrorToastRef.current = null;
      return;
    }

    if (lastErrorToastRef.current === error) {
      return;
    }

    lastErrorToastRef.current = error;
    toast.error(error);
  }, [error]);

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

  const suggestions = useMemo<AIChatSuggestion[]>(
    () => [
      { text: t('chat.suggestions.email'), key: 'email' },
      { text: t('chat.suggestions.meeting'), key: 'meeting' },
      { text: t('chat.suggestions.report'), key: 'report' },
      { text: t('chat.suggestions.polish'), key: 'polish' },
    ],
    [t]
  );

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
      if (!modelSelectorValue.model) {
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
        useMemory,
      });
    },
    [controller, input, isSending, modelSelectorValue, t]
  );

  const handleRegenerate = useCallback(
    (message: AIChatMessage) => {
      const branchCount = branchNavigationByMessageId.get(message.id)?.total ?? 1;
      const canReplaceRoot = canReplaceRootMessage(message);
      if (!canReplaceRoot && (!message.parent_id || branchCount >= MAX_AICHAT_BRANCHES)) return;
      if (!modelSelectorValue.model) {
        toast.error(t('consoleChat.modelRequired'));
        return;
      }

      void controller.regenerate(message.id, {
        provider: modelSelectorValue.provider,
        model: modelSelectorValue.model,
        parameters: modelSelectorValue.params,
      });
    },
    [branchNavigationByMessageId, canReplaceRootMessage, controller, modelSelectorValue, t]
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
      if (!modelSelectorValue.model) {
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
      });
    },
    [
      branchNavigationByMessageId,
      canReplaceRootMessage,
      controller,
      editingQuery,
      isSending,
      modelSelectorValue,
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

  return (
    <div className="flex h-full w-full overflow-hidden bg-background">
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

      <main className="relative flex min-w-0 flex-1 flex-col overflow-hidden bg-background">
        <AIChatHeader
          isMobile={isMobile}
          isHome={isHome}
          title={activeConversation?.title || t('consoleChat.title')}
          onToggleSidebar={handleToggleSidebar}
          onStartNew={handleNewChat}
        />

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
        />

        <AIChatHomeView
          isVisible={isHome && !isLoadingMessages}
          suggestions={suggestions}
          onSelectSuggestion={setInput}
        />

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
        />
      </main>

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
    </div>
  );
}
