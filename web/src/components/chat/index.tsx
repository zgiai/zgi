import React, { useEffect, useMemo, useCallback, useState } from 'react';
import type { Conversation } from '@/components/chat/types';
import { useChatStore } from '@/components/chat/store';
import ConversationBox from '@/components/chat/ui/conversation-box';
import UserInput from '@/components/chat/ui/user-input';
import type { ChatAttachment, Message } from '@/components/chat/types';
import type { WorkflowFeatures } from '@/components/workflow/store/type';
import { useT } from '@/i18n';
import type { InputVar } from '@/components/workflow/types/input-var';
import ChatWithController from '@/components/chat/chat-with-controller';
import type { ChatController } from '@/components/chat/controllers/types';
import type { OpeningGuideConfig } from '@/utils/webapp/opening-statement';

import { SysChat, type SysChatProps } from './variants/sys/sys-chat';
import { ImgChat } from './variants/img/img-chat';
import {
  type ModelSelectorModelProps,
  type ModelSelectorValue,
} from '../common/model-selector/model-selector';
import { AIChatShell, type AIChatModelValue } from '@/components/chat/variants/aichat/aichat-chat';
import type { AIChatUploadScope } from '@/components/chat/variants/aichat/input-area';
import type { AIChatController } from '@/components/chat/controllers/aichat-controller';
import type { OpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';
import type { AIChatRuntimeSurface } from '@/services/types/aichat';
import type { ImageRuntimeModel } from '@/services/types/image-runtime';
import type { ModelUseCase } from '@/services/types/model';

interface SingleTestVariantProps {
  mode: 'singleTest';
  conversation: { id: string; conversationId?: string; title?: string };
  onSend: (
    conversations: Array<{ id: string; conversationId: string | null }>,
    userInput: {
      query: string;
      files?: ChatAttachment[];
      inputs: Record<string, unknown>;
      history_window_size?: number;
    }
  ) => void;
  /** Callback when stop button is clicked */
  onStop?: () => void;
  renderHeader?: (c: Conversation) => React.ReactNode;
  className?: string;
  enableUpload?: boolean;
  features?: Pick<WorkflowFeatures, 'file_upload' | 'retriever_resource'>;
  inputDisabled?: boolean;
  /** Custom overlay content to show when input is disabled */
  inputDisabledOverlay?: React.ReactNode;
  sendDisabled?: boolean;
  /** Whether workflow is currently running - shows stop button instead of send */
  isRunning?: boolean;
  /** Whether stop action is in progress */
  isStopping?: boolean;
  placeholder?: string;
  inputClassName?: string;
  openingGuide?: OpeningGuideConfig;
  openingGuideBrand?: OpeningGuideBrand;
  suggestions?: string[];
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
  historyWindowSize?: number;
  enableInit?: boolean;
  inputTopNotice?: React.ReactNode;
  inputReplacement?: React.ReactNode;
  renderMessageAddon?: (message: Message) => React.ReactNode;
}

interface SingleChatVariantProps {
  mode: 'singleChat';
  controller: ChatController;
  renderHeader?: (c: Conversation) => React.ReactNode;
  className?: string;
  enableUpload?: boolean;
  features?: Pick<WorkflowFeatures, 'file_upload' | 'retriever_resource'>;
  inputDisabled?: boolean;
  placeholder?: string;
  openingGuide?: OpeningGuideConfig;
  openingGuideBrand?: OpeningGuideBrand;
  suggestions?: string[];
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
  historyWindowSize?: number;
  inputTopNotice?: React.ReactNode;
  inputReplacement?: React.ReactNode;
  conversationSearchKey?: readonly unknown[];
}

interface SysChatVariantProps extends SysChatProps {
  mode: 'sys';
}

interface ImgChatVariantProps {
  mode: 'img';
  controller: ChatController;
  modelSelectorValue?: ModelSelectorValue;
  onModelChange?: (value: ModelSelectorValue) => void;
  inputTopNotice?: React.ReactNode;
  conversationSearchKey?: readonly unknown[];
  imageRuntimeModels?: ImageRuntimeModel[];
}

interface AIChatVariantProps {
  mode: 'aichat';
  controller: AIChatController;
  modelSelectorValue: AIChatModelValue;
  modelProps?: ModelSelectorModelProps | null;
  supportsVisionOverride?: boolean;
  isModelInitializing?: boolean;
  onModelChange: (value: ModelSelectorValue) => void;
  beforeSend?: () => boolean | Promise<boolean>;
  variant?: 'full' | 'embedded';
  showModelSelector?: boolean;
  modelUseCase?: ModelUseCase;
  preferredModelUseCase?: ModelUseCase;
  requireModel?: boolean;
  showMemoryToggle?: boolean;
  forcedUseMemory?: boolean;
  enableUpload?: boolean;
  uploadScope?: AIChatUploadScope;
  showFileLibraryPicker?: boolean;
  allowWorkspaceSwitch?: boolean;
  homeBrand?: React.ReactNode;
  openingGuideBrand?: OpeningGuideBrand;
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
  onSelectConversation?: (id: string) => void;
  onStartNewConversation?: () => void;
  showAssistantModelMeta?: boolean;
  surface?: 'aichat' | 'agent-draft' | 'agent-webapp';
  runtimeSurface?: AIChatRuntimeSurface;
  themeColor?: string;
  enableToolGovernance?: boolean;
}

type ChatProps =
  | SingleTestVariantProps
  | SingleChatVariantProps
  | SysChatVariantProps
  | ImgChatVariantProps
  | AIChatVariantProps;

const SingleTestChat: React.FC<SingleTestVariantProps> = ({
  mode,
  conversation,
  onSend,
  onStop,
  renderHeader,
  className,
  enableUpload = true,
  features,
  inputDisabled,
  inputDisabledOverlay,
  sendDisabled,
  isRunning,
  isStopping,
  placeholder,
  inputClassName,
  openingGuide,
  openingGuideBrand,
  suggestions,
  toolbarForm,
  showWorkflowNodeDetail,
  showWorkflowRunHeader,
  showWorkflowDetail,
  hideCompletedWorkflowDetail,
  allowWorkflowDetailExpand,
  defaultWorkflowDetailOpen,
  historyWindowSize,
  enableInit = false,
  inputTopNotice,
  inputReplacement,
  renderMessageAddon,
}) => {
  const t = useT();
  const initSingle = useChatStore.use.initSingle();
  // Select live conversation by id to refresh UI on store updates
  const conv = useChatStore(state => state.conversations[conversation.id]);
  const [draftSuggestion, setDraftSuggestion] = useState<{ id: number; text: string } | null>(null);

  useEffect(() => {
    if (mode !== 'singleTest' || !enableInit) return;
    initSingle({
      id: conversation.id,
      conversationId: conversation.conversationId ?? '',
      title: conversation.title ?? t('agents.workflow.chat.newConversation'),
    });
  }, [
    mode,
    initSingle,
    conversation.id,
    conversation.conversationId,
    conversation.title,
    t,
    enableInit,
  ]);

  // Fallback conversation to avoid empty UI on first paint before init
  const fallbackConv = useMemo<Conversation>(
    () => ({
      id: conversation.id,
      conversationId: conversation.conversationId ?? '',
      title: conversation.title ?? t('agents.workflow.chat.newConversation'),
      messages: [],
      conversationData: {},
    }),
    [conversation.id, conversation.conversationId, conversation.title, t]
  );

  const handleSend = useCallback(
    (payload: { query: string; files?: ChatAttachment[]; inputs?: Record<string, unknown> }) => {
      const current = conv;
      const backendId = (current?.conversationId ?? '').trim();
      onSend([{ id: conversation.id, conversationId: backendId.length > 0 ? backendId : null }], {
        query: payload.query,
        files: payload.files,
        inputs: payload.inputs ?? {},
        history_window_size: typeof historyWindowSize === 'number' ? historyWindowSize : undefined,
      });
    },
    [onSend, conv, conversation.id, historyWindowSize]
  );

  const handleSuggestionClick = useCallback(
    (text: string) => {
      if (inputDisabled || sendDisabled || isRunning) return;
      setDraftSuggestion(prev => ({
        id: (prev?.id ?? 0) + 1,
        text,
      }));
    },
    [inputDisabled, sendDisabled, isRunning]
  );

  // Responsive container focusing on singleTest width adaptability
  return (
    <div className={className}>
      <div className="flex flex-col h-full gap-3">
        <div className="flex-1 min-h-0">
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
            openingGuide={openingGuide}
            openingGuideBrand={openingGuideBrand}
            suggestions={suggestions}
            onSuggestionClick={handleSuggestionClick}
            renderMessageAddon={renderMessageAddon}
          />
        </div>
        <div className="p-3">
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
              sendDisabled={sendDisabled}
              isRunning={isRunning}
              isStopping={isStopping}
              placeholder={placeholder}
              className={inputClassName}
              toolbarForm={toolbarForm}
              topNotice={inputTopNotice}
              draftValue={draftSuggestion}
            />
          )}
        </div>
      </div>
    </div>
  );
};

const SingleChatWrapper: React.FC<SingleChatVariantProps> = ({
  controller,
  renderHeader,
  className,
  enableUpload = true,
  features,
  inputDisabled,
  placeholder,
  openingGuide,
  openingGuideBrand,
  suggestions,
  toolbarForm,
  showWorkflowNodeDetail,
  showWorkflowRunHeader,
  showWorkflowDetail,
  hideCompletedWorkflowDetail,
  allowWorkflowDetailExpand,
  defaultWorkflowDetailOpen,
  historyWindowSize,
  inputTopNotice,
  inputReplacement,
  conversationSearchKey,
}) => {
  return (
    <ChatWithController
      controller={controller}
      renderHeader={renderHeader}
      className={className}
      enableUpload={enableUpload}
      features={features}
      inputDisabled={inputDisabled}
      placeholder={placeholder}
      openingGuide={openingGuide}
      openingGuideBrand={openingGuideBrand}
      suggestions={suggestions}
      toolbarForm={toolbarForm}
      showWorkflowNodeDetail={showWorkflowNodeDetail}
      showWorkflowRunHeader={showWorkflowRunHeader}
      showWorkflowDetail={showWorkflowDetail}
      hideCompletedWorkflowDetail={hideCompletedWorkflowDetail}
      allowWorkflowDetailExpand={allowWorkflowDetailExpand}
      defaultWorkflowDetailOpen={defaultWorkflowDetailOpen}
      historyWindowSize={historyWindowSize}
      inputTopNotice={inputTopNotice}
      inputReplacement={inputReplacement}
      conversationSearchKey={conversationSearchKey}
    />
  );
};

const Chat: React.FC<ChatProps> = props => {
  if (props.mode === 'sys') {
    return <SysChat {...(props as SysChatVariantProps)} />;
  }
  if (props.mode === 'img') {
    return <ImgChat {...(props as ImgChatVariantProps)} />;
  }
  if (props.mode === 'aichat') {
    return <AIChatShell {...(props as AIChatVariantProps)} />;
  }
  if (props.mode === 'singleChat') {
    return <SingleChatWrapper {...(props as SingleChatVariantProps)} />;
  }
  return <SingleTestChat {...(props as SingleTestVariantProps)} />;
};

export default Chat;

// Public exports for consumers
export { useChatApi } from '@/components/chat/hooks/use-chat-api';
export { useChatStore } from '@/components/chat/store';
export type {
  Conversation,
  Message,
  NodeInfo,
  ChatRunCallbacks,
  ChatRunFinishedContext,
  ChatRunStartedContext,
} from '@/components/chat/types';
export type { UseChatApi } from '@/components/chat/hooks/use-chat-api';

// New controller-based exports
export { default as ChatWithController } from '@/components/chat/chat-with-controller';
export { SingleChatController } from '@/components/chat/controllers/single-chat-controller';
export { WebappConversationTransport } from '@/components/chat/transports/webapp-transport';
export { AgentAdvancedChatTransport } from '@/components/chat/transports/agent-advanced-chat-transport';
export { AIChatTransport } from '@/components/chat/transports/aichat-transport';
export {
  AgentRuntimeTransport,
  createAgentDraftTransport,
  createAgentWebAppTransport,
} from '@/components/chat/transports/agent-runtime-transport';
export {
  useAIChatController,
  useChatRuntimeController,
} from '@/components/chat/runtime/controller/use-chat-runtime-controller';
export { AIChatShell, AIChatMessageBubble } from '@/components/chat/variants/aichat/aichat-chat';
export {
  buildCurrentChatPath,
  buildChatMessageTopology,
  buildChatBranchNavigationByMessageId,
} from '@/components/chat/utils/message-tree';
export { buildCurrentAIChatPath } from '@/components/chat/utils/aichat';
export type {
  AIChatController,
  AIChatModelSelection,
} from '@/components/chat/controllers/aichat-controller';
export type { AIChatModelValue } from '@/components/chat/variants/aichat/aichat-chat';
export type {
  ChatController,
  ConversationTransport,
  ConversationSummary,
  ConversationDetail,
  ChatMode,
  SendMessagePayload,
  Pagination,
  ChatRunCallbacks as ControllerChatRunCallbacks,
} from '@/components/chat/controllers/types';
