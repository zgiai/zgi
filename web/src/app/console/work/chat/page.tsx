'use client';

import React, { Suspense, useCallback, useEffect, useRef, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Chat, { useAIChatController } from '@/components/chat';
import type { AIChatModelValue } from '@/components/chat';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useT } from '@/i18n/translations';
import { useCurrentUser } from '@/store/auth-store';
import { isDraftAIChatConversationId } from '@/components/chat/utils/aichat-message';
import {
  getLastSelectedAiModel,
  saveLastSelectedAiModel,
} from '@/utils/ui-local';

function ChatPageContent() {
  const t = useT('webapp');
  const user = useCurrentUser();
  const router = useRouter();
  const searchParams = useSearchParams();
  const conversationIdParam = searchParams.get('convId');
  const controller = useAIChatController();
  const { init, activeConversationId } = controller;
  const initRef = useRef(init);
  const lastInitializedConversationIdRef = useRef<string | null | undefined>(undefined);

  const [modelSelectorValue, setModelSelectorValue] = useState<AIChatModelValue>(() => {
    if (!user?.id) return { provider: '', model: '', params: {} };
    const saved = getLastSelectedAiModel(user.id, 'consoleChat');
    return saved
      ? { provider: saved.provider, model: saved.model, params: {} }
      : { provider: '', model: '', params: {} };
  });

  useInitializeDefaultModelByUseCase({
    useCase: 'text-chat',
    currentModel: modelSelectorValue,
    enabled: Boolean(user?.id && !getLastSelectedAiModel(user.id, 'consoleChat')),
    onInitialize: value => {
      setModelSelectorValue({
        provider: value.provider,
        model: value.model,
        params: value.params,
      });
    },
  });

  useEffect(() => {
    if (!user?.id || modelSelectorValue.model) return;
    const saved = getLastSelectedAiModel(user.id, 'consoleChat');
    if (!saved) return;

    setModelSelectorValue(previous => ({
      ...previous,
      provider: saved.provider,
      model: saved.model,
    }));
  }, [modelSelectorValue.model, user?.id]);

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
    },
    [user?.id]
  );

  useEffect(() => {
    initRef.current = init;
  }, [init]);

  useEffect(() => {
    if (lastInitializedConversationIdRef.current === conversationIdParam) return;
    lastInitializedConversationIdRef.current = conversationIdParam;
    initRef.current(conversationIdParam);
  }, [conversationIdParam]);

  useEffect(() => {
    const params = new URLSearchParams(searchParams.toString());
    const current = params.get('convId');
    const active = activeConversationId;

    if (isDraftAIChatConversationId(active)) {
      return;
    }

    if (active && current !== active) {
      params.set('convId', active);
      router.replace(`?${params.toString()}`);
      return;
    }

    if (!active && current) {
      params.delete('convId');
      const next = params.toString();
      router.replace(next ? `?${next}` : '/console/work/chat');
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
          onModelChange={handleModelChange}
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
