'use client';

import { useCallback, useEffect, useRef, type UIEvent } from 'react';
import type { AIChatPagination } from '@/components/chat/controllers/aichat-controller';
import type { AIChatMessage } from '@/services/types/aichat';

interface UseAIChatScrollParams {
  messages: AIChatMessage[];
  activeMessagePagination: AIChatPagination;
  isLoadingMessages: boolean;
  isLoadingOlderMessages: boolean;
  isSending: boolean;
  loadOlderMessages: () => Promise<void>;
}

/**
 * @hook useAIChatScroll
 * @description Handles AIChat auto-scroll and upward pagination anchor restoration.
 * @usage Use inside AIChatShell to wire ScrollArea viewport refs and scroll events
 */
export function useAIChatScroll({
  messages,
  activeMessagePagination,
  isLoadingMessages,
  isLoadingOlderMessages,
  isSending,
  loadOlderMessages,
}: UseAIChatScrollParams) {
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const scrollViewportRef = useRef<HTMLDivElement | null>(null);
  const loadingOlderRequestRef = useRef(false);
  const skipNextAutoScrollRef = useRef(false);

  useEffect(() => {
    if (skipNextAutoScrollRef.current) {
      skipNextAutoScrollRef.current = false;
      return;
    }

    requestAnimationFrame(() => {
      const viewport = scrollViewportRef.current;
      if (!viewport) return;
      viewport.scrollTop = viewport.scrollHeight;
    });
  }, [messages, isSending]);

  const handleMessagesScroll = useCallback(
    (event: UIEvent<HTMLDivElement>) => {
      const viewport = event.currentTarget;
      if (
        viewport.scrollTop > 80 ||
        !activeMessagePagination.hasMore ||
        isLoadingMessages ||
        isLoadingOlderMessages ||
        loadingOlderRequestRef.current
      ) {
        return;
      }

      const previousScrollHeight = viewport.scrollHeight;
      const previousScrollTop = viewport.scrollTop;
      loadingOlderRequestRef.current = true;
      skipNextAutoScrollRef.current = true;

      void loadOlderMessages()
        .then(() => {
          requestAnimationFrame(() => {
            const nextViewport = scrollViewportRef.current;
            if (!nextViewport) return;
            nextViewport.scrollTop =
              nextViewport.scrollHeight - previousScrollHeight + previousScrollTop;
          });
        })
        .finally(() => {
          loadingOlderRequestRef.current = false;
        });
    },
    [activeMessagePagination.hasMore, isLoadingMessages, isLoadingOlderMessages, loadOlderMessages]
  );

  return {
    bottomRef,
    scrollViewportRef,
    handleMessagesScroll,
  };
}
