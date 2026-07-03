'use client';

import React, { Suspense, useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Chat, { useAIChatController } from '@/components/chat';
import type { AIChatModelValue } from '@/components/chat';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useT } from '@/i18n/translations';
import { useCurrentUser } from '@/store/auth-store';
import { isDraftAIChatConversationId } from '@/components/chat/utils/aichat-message';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';

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

  const [modelSelectorValue, setModelSelectorValue] = useState<AIChatModelValue>(() => {
    if (!user?.id) return { provider: '', model: '', params: {} };
    const saved = getLastSelectedAiModel(user.id, 'consoleChat');
    return saved
      ? { provider: saved.provider, model: saved.model, params: {} }
      : { provider: '', model: '', params: {} };
  });
  const [isInitialModelResolved, setIsInitialModelResolved] = useState(() => {
    if (!user?.id) return false;
    return Boolean(getLastSelectedAiModel(user.id, 'consoleChat'));
  });

  const shouldInitializeDefaultModel = Boolean(
    user?.id && !getLastSelectedAiModel(user.id, 'consoleChat')
  );
  const defaultModelInitialization = useInitializeDefaultModelByUseCase({
    useCase: 'text-chat',
    currentModel: modelSelectorValue,
    enabled: shouldInitializeDefaultModel,
    onInitialize: value => {
      setModelSelectorValue({
        provider: value.provider,
        model: value.model,
        params: value.params,
      });
      setIsInitialModelResolved(true);
    },
  });
  const isModelInitializing = !modelSelectorValue.model && !isInitialModelResolved;

  useLayoutEffect(() => {
    if (!user?.id) {
      setIsInitialModelResolved(false);
      return;
    }
    if (modelSelectorValue.model) {
      setIsInitialModelResolved(true);
      return;
    }
    const saved = getLastSelectedAiModel(user.id, 'consoleChat');
    if (!saved) {
      if (defaultModelInitialization.isResolved && !defaultModelInitialization.value) {
        setIsInitialModelResolved(true);
      }
      return;
    }

    setModelSelectorValue(previous => ({
      ...previous,
      provider: saved.provider,
      model: saved.model,
    }));
    setIsInitialModelResolved(true);
  }, [
    defaultModelInitialization.isResolved,
    defaultModelInitialization.value,
    modelSelectorValue.model,
    user?.id,
  ]);

  const handleModelChange = useCallback(
    (value: ModelSelectorValue) => {
      setModelSelectorValue(previous => ({
        ...previous,
        provider: value.provider,
        model: value.model,
      }));

      if (user?.id) {
        saveLastSelectedAiModel(user.id, 'consoleChat', {
          provider: value.provider,
          model: value.model,
        });
      }
      setIsInitialModelResolved(true);
    },
    [user?.id]
  );

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
          modelSelectorValue={modelSelectorValue}
          isModelInitializing={isModelInitializing}
          onModelChange={handleModelChange}
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
