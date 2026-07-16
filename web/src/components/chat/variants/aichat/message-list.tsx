'use client';

import { ModelIcon } from 'modelicons';
import { Bot, Loader2 } from 'lucide-react';
import type { Ref, UIEvent } from 'react';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { ChatBranchNavigation } from '@/components/chat/utils/message-tree';
import type { AIChatConversation, AIChatMessage, AIChatMessageFile } from '@/services/types/aichat';
import type { AIChatStreamingMessageState } from '@/components/chat/controllers/aichat';
import { AIChatMessageBubble } from '@/components/chat/variants/aichat/message-bubble';
import type { AIChatSkillDisplayMap } from '@/components/chat/variants/aichat/skill-display';
import type { AIChatToolGovernanceDecisionSubmitPayload } from '@/components/chat/variants/aichat/agentic-timeline';

interface AIChatMessageListProps {
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
  onRegenerate: (message: AIChatMessage) => void;
  onToolGovernanceDecision?: (
    payload: AIChatToolGovernanceDecisionSubmitPayload
  ) => void | Promise<void>;
  onSwitchBranch: (messageId: string) => void;
  onEditStart: (message: AIChatMessage) => void;
  onEditChange: (value: string) => void;
  onEditCancel: () => void;
  onEditSubmit: (message: AIChatMessage) => void;
  showAssistantModelMeta?: boolean;
  layout?: 'full' | 'embedded';
  showMemoryKey?: boolean;
  showSkillEventDetails?: boolean;
  enableToolGovernanceApprovals?: boolean;
  suppressPendingToolGovernanceApprovals?: boolean;
  showPlanningPlaceholder?: boolean;
  pendingUserMessage?: AIChatPendingUserMessage | null;
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
  onSwitchBranch,
  onEditStart,
  onEditChange,
  onEditCancel,
  onEditSubmit,
  showAssistantModelMeta = true,
  layout = 'full',
  showMemoryKey = true,
  showSkillEventDetails = true,
  enableToolGovernanceApprovals = false,
  suppressPendingToolGovernanceApprovals = false,
  showPlanningPlaceholder = false,
  pendingUserMessage = null,
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
          layout === 'embedded' ? 'max-w-full px-4 pt-5' : 'max-w-5xl px-4 pt-20 sm:px-6 lg:px-8'
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
              <AIChatMessageBubble
                key={message.id}
                message={message}
                isSending={isSending}
                timeline={streamingByMessageId[message.id]?.timeline ?? []}
                skillDisplayById={skillDisplayById}
                isLastMessage={index === messages.length - 1}
                canReplaceRoot={canReplaceRootMessage(
                  activeConversation,
                  activeMessageCount,
                  message
                )}
                onRegenerate={onRegenerate}
                onToolGovernanceDecision={onToolGovernanceDecision}
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
                enableToolGovernanceApprovals={enableToolGovernanceApprovals}
                suppressPendingToolGovernanceApprovals={suppressPendingToolGovernanceApprovals}
              />
            ))}
            {pendingUserMessage ? (
              <PendingUserMessageBubble message={pendingUserMessage} />
            ) : null}
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
