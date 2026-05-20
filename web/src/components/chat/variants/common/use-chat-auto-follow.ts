import * as React from 'react';
import type { Message } from '@/components/chat/types';

interface UseChatAutoFollowParams {
  messages: Message[];
  activeId: string | null;
}

export function useChatAutoFollow({ messages, activeId }: UseChatAutoFollowParams) {
  const viewportRef = React.useRef<HTMLDivElement>(null!);
  const bottomRef = React.useRef<HTMLDivElement>(null!);
  const autoFollowRef = React.useRef(true);

  const scrollToBottom = React.useCallback((behavior: ScrollBehavior = 'auto') => {
    if (bottomRef.current && autoFollowRef.current) {
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          bottomRef.current?.scrollIntoView({ block: 'end', behavior });
        });
      });
    }
  }, []);

  const handleScroll = React.useCallback((e: React.UIEvent<HTMLDivElement>) => {
    const el = e.currentTarget;
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    autoFollowRef.current = distanceFromBottom <= 50;
  }, []);

  const lastMessage = messages[messages.length - 1];

  React.useEffect(() => {
    scrollToBottom();
  }, [
    messages.length,
    lastMessage?.answer,
    lastMessage?.WorkflowRunInfo?.status,
    lastMessage?.WorkflowRunInfo?.runNodeInfo?.length,
    scrollToBottom,
  ]);

  React.useEffect(() => {
    autoFollowRef.current = true;
    scrollToBottom();
  }, [activeId, scrollToBottom]);

  return {
    viewportRef,
    bottomRef,
    handleScroll,
  };
}
