import * as React from 'react';
import MessageItem from '@/components/chat/ui/message-item';
import type { Message } from '@/components/chat/types';
import { cn } from '@/lib/utils';

interface ChatMessageViewportProps {
  isLoadingDetail: boolean;
  isHome: boolean;
  messages: Message[];
  viewportRef: React.RefObject<HTMLDivElement>;
  bottomRef: React.RefObject<HTMLDivElement>;
  onScroll: (e: React.UIEvent<HTMLDivElement>) => void;
  loadingFallback: React.ReactNode;
  loadingClassName?: string;
  containerClassName?: string;
  scrollAreaClassName?: string;
  listClassName?: string;
  itemClassName?: string;
  showCopyButton?: boolean;
}

export function ChatMessageViewport({
  isLoadingDetail,
  isHome,
  messages,
  viewportRef,
  bottomRef,
  onScroll,
  loadingFallback,
  loadingClassName,
  containerClassName,
  scrollAreaClassName,
  listClassName,
  itemClassName,
  showCopyButton,
}: ChatMessageViewportProps) {
  if (isLoadingDetail) {
    return (
      <div className={cn('flex-1 flex flex-col relative z-20', loadingClassName)}>
        {loadingFallback}
      </div>
    );
  }

  return (
    <div
      className={cn(
        'flex-1 flex flex-col relative min-w-0 transition-opacity duration-300 pt-14 sm:pt-16 pb-36 sm:pb-44',
        isHome ? 'opacity-0 pointer-events-none' : 'opacity-100',
        containerClassName
      )}
    >
      <div className="w-full grow h-0 min-h-0">
        <div
          ref={viewportRef}
          className={cn('h-full overflow-y-auto p-3', scrollAreaClassName)}
          onScroll={onScroll}
        >
          <div className={cn('space-y-6 max-w-4xl mx-auto w-full min-w-0', listClassName)}>
            {messages.map((msg, idx) => (
              <div
                key={msg.messageId || `msg-${idx}`}
                className={cn('w-full min-w-0', itemClassName)}
              >
                <MessageItem
                  message={msg}
                  showWorkflowNodeDetail={false}
                  showWorkflowDetail={false}
                  showAvatar={false}
                  showCopyButton={showCopyButton}
                />
              </div>
            ))}
            <div ref={bottomRef} className="h-px w-full" />
          </div>
        </div>
      </div>
    </div>
  );
}
