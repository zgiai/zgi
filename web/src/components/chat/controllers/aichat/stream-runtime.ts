/* eslint-disable react-hooks/exhaustive-deps */
import { useCallback, useEffect, useRef } from 'react';
import type { MutableRefObject } from 'react';
import type { AIChatRecoveryMode, AIChatSetControllerState } from './types';
import { getNextActiveSendingState } from './selectors';

export interface AIChatStreamRuntime {
  pendingStreamAbortRef: MutableRefObject<AbortController | null>;
  streamAbortByConversationRef: MutableRefObject<Record<string, AbortController>>;
  streamingMessageRef: MutableRefObject<{ conversationId: string; messageId: string } | null>;
  recoveryAbortByConversationRef: MutableRefObject<Record<string, AbortController>>;
  recoveryRetryTimeoutsRef: MutableRefObject<Record<string, ReturnType<typeof setTimeout>>>;
  recoveryModeByConversationRef: MutableRefObject<Record<string, AIChatRecoveryMode>>;
  backgroundConversationIdRef: MutableRefObject<string | null>;
  clearRecoveryRetry: (conversationId: string) => void;
  closeRecoveryConnection: (conversationId: string) => void;
  closeStreamConnection: (conversationId: string) => void;
  closeConversationConnection: (conversationId: string) => void;
  setBackgroundConversation: (conversationId: string | null) => void;
  setRecoveryMode: (conversationId: string, mode: AIChatRecoveryMode) => void;
}

export type AIChatStreamManager = AIChatStreamRuntime;

/**
 * @hook useAIChatStreamRuntime
 * @description Owns AIChat stream/recovery refs and connection lifecycle cleanup.
 */
export function useAIChatStreamRuntime(
  setControllerState: AIChatSetControllerState
): AIChatStreamRuntime {
  const pendingStreamAbortRef = useRef<AbortController | null>(null);
  const streamAbortByConversationRef = useRef<Record<string, AbortController>>({});
  const streamingMessageRef = useRef<{ conversationId: string; messageId: string } | null>(null);
  const recoveryAbortByConversationRef = useRef<Record<string, AbortController>>({});
  const recoveryRetryTimeoutsRef = useRef<Record<string, ReturnType<typeof setTimeout>>>({});
  const recoveryModeByConversationRef = useRef<Record<string, AIChatRecoveryMode>>({});
  const backgroundConversationIdRef = useRef<string | null>(null);

  const abortAllRecoveryConnections = useCallback(() => {
    Object.values(recoveryAbortByConversationRef.current).forEach(controller =>
      controller.abort()
    );
    Object.values(recoveryRetryTimeoutsRef.current).forEach(timeout => clearTimeout(timeout));
    recoveryAbortByConversationRef.current = {};
    recoveryRetryTimeoutsRef.current = {};
    recoveryModeByConversationRef.current = {};
    backgroundConversationIdRef.current = null;
  }, []);

  // Reads latest refs in cleanup so every live controller is aborted on unmount.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    return () => {
      pendingStreamAbortRef.current?.abort();
      Object.values(streamAbortByConversationRef.current).forEach(controller =>
        controller.abort()
      );
      streamAbortByConversationRef.current = {};
      abortAllRecoveryConnections();
      streamingMessageRef.current = null;
    };
  }, [abortAllRecoveryConnections]);

  const clearRecoveryRetry = useCallback((conversationId: string) => {
    const timeout = recoveryRetryTimeoutsRef.current[conversationId];
    if (timeout) {
      clearTimeout(timeout);
      delete recoveryRetryTimeoutsRef.current[conversationId];
    }
  }, []);

  const closeRecoveryConnection = useCallback(
    (conversationId: string) => {
      recoveryAbortByConversationRef.current[conversationId]?.abort();
      delete recoveryAbortByConversationRef.current[conversationId];
      delete recoveryModeByConversationRef.current[conversationId];
      if (backgroundConversationIdRef.current === conversationId) {
        backgroundConversationIdRef.current = null;
      }
      clearRecoveryRetry(conversationId);
      setControllerState(current => ({
        ...current,
        isSending: getNextActiveSendingState(current, conversationId, false),
        recoveringByConversation: {
          ...current.recoveringByConversation,
          [conversationId]: false,
        },
      }));
    },
    [clearRecoveryRetry, setControllerState]
  );

  const closeStreamConnection = useCallback((conversationId: string) => {
    streamAbortByConversationRef.current[conversationId]?.abort();
    delete streamAbortByConversationRef.current[conversationId];
    if (backgroundConversationIdRef.current === conversationId) {
      backgroundConversationIdRef.current = null;
    }
  }, []);

  const closeConversationConnection = useCallback(
    (conversationId: string) => {
      closeRecoveryConnection(conversationId);
      closeStreamConnection(conversationId);
    },
    [closeRecoveryConnection, closeStreamConnection]
  );

  const setBackgroundConversation = useCallback(
    (conversationId: string | null) => {
      const previousBackgroundConversationId = backgroundConversationIdRef.current;
      if (
        previousBackgroundConversationId &&
        previousBackgroundConversationId !== conversationId
      ) {
        closeConversationConnection(previousBackgroundConversationId);
      }
      backgroundConversationIdRef.current = conversationId;
    },
    [closeConversationConnection]
  );

  const setRecoveryMode = useCallback(
    (conversationId: string, mode: AIChatRecoveryMode) => {
      if (mode === 'background') {
        setBackgroundConversation(conversationId);
      } else if (backgroundConversationIdRef.current === conversationId) {
        backgroundConversationIdRef.current = null;
      }
      recoveryModeByConversationRef.current[conversationId] = mode;
    },
    [setBackgroundConversation]
  );

  return {
    pendingStreamAbortRef,
    streamAbortByConversationRef,
    streamingMessageRef,
    recoveryAbortByConversationRef,
    recoveryRetryTimeoutsRef,
    recoveryModeByConversationRef,
    backgroundConversationIdRef,
    clearRecoveryRetry,
    closeRecoveryConnection,
    closeStreamConnection,
    closeConversationConnection,
    setBackgroundConversation,
    setRecoveryMode,
  };
}
