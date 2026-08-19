'use client';

import { useCallback, useEffect, useMemo } from 'react';
import Link from 'next/link';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { Bot, Settings2 } from 'lucide-react';
import { useStore } from 'zustand';
import { toast } from 'sonner';
import {
  AIChatShell,
  createDigitalHumanRuntimeTransport,
  useChatRuntimeController,
} from '@/components/chat';
import { isPersistedAIChatRuntimeId } from '@/components/chat/utils/aichat-message';
import { Button } from '@/components/ui/button';
import { useDigitalHuman } from '@/hooks/digital-human/use-digital-humans';
import { useT } from '@/i18n';

const conversationParam = 'conversation';

export function DigitalHumanChat({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const detail = useDigitalHuman(id).data?.data;
  const published = detail?.published;
  const transport = useMemo(() => createDigitalHumanRuntimeTransport(id), [id]);
  const controller = useChatRuntimeController({
    transport,
    requireModel: false,
    memoryMutationMode: 'disabled',
  });
  const activeConversationId = useStore(controller.store, state => state.activeConversationId);
  const initialConversationId = searchParams.get(conversationParam);

  const replaceConversationParam = useCallback(
    (conversationId?: string | null) => {
      const params = new URLSearchParams(searchParams.toString());
      if (conversationId) params.set(conversationParam, conversationId);
      else params.delete(conversationParam);
      const query = params.toString();
      router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams]
  );

  useEffect(() => {
    controller.init(initialConversationId);
  }, [controller, initialConversationId]);

  useEffect(() => {
    if (
      isPersistedAIChatRuntimeId(activeConversationId) &&
      activeConversationId !== initialConversationId
    ) {
      replaceConversationParam(activeConversationId);
    }
  }, [activeConversationId, initialConversationId, replaceConversationParam]);

  const selectConversation = useCallback(
    (conversationId: string) => {
      replaceConversationParam(conversationId);
      void controller.select(conversationId);
    },
    [controller, replaceConversationParam]
  );

  const startNewConversation = useCallback(() => {
    replaceConversationParam(null);
    controller.startNew();
  }, [controller, replaceConversationParam]);

  if (!published) {
    return (
      <div className="mx-auto flex min-h-[520px] max-w-5xl items-center justify-center px-4 py-10 md:px-8">
        <div className="max-w-lg rounded-2xl border bg-background p-8 text-center shadow-sm">
          <div className="mx-auto flex size-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
            <Bot className="size-6" />
          </div>
          <h2 className="mt-5 text-xl font-semibold">{t('chat.publishTitle')}</h2>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            {t('configuration.notPublished')}
          </p>
          <Button className="mt-6" asChild>
            <Link href={`/console/digital-humans/${id}/settings`}>
              <Settings2 className="mr-2 size-4" />
              {t('chat.openSettings')}
            </Link>
          </Button>
        </div>
      </div>
    );
  }

  const modelSelectorValue = {
    provider: '',
    model: published.default_model,
    params: {},
  };

  return (
    <div className="h-[calc(100vh-13rem)] min-h-[580px] overflow-hidden border-t bg-background">
      <AIChatShell
        controller={controller}
        modelSelectorValue={modelSelectorValue}
        onModelChange={() => undefined}
        beforeSend={() => {
          if (published) return true;
          toast.error(t('configuration.notPublished'));
          return false;
        }}
        showModelSelector={false}
        requireModel={false}
        showMemoryToggle={false}
        forcedUseMemory={false}
        enableUpload={false}
        showFileLibraryPicker={false}
        allowWorkspaceSwitch={false}
        enableMessageReplay={false}
        showAssistantModelMeta={false}
        surface="digital-human"
        runtimeSurface="external_page_chat"
        defaultSidebarOpen
        themeColor="violet"
        homeBrand={
          <div className="flex size-14 items-center justify-center rounded-2xl bg-primary/10 text-primary shadow-sm">
            <Bot className="size-7" />
          </div>
        }
        homeTitle={t('chat.homeTitle', { name: detail?.definition.name ?? t('title') })}
        homeDescription={t('chat.homeDescription')}
        suggestions={[
          t('chat.suggestions.introduce'),
          t('chat.suggestions.capabilities'),
          t('chat.suggestions.plan'),
        ]}
        inputPlaceholder={t('chat.placeholder')}
        onSelectConversation={selectConversation}
        onStartNewConversation={startNewConversation}
      />
    </div>
  );
}
