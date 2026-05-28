'use client';

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import type { ChatController } from '@/components/chat/controllers/types';
import { useChatStore } from '@/components/chat/store';
import ConversationBox from '@/components/chat/ui/conversation-box';
import UserInput from '@/components/chat/ui/user-input';
import type { ChatAttachment, Message } from '@/components/chat/types';
import type { WorkflowFeatures } from '@/components/workflow/store/type';
import type { InputVar } from '@/components/workflow/types/input-var';
import type { Conversation } from '@/components/chat/types';
import { useT } from '@/lib/i18n';
// Skeleton moved to ConversationBox
import ConversationHistoryList from '@/components/chat/ui/conversation-history-list';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { Menu, Plus } from 'lucide-react';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import type { WebAppWorkflowMeta } from '@/services/types/webapp';
import { useStore } from 'zustand';
import { ICON_BG, ICON_TEXT, WEBAPP_CHAT_SIDEBAR_BG_IMAGE } from '@/lib/config';
import type { OpeningGuideConfig } from '@/utils/webapp/opening-statement';
import { cn } from '@/lib/utils';
import type { WorkflowFileUploadAccessMode } from '@/components/workflow/common/workflow-input-form';
import type { OpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';

interface ChatWithControllerProps {
  controller: ChatController;
  renderHeader?: (c: Conversation) => React.ReactNode;
  className?: string;
  enableUpload?: boolean;
  features?: Pick<WorkflowFeatures, 'file_upload' | 'retriever_resource'>;
  inputDisabled?: boolean;
  /** Custom overlay content to show when input is disabled */
  inputDisabledOverlay?: React.ReactNode;
  sendDisabled?: boolean;
  placeholder?: string;
  openingGuide?: OpeningGuideConfig;
  openingGuideBrand?: OpeningGuideBrand;
  suggestions?: string[];
  suggestionsTitle?: string;
  toolbarForm?: {
    variables: InputVar[];
    initialValues?: Record<string, unknown>;
    icon?: React.ReactNode;
    title?: string;
  };
  showWorkflowNodeDetail?: boolean;
  showWorkflowRunHeader?: boolean;
  showWorkflowDetail?: boolean;
  hideCompletedWorkflowDetail?: boolean;
  allowWorkflowDetailExpand?: boolean;
  defaultWorkflowDetailOpen?: boolean;
  webappMeta?: WebAppWorkflowMeta;
  historyWindowSize?: number;
  /** Extra inputs to merge when sending messages (e.g., model_config) */
  extraInputs?: Record<string, unknown>;
  /** Callback when stop button is clicked */
  onStop?: () => void;
  /** Whether workflow is currently running */
  isRunning?: boolean;
  /** Whether stop action is in progress */
  isStopping?: boolean;
  inputTopNotice?: React.ReactNode;
  inputReplacement?: React.ReactNode;
  allowPendingQuestionInput?: boolean;
  uploadAccessMode?: WorkflowFileUploadAccessMode;
  allowWorkspaceSwitch?: boolean;
  renderMessageAddon?: (message: Message) => React.ReactNode;
  surface?: 'default' | 'webapp';
}

const ChatWithController: React.FC<ChatWithControllerProps> = ({
  controller,
  renderHeader,
  className,
  enableUpload = true,
  features,
  inputDisabled,
  inputDisabledOverlay,
  sendDisabled,
  placeholder,
  openingGuide,
  openingGuideBrand,
  suggestions,
  suggestionsTitle,
  toolbarForm,
  showWorkflowNodeDetail,
  showWorkflowRunHeader,
  showWorkflowDetail,
  hideCompletedWorkflowDetail,
  allowWorkflowDetailExpand,
  defaultWorkflowDetailOpen,
  webappMeta,
  historyWindowSize,
  extraInputs,
  onStop,
  isStopping,
  inputTopNotice,
  inputReplacement,
  allowPendingQuestionInput = false,
  uploadAccessMode = 'enabled',
  allowWorkspaceSwitch = false,
  renderMessageAddon,
  surface = 'default',
}) => {
  const t = useT();
  const isWebappSurface = surface === 'webapp';

  // Mobile drawer state for conversation history
  const [mobileDrawerOpen, setMobileDrawerOpen] = useState<boolean>(false);
  const [draftSuggestion, setDraftSuggestion] = useState<{ id: number; text: string } | null>(null);

  // Derive icon props from webapp meta for mobile header
  const iconType = webappMeta?.icon_type;
  let textIcon = (webappMeta?.title || ICON_TEXT).slice(0, 2).toUpperCase();
  let iconBackground = ICON_BG;
  let imgSrc: string | undefined = undefined;
  if (iconType === 'image') {
    imgSrc = webappMeta?.icon_url || webappMeta?.icon || '';
  } else if (iconType === 'text') {
    try {
      const parsed = JSON.parse(webappMeta?.icon || '{}');
      textIcon = parsed?.icon || textIcon;
      iconBackground = parsed?.icon_background || iconBackground;
    } catch {
      /* ignore parse error */
    }
  } else if (webappMeta?.icon) {
    try {
      const parsed = JSON.parse(webappMeta.icon);
      if (parsed?.icon) textIcon = parsed.icon;
      if (parsed?.icon_background) iconBackground = parsed.icon_background;
    } catch {
      /* ignore parse error */
    }
  }

  // Initialize controller on mount
  useEffect(() => {
    controller.init();
  }, [controller]);

  // Reactive controller state (use individual selectors to avoid new object each render)
  const activeId = useStore(controller.store, s => s.activeId);
  const activeConversationSummary = useStore(controller.store, s =>
    s.conversations.find(item => item.id === s.activeId)
  );
  const isLoadingDetail = useStore(controller.store, s => s.isLoadingDetail);

  useEffect(() => {
    if (!mobileDrawerOpen) return;
    setMobileDrawerOpen(false);
  }, [activeId, mobileDrawerOpen]);

  // Get active conversation from chat store for message rendering
  const conv = useChatStore(state => (activeId ? state.conversations[activeId] : undefined));

  // Fallback conversation to avoid empty UI
  const fallbackConv = useMemo<Conversation>(
    () => ({
      id: activeId ?? 'placeholder',
      conversationId: '',
      title: t('agents.workflow.chat.newConversation'),
      messages: [],
      conversationData: {},
    }),
    [activeId, t]
  );

  const currentConversation = conv ?? fallbackConv;
  const latestMessage = currentConversation.messages[currentConversation.messages.length - 1];
  const latestRunStatus = latestMessage?.WorkflowRunInfo?.status as string | undefined;
  const hasWorkflowRun = Boolean(latestMessage?.WorkflowRunInfo);
  const isCurrentWorkflowRunning =
    latestRunStatus === 'running' ||
    (hasWorkflowRun && latestMessage?.clientState?.phase === 'streaming');
  const isCurrentWorkflowPendingApproval = latestRunStatus === 'pending_approval';
  const isCurrentWorkflowPendingQuestion = latestRunStatus === 'pending_question';
  const isSendBlocked = Boolean(
    sendDisabled ||
      isCurrentWorkflowRunning ||
      isCurrentWorkflowPendingApproval ||
      (isCurrentWorkflowPendingQuestion && !allowPendingQuestionInput)
  );
  const isConversationActionBlocked = Boolean(
    sendDisabled ||
      isCurrentWorkflowRunning ||
      isCurrentWorkflowPendingApproval ||
      isCurrentWorkflowPendingQuestion
  );
  const effectiveIsRunning = Boolean(onStop && isCurrentWorkflowRunning);
  const isNewConversation = activeConversationSummary
    ? !activeConversationSummary.conversationId || activeConversationSummary.id.startsWith('draft-')
    : !currentConversation.conversationId || currentConversation.id.startsWith('draft-');

  const handleCreateNewConversation = () => {
    if (isConversationActionBlocked) return;
    const draft = controller.createDraft(t('agents.workflow.chat.newConversation'));
    controller.select(draft.id);
  };

  const handleSend = (payload: {
    query: string;
    files?: ChatAttachment[];
    inputs?: Record<string, unknown>;
  }) => {
    if (isSendBlocked) return;
    // Merge extra inputs with payload inputs
    const mergedInputs = extraInputs
      ? { ...extraInputs, ...(payload.inputs ?? {}) }
      : payload.inputs;
    controller.send({
      query: payload.query,
      files: payload.files,
      inputs: mergedInputs,
      historyWindowSize: typeof historyWindowSize === 'number' ? historyWindowSize : undefined,
    });
  };

  const handleSuggestionClick = useCallback(
    (text: string) => {
      if (inputDisabled || isSendBlocked) return;
      setDraftSuggestion(prev => ({
        id: (prev?.id ?? 0) + 1,
        text,
      }));
    },
    [inputDisabled, isSendBlocked]
  );

  return (
    <div className={cn('min-h-0 overflow-hidden', className)}>
      <div className="flex h-full min-h-0 overflow-hidden">
        {/* Desktop: persistent sidebar */}
        <div className="hidden md:block">
          <ConversationHistoryList
            controller={controller}
            backgroundImage={isWebappSurface ? WEBAPP_CHAT_SIDEBAR_BG_IMAGE : undefined}
          />
        </div>

        {/* Main: messages and input */}
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
          {/* Mobile: toolbar with drawer trigger */}
          <div
            className={cn(
              'md:hidden flex items-center justify-between',
              isWebappSurface ? 'border-b bg-background/95 px-3 py-2' : 'py-1 px-2'
            )}
          >
            <div className="flex min-w-0 items-center gap-2">
              <Button
                variant="ghost"
                isIcon
                size="sm"
                onClick={() => setMobileDrawerOpen(true)}
                aria-label={t('webapp.chat.openConversations')}
              >
                <Menu className="h-4 w-4" />
              </Button>
              {isWebappSurface ? (
                <>
                  <IconPreview
                    iconType={iconType === 'image' ? 'image' : 'text'}
                    src={iconType === 'image' ? imgSrc : ''}
                    icon={textIcon}
                    iconBackground={iconBackground}
                    editable={false}
                    size="xs"
                  />
                  <div className="min-w-0 truncate text-sm font-semibold" title={webappMeta?.title}>
                    {webappMeta?.title}
                  </div>
                </>
              ) : null}
            </div>
            {!isNewConversation ? (
              <Button
                variant="ghost"
                size="xs"
                disabled={isConversationActionBlocked}
                onClick={handleCreateNewConversation}
                className="h-8 w-8 p-0"
                aria-label={t('webapp.chat.newConversation')}
              >
                <Plus className="h-4 w-4" />
              </Button>
            ) : null}
          </div>

          <div className="mx-auto flex-1 min-h-0 w-full max-w-6xl">
            <ConversationBox
              conversation={conv ?? fallbackConv}
              renderHeader={renderHeader}
              className="h-full"
              showWorkflowNodeDetail={showWorkflowNodeDetail}
              showWorkflowRunHeader={showWorkflowRunHeader}
              showWorkflowDetail={showWorkflowDetail}
              hideCompletedWorkflowDetail={hideCompletedWorkflowDetail}
              allowWorkflowDetailExpand={allowWorkflowDetailExpand}
              defaultWorkflowDetailOpen={defaultWorkflowDetailOpen}
              isLoading={isLoadingDetail}
              onSuggestionClick={handleSuggestionClick}
              openingGuide={openingGuide}
              openingGuideBrand={
                webappMeta
                  ? {
                      title: webappMeta.title,
                      iconType: iconType === 'image' ? 'image' : 'text',
                      icon: textIcon,
                      iconBackground,
                      iconSrc: imgSrc,
                    }
                  : openingGuideBrand
              }
              suggestions={suggestions}
              suggestionsTitle={suggestionsTitle}
              renderMessageAddon={renderMessageAddon}
            />
          </div>
          <div className="p-2 w-full max-w-6xl mx-auto">
            {inputReplacement ? (
              inputReplacement
            ) : (
              <UserInput
                onSend={handleSend}
                onStop={onStop}
                enableUpload={features?.file_upload?.enabled ?? enableUpload}
                uploadFeature={features?.file_upload}
                disabled={inputDisabled}
                disabledOverlay={inputDisabledOverlay}
                sendDisabled={isSendBlocked}
                isRunning={effectiveIsRunning}
                isStopping={isStopping}
                placeholder={placeholder}
                toolbarForm={toolbarForm}
                variant={isWebappSurface ? 'webapp' : 'default'}
                topNotice={inputTopNotice}
                uploadAccessMode={uploadAccessMode}
                allowWorkspaceSwitch={allowWorkspaceSwitch}
                draftValue={draftSuggestion}
              />
            )}
          </div>
        </div>
      </div>

      {/* Mobile: conversation history drawer */}
      <Sheet open={mobileDrawerOpen} onOpenChange={setMobileDrawerOpen}>
        <SheetContent
          aria-description="conversation history"
          side="left"
          className="flex h-screen w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-sm"
        >
          <div className="h-full overflow-hidden flex flex-col">
            <div className="shrink-0 border-b px-4 py-2">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <IconPreview
                    iconType={iconType === 'image' ? 'image' : 'text'}
                    src={iconType === 'image' ? imgSrc : ''}
                    icon={textIcon}
                    iconBackground={iconBackground}
                    editable={false}
                    size="xs"
                  />
                  <SheetTitle className="text-sm font-medium" title={webappMeta?.title}>
                    {webappMeta?.title}
                  </SheetTitle>
                </div>
              </div>
            </div>
            <div className="h-0 grow min-h-0 overflow-hidden">
              <ConversationHistoryList
                controller={controller}
                className="border-0 h-full w-full"
                backgroundImage={isWebappSurface ? WEBAPP_CHAT_SIDEBAR_BG_IMAGE : undefined}
                onCreateWhileDraft={() => setMobileDrawerOpen(false)}
              />
            </div>
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
};

export default ChatWithController;
