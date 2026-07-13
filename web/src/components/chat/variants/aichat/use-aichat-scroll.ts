'use client';

import { useCallback, useEffect, useRef, useState, type UIEvent } from 'react';
import type { AIChatPagination } from '@/components/chat/controllers/aichat-controller';
import type { AIChatMessage } from '@/services/types/aichat';

interface UseAIChatScrollParams {
  messages: AIChatMessage[];
  activeMessagePagination: AIChatPagination;
  isLoadingMessages: boolean;
  isLoadingOlderMessages: boolean;
  isSending: boolean;
  bottomInsetHeight: number;
  loadOlderMessages: () => Promise<void>;
}

const detachDistanceFromBottom = 120;
const resumeDistanceFromBottom = 48;

function getDistanceFromBottom(viewport: HTMLDivElement): number {
  return Math.max(0, viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight);
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
  bottomInsetHeight,
  loadOlderMessages,
}: UseAIChatScrollParams) {
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const scrollViewportRef = useRef<HTMLDivElement | null>(null);
  const loadingOlderRequestRef = useRef(false);
  const skipNextAutoScrollRef = useRef(false);
  const autoFollowRef = useRef(true);
  const previousScrollTopRef = useRef(0);
  const wasSendingRef = useRef(false);
  const [isAutoFollowPaused, setIsAutoFollowPaused] = useState(false);

  const setAutoFollow = useCallback((next: boolean) => {
    autoFollowRef.current = next;
    setIsAutoFollowPaused(!next);
  }, []);

  const scrollToBottom = useCallback((behavior: ScrollBehavior = 'auto') => {
    requestAnimationFrame(() => {
      const viewport = scrollViewportRef.current;
      if (!viewport) return;
      viewport.scrollTo({ top: viewport.scrollHeight, behavior });
      previousScrollTopRef.current = viewport.scrollTop;
    });
  }, []);

  const resumeAutoFollow = useCallback(() => {
    setAutoFollow(true);
    scrollToBottom('smooth');
  }, [scrollToBottom, setAutoFollow]);

  useEffect(() => {
    if (isSending && !wasSendingRef.current) {
      setAutoFollow(true);
    }
    wasSendingRef.current = isSending;
  }, [isSending, setAutoFollow]);

  useEffect(() => {
    if (skipNextAutoScrollRef.current) {
      skipNextAutoScrollRef.current = false;
      return;
    }

    if (autoFollowRef.current) {
      scrollToBottom();
    }
  }, [messages, isSending, scrollToBottom]);

  useEffect(() => {
    if (autoFollowRef.current) {
      scrollToBottom();
    }
  }, [bottomInsetHeight, scrollToBottom]);

  const handleMessagesScroll = useCallback(
    (event: UIEvent<HTMLDivElement>) => {
      const viewport = event.currentTarget;
      const previousScrollTop = previousScrollTopRef.current;
      const currentScrollTop = viewport.scrollTop;
      const distanceFromBottom = getDistanceFromBottom(viewport);
      const isScrollingUp = currentScrollTop < previousScrollTop - 2;
      previousScrollTopRef.current = currentScrollTop;

      if (distanceFromBottom <= resumeDistanceFromBottom) {
        if (!autoFollowRef.current) {
          setAutoFollow(true);
        }
      } else if (
        autoFollowRef.current &&
        isScrollingUp &&
        distanceFromBottom > detachDistanceFromBottom
      ) {
        setAutoFollow(false);
      }

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
      const anchorScrollTop = viewport.scrollTop;
      loadingOlderRequestRef.current = true;
      skipNextAutoScrollRef.current = true;

      void loadOlderMessages()
        .then(() => {
          requestAnimationFrame(() => {
            const nextViewport = scrollViewportRef.current;
            if (!nextViewport) return;
            nextViewport.scrollTop =
              nextViewport.scrollHeight - previousScrollHeight + anchorScrollTop;
          });
        })
        .finally(() => {
          loadingOlderRequestRef.current = false;
        });
    },
    [
      activeMessagePagination.hasMore,
      isLoadingMessages,
      isLoadingOlderMessages,
      loadOlderMessages,
      setAutoFollow,
    ]
  );

  return {
    bottomRef,
    scrollViewportRef,
    handleMessagesScroll,
    isAutoFollowPaused,
    resumeAutoFollow,
  };
}
