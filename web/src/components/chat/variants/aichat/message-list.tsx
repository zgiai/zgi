'use client';

import { ModelIcon } from 'modelicons';
import { Bot, Loader2 } from 'lucide-react';
import type { Ref, UIEvent } from 'react';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { ChatBranchNavigation } from '@/components/chat/utils/message-tree';
import type {
  AIChatConversation,
  ExecutionFileReader,
  AIChatMessage,
  AIChatMessageFile,
  AIChatTokenUsageUpdatedEventData,
} from '@/services/types/aichat';
import type { AIChatStreamingMessageState } from '@/components/chat/controllers/aichat';
import { AIChatMessageBubble } from '@/components/chat/variants/aichat/message-bubble';
import type { AIChatSkillDisplayMap } from '@/components/chat/variants/aichat/skill-display';
import type {
  AIChatToolGovernanceDecisionSubmitPayload,
  AITaskProposalActionPayload,
} from '@/components/chat/variants/aichat/agentic-timeline';
import { presentationProjectionFromMetadata } from '@/components/chat/controllers/aichat/presentation-order';
import type { AIChatSpeechPlaybackController } from '@/components/chat/variants/aichat/voice/use-agent-speech-playback';
import type {
  ConnectorRuntimeConfirmationCommand,
  ConnectorRuntimeDecisionResult,
} from '@/services/types/connector';

interface AIChatMessageListProps {
  readExecutionFile?: ExecutionFileReader;
  messages: AIChatMessage[];
  activeConversation: AIChatConversation | null;
  activeMessageCount: number;
  branchNavigationByMessageId: Map<string, ChatBranchNavigation>;
  isLoadingMessages: boolean;
  isLoadingOlderMessages: boolean;
  isSending: boolean;
  streamingByMessageId: Record<string, AIChatStreamingMessageState>;
  skillDisplayById: AIChatSkillDisplayMap;
  editingMessageId: string | null;
  editingQuery: string;
  bottomRef: Ref<HTMLDivElement>;
  scrollViewportRef: Ref<HTMLDivElement>;
  bottomSpacerHeight: number;
  onScroll: (event: UIEvent<HTMLDivElement>) => void;
  onRegenerate?: (message: AIChatMessage) => void;
  onToolGovernanceDecision?: (
    payload: AIChatToolGovernanceDecisionSubmitPayload
  ) => void | Promise<void>;
  onTaskProposalAction?: (payload: AITaskProposalActionPayload) => void | Promise<void>;
  onTaskMutation?: () => void;
  onConnectorRuntimeDecision?: (
    conversationId: string,
    messageId: string,
    command: ConnectorRuntimeConfirmationCommand,
    scope?: string,
    rememberForSession?: boolean
  ) => Promise<ConnectorRuntimeDecisionResult>;
  onSwitchBranch: (messageId: string) => void;
  onEditStart?: (message: AIChatMessage) => void;
  onEditChange: (value: string) => void;
  onEditCancel: () => void;
  onEditSubmit: (message: AIChatMessage) => void;
  showAssistantModelMeta?: boolean;
  layout?: 'full' | 'embedded';
  messageDensity?: 'default' | 'compact';
  showMemoryKey?: boolean;
  showSkillEventDetails?: boolean;
  showWorkflowFailureDetails?: boolean;
  showContextualOperationStatus?: boolean;
  enableToolGovernanceApprovals?: boolean;
  suppressPendingToolGovernanceApprovals?: boolean;
  showPlanningPlaceholder?: boolean;
  pendingUserMessage?: AIChatPendingUserMessage | null;
  speechPlayback?: AIChatSpeechPlaybackController;
}

export interface AIChatPendingUserMessage {
  id: string;
  query: string;
  files?: AIChatMessageFile[];
  assistantModelName?: string;
  assistantCreatedAt?: number;
  showAssistantPlanning?: boolean;
}

function formatAIChatPendingTime(timestamp: number): string {
  if (!timestamp) return '';

  const date = new Date(timestamp * 1000);
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function isReplaceableRootStatus(status: AIChatMessage['status']): boolean {
  return status === 'completed' || status === 'stopped' || status === 'error';
}

function canReplaceRootMessage(
  conversation: AIChatConversation | null,
  activeMessageCount: number,
  message: AIChatMessage
): boolean {
  if (!conversation) return false;

  return (
    conversation.runtime_status === 'idle' &&
    conversation.dialogue_count === 1 &&
    !message.parent_id &&
    conversation.current_leaf_message_id === message.id &&
    isReplaceableRootStatus(message.status) &&
    activeMessageCount === 1
  );
}

function tokenUsageFromMessage(
  message: AIChatMessage
): AIChatTokenUsageUpdatedEventData | undefined {
  const budget = message.metadata?.task_token_budget;
  const usage = message.metadata?.usage;
  if (!budget && !usage?.total_tokens) return undefined;
  const total = Math.max(0, usage?.total_tokens ?? 0);
  return {
    conversation_id: message.conversation_id,
    message_id: message.id,
    usage: usage ?? { total_tokens: total },
    ...(budget ? { task_token_budget: budget } : {}),
  };
}

/**
 * @component AIChatMessageList
 * @category Feature
 * @status Stable
 * @description Scrollable AIChat message list with loading skeletons and upward pagination row.
 * @usage Render inside AIChatShell main area
 * @example
 * <AIChatMessageList messages={messages} />
 */
export function AIChatMessageList({
  readExecutionFile,
  messages,
  activeConversation,
  activeMessageCount,
  branchNavigationByMessageId,
  isLoadingMessages,
  isLoadingOlderMessages,
  isSending,
  streamingByMessageId,
  skillDisplayById,
  editingMessageId,
  editingQuery,
  bottomRef,
  scrollViewportRef,
  bottomSpacerHeight,
  onScroll,
  onRegenerate,
  onToolGovernanceDecision,
  onTaskProposalAction,
  onTaskMutation,
  onConnectorRuntimeDecision,
  onSwitchBranch,
  onEditStart,
  onEditChange,
  onEditCancel,
  onEditSubmit,
  showAssistantModelMeta = true,
  layout = 'full',
  messageDensity = 'default',
  showMemoryKey = true,
  showSkillEventDetails = true,
  showWorkflowFailureDetails = true,
  showContextualOperationStatus = false,
  enableToolGovernanceApprovals = false,
  suppressPendingToolGovernanceApprovals = false,
  showPlanningPlaceholder = false,
  pendingUserMessage = null,
  speechPlayback,
}: AIChatMessageListProps) {
  const t = useT('webapp');

  return (
    <ScrollArea
      className="min-h-0 flex-1"
      viewportRef={scrollViewportRef}
      viewportProps={{
        onScroll,
        className: '[&>div]:!block [&>div]:!w-full [&>div]:!min-w-0',
      }}
    >
      <div
        className={cn(
          'mx-auto flex min-h-full w-full min-w-0 flex-col pb-4',
          layout === 'embedded'
            ? 'max-w-full px-4 pt-5'
            : messageDensity === 'compact'
              ? 'max-w-6xl px-4 pt-8 sm:px-6 lg:px-8'
              : 'max-w-5xl px-4 pt-20 sm:px-6 lg:px-8'
        )}
      >
        {isLoadingMessages ? (
          <div className="space-y-6">
            {Array.from({ length: 3 }).map((_, index) => (
              <div key={index} className="space-y-3">
                <Skeleton className="ml-auto h-10 w-2/5 rounded-2xl" />
                <Skeleton className="h-20 w-4/5 rounded-md" />
              </div>
            ))}
          </div>
        ) : (
          <div className="min-w-0 space-y-8">
            {isLoadingOlderMessages ? (
              <div className="flex h-8 items-center justify-center text-muted-foreground">
                <Loader2 className="size-4 animate-spin" />
              </div>
            ) : null}
            {messages.map((message, index) => (
              // The message id anchor lets callers bring a message into view.
              <div key={message.id} data-chat-message-id={message.id}>
                <AIChatMessageBubble
                  readExecutionFile={readExecutionFile}
                  message={message}
                  isSending={isSending}
                  timeline={streamingByMessageId[message.id]?.timeline ?? []}
                  modelProcessing={streamingByMessageId[message.id]?.modelProcessing}
                  tokenUsage={
                    streamingByMessageId[message.id]?.tokenUsage ?? tokenUsageFromMessage(message)
                  }
                  presentationItems={
                    streamingByMessageId[message.id]?.presentationItems ??
                    presentationProjectionFromMetadata(message.metadata)?.items
                  }
                  answerBeforeTimelineLength={
                    streamingByMessageId[message.id]?.answer_before_timeline_length ??
                    message.metadata?.answer_before_timeline_length
                  }
                  skillDisplayById={skillDisplayById}
                  isLastMessage={index === messages.length - 1}
                  canReplaceRoot={canReplaceRootMessage(
                    activeConversation,
                    activeMessageCount,
                    message
                  )}
                  onRegenerate={onRegenerate}
                  onToolGovernanceDecision={onToolGovernanceDecision}
                  onTaskProposalAction={onTaskProposalAction}
                  onTaskMutation={onTaskMutation}
                  onConnectorRuntimeDecision={
                    onConnectorRuntimeDecision
                      ? (command, scope, rememberForSession) =>
                          onConnectorRuntimeDecision(
                            message.conversation_id,
                            message.id,
                            command,
                            scope,
                            rememberForSession
                          )
                      : undefined
                  }
                  branchNavigation={branchNavigationByMessageId.get(message.id)}
                  onSwitchBranch={onSwitchBranch}
                  isEditing={editingMessageId === message.id}
                  editValue={editingMessageId === message.id ? editingQuery : ''}
                  onEditStart={onEditStart}
                  onEditChange={onEditChange}
                  onEditCancel={onEditCancel}
                  onEditSubmit={onEditSubmit}
                  showAssistantModelMeta={showAssistantModelMeta}
                  showMemoryKey={showMemoryKey}
                  showSkillEventDetails={showSkillEventDetails}
                  showWorkflowFailureDetails={showWorkflowFailureDetails}
                  showContextualOperationStatus={showContextualOperationStatus}
                  enableToolGovernanceApprovals={enableToolGovernanceApprovals}
                  suppressPendingToolGovernanceApprovals={suppressPendingToolGovernanceApprovals}
                  speechPlayback={speechPlayback}
                />
              </div>
            ))}
            {pendingUserMessage ? <PendingUserMessageBubble message={pendingUserMessage} /> : null}
            {showPlanningPlaceholder ? (
              <PendingAssistantPlanningStatus
                label={t('consoleChat.operationStatus.planning')}
                modelName={pendingUserMessage?.assistantModelName}
                createdAt={pendingUserMessage?.assistantCreatedAt}
                showAssistantModelMeta={showAssistantModelMeta}
                streamingLabel={t('consoleChat.streaming')}
              />
            ) : null}
            <div ref={bottomRef} className="shrink-0" style={{ height: bottomSpacerHeight }} />
          </div>
        )}
      </div>
    </ScrollArea>
  );
}

function PendingAssistantPlanningStatus({
  label,
  modelName,
  createdAt,
  showAssistantModelMeta,
  streamingLabel,
}: {
  label: string;
  modelName?: string;
  createdAt?: number;
  showAssistantModelMeta: boolean;
  streamingLabel: string;
}) {
  return (
    <div className="flex min-w-0 justify-start gap-3">
      <div
        className={cn(
          'mt-1 flex size-7 shrink-0 items-center justify-center rounded-full',
          showAssistantModelMeta ? 'border bg-background' : 'bg-primary text-primary-foreground'
        )}
      >
        {showAssistantModelMeta ? (
          <ModelIcon model={modelName || 'unknown'} size={28} />
        ) : (
          <Bot className="size-4" />
        )}
      </div>
      <div className="min-w-0 max-w-full flex-1 overflow-hidden">
        <div className="mb-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {showAssistantModelMeta && modelName ? <span>{modelName}</span> : null}
          {createdAt ? <span>{formatAIChatPendingTime(createdAt)}</span> : null}
          <span className="inline-flex items-center gap-1">
            <Loader2 className="size-3 animate-spin" />
            {streamingLabel}
          </span>
        </div>
        <div className="border-l-2 border-muted-foreground/20 pl-3 text-sm leading-7 text-muted-foreground">
          <span className="inline-flex min-w-0 items-center gap-2">
            <Loader2 className="size-3.5 shrink-0 animate-spin" />
            <span className="min-w-0">{label}</span>
          </span>
        </div>
      </div>
    </div>
  );
}

function PendingUserMessageBubble({ message }: { message: AIChatPendingUserMessage }) {
  const files = message.files ?? [];
  const documentFiles = files.filter(file => file.kind !== 'image');
  const imageFiles = files.filter(file => file.kind === 'image');

  return (
    <div className="flex justify-end">
      <div className="max-w-[82%]">
        <div className="rounded-2xl bg-muted px-3 py-2 text-sm text-foreground opacity-90 shadow-sm">
          <div className="whitespace-pre-wrap break-words">{message.query}</div>
          {files.length > 0 ? (
            <div className="mt-2 flex flex-wrap gap-1.5">
              {[...imageFiles, ...documentFiles].map(file => (
                <span
                  key={file.id}
                  className="inline-flex max-w-44 items-center gap-1 rounded-md border border-border bg-background/70 px-2 py-1 text-xs text-muted-foreground"
                  title={file.name}
                >
                  <span className="truncate">{file.name}</span>
                </span>
              ))}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
