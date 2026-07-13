'use client';

import React, { Suspense, useCallback, useEffect, useRef } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Chat, { useAIChatController } from '@/components/chat';
import { usePersistedAIChatModelSelection } from '@/hooks/model/use-persisted-ai-chat-model-selection';
import { useT } from '@/i18n/translations';
import { useCurrentUser } from '@/store/auth-store';
import { isDraftAIChatConversationId } from '@/components/chat/utils/aichat-message';

function ChatPageContent() {
  const t = useT('webapp');
  const user = useCurrentUser();
  const router = useRouter();
  const searchParams = useSearchParams();
  const conversationIdParam = searchParams.get('convId');
  const controller = useAIChatController();
  const { init, startNew, activeConversationId } = controller;
  const initRef = useRef(init);
  const startNewRef = useRef(startNew);
  const lastInitializedConversationIdRef = useRef<string | null | undefined>(undefined);
  const routeSelectionTargetRef = useRef<string | null | undefined>(undefined);
  const { modelSelectorValue, isModelInitializing, handleModelChange } =
    usePersistedAIChatModelSelection({
      accountId: user?.id,
      scope: 'consoleChat',
      useCase: 'text-chat',
    });

  const replaceConversationRoute = useCallback(
    (conversationId: string | null) => {
      const params = new URLSearchParams(searchParams.toString());
      routeSelectionTargetRef.current = conversationId;

      if (conversationId) {
        params.set('convId', conversationId);
      } else {
        params.delete('convId');
      }

      const next = params.toString();
      router.replace(next ? `?${next}` : '/console/work/chat', { scroll: false });
    },
    [router, searchParams]
  );

  const handleSelectConversation = useCallback(
    (conversationId: string) => {
      if (!conversationId) return;
      if (conversationId === conversationIdParam) {
        initRef.current(conversationId);
        return;
      }
      replaceConversationRoute(conversationId);
    },
    [conversationIdParam, replaceConversationRoute]
  );

  const handleStartNewConversation = useCallback(() => {
    routeSelectionTargetRef.current = null;
    startNewRef.current();
    replaceConversationRoute(null);
  }, [replaceConversationRoute]);

  useEffect(() => {
    initRef.current = init;
    startNewRef.current = startNew;
  }, [init, startNew]);

  useEffect(() => {
    if (lastInitializedConversationIdRef.current === conversationIdParam) return;
    lastInitializedConversationIdRef.current = conversationIdParam;
    routeSelectionTargetRef.current = conversationIdParam;
    initRef.current(conversationIdParam);
    if (!conversationIdParam && activeConversationId) {
      startNewRef.current();
    }
  }, [activeConversationId, conversationIdParam]);

  useEffect(() => {
    const params = new URLSearchParams(searchParams.toString());
    const current = params.get('convId');
    const active = activeConversationId;
    const routeSelectionTarget = routeSelectionTargetRef.current;

    // URL-driven selection runs in a separate effect. During that handoff, do not let the
    // previous active conversation rewrite the URL back to its old convId.
    if (routeSelectionTarget !== undefined) {
      if (active === routeSelectionTarget) {
        routeSelectionTargetRef.current = undefined;
      } else if (current === routeSelectionTarget) {
        return;
      }
    }

    if (isDraftAIChatConversationId(active)) {
      return;
    }

    if (active && current !== active) {
      if (current) {
        return;
      }
      params.set('convId', active);
      router.replace(`?${params.toString()}`, { scroll: false });
      return;
    }

    if (!active && current) {
      params.delete('convId');
      const next = params.toString();
      router.replace(next ? `?${next}` : '/console/work/chat', { scroll: false });
    }
  }, [activeConversationId, router, searchParams]);

  return (
    <div className="h-full w-full">
      <React.Suspense
        fallback={
          <div className="flex h-full w-full items-center justify-center text-sm text-muted-foreground">
            {t('consoleChat.loading')}
          </div>
        }
      >
        <Chat
          mode="aichat"
          controller={controller}
          runtimeSurface="work_chat"
          modelSelectorValue={modelSelectorValue}
          isModelInitializing={isModelInitializing}
          onModelChange={handleModelChange}
          showMemoryToggle={false}
          homeTitle={t('consoleChat.homeTitle')}
          homeDescription={t('consoleChat.homeDescription')}
          inputPlaceholder={t('consoleChat.inputPlaceholder')}
          onSelectConversation={handleSelectConversation}
          onStartNewConversation={handleStartNewConversation}
        />
      </React.Suspense>
    </div>
  );
}

export default function ChatPage() {
  return (
    <Suspense fallback={null}>
      <ChatPageContent />
    </Suspense>
  );
}
