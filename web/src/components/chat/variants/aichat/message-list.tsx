'use client';

import { Loader2 } from 'lucide-react';
import type { Ref, UIEvent } from 'react';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import type { ChatBranchNavigation } from '@/components/chat/utils/message-tree';
import type { AIChatConversation, AIChatMessage } from '@/services/types/aichat';
import type { AIChatStreamingMessageState } from '@/components/chat/controllers/aichat';
import { AIChatMessageBubble } from '@/components/chat/variants/aichat/message-bubble';
import type { AIChatSkillDisplayMap } from '@/components/chat/variants/aichat/skill-display';

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
  onSwitchBranch: (messageId: string) => void;
  onEditStart: (message: AIChatMessage) => void;
  onEditChange: (value: string) => void;
  onEditCancel: () => void;
  onEditSubmit: (message: AIChatMessage) => void;
  showAssistantModelMeta?: boolean;
  layout?: 'full' | 'embedded';
  showMemoryKey?: boolean;
  showSkillEventDetails?: boolean;
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
  onSwitchBranch,
  onEditStart,
  onEditChange,
  onEditCancel,
  onEditSubmit,
  showAssistantModelMeta = true,
  layout = 'full',
  showMemoryKey = true,
  showSkillEventDetails = true,
}: AIChatMessageListProps) {
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
          'mx-auto flex min-h-full w-full min-w-0 flex-col px-4 pb-4 pt-20 sm:px-6 lg:px-8',
          layout === 'embedded' ? 'max-w-full' : 'max-w-5xl'
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
          <div className="space-y-8">
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
              />
            ))}
            <div ref={bottomRef} className="shrink-0" style={{ height: bottomSpacerHeight }} />
          </div>
        )}
      </div>
    </ScrollArea>
  );
}
