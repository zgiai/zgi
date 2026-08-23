'use client';

import { useCallback, useEffect, useMemo, useRef } from 'react';
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
import { AIChatEmbeddedConversationControls } from '@/components/chat/variants/aichat/embedded-conversation-controls';
import {
  resolveConversationRouteSync,
  shouldStartNewConversationForRoute,
  type ConversationRouteHandoff,
} from '@/components/chat/runtime/conversation-route-handoff';
import { isConversationRouteRestoring } from '@/components/chat/runtime/conversation-route-state';
import { isDraftAIChatConversationId } from '@/components/chat/utils/aichat-message';
import { Button } from '@/components/ui/button';
import { useDigitalHuman } from '@/hooks/digital-human/use-digital-humans';
import { useT } from '@/i18n';

const conversationParam = 'conversation';

interface DigitalHumanChatProps {
  id: string;
  controlsPortalId?: string;
}

export function DigitalHumanChat({ id, controlsPortalId }: DigitalHumanChatProps) {
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
  const initController = controller.init;
  const startNewConversation = controller.startNew;
  const activeConversationId = useStore(controller.store, state => state.activeConversationId);
  const initialConversationId = searchParams.get(conversationParam);
  const initControllerRef = useRef(initController);
  const startNewConversationRef = useRef(startNewConversation);
  const lastInitializedConversationIdRef = useRef<string | null | undefined>(undefined);
  const routeHandoffRef = useRef<ConversationRouteHandoff | undefined>(undefined);
  const isRestoringConversationRoute = isConversationRouteRestoring(
    initialConversationId,
    activeConversationId
  );

  const replaceConversationParam = useCallback(
    (conversationId?: string | null, nullMode: 'new-chat' | 'draft-persistence' = 'new-chat') => {
      const params = new URLSearchParams(searchParams.toString());
      routeHandoffRef.current = conversationId
        ? { conversationId, mode: 'selection' }
        : { conversationId: null, mode: nullMode };
      if (conversationId) params.set(conversationParam, conversationId);
      else params.delete(conversationParam);
      const query = params.toString();
      router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams]
  );

  useEffect(() => {
    initControllerRef.current = initController;
    startNewConversationRef.current = startNewConversation;
  }, [initController, startNewConversation]);

  useEffect(() => {
    if (lastInitializedConversationIdRef.current === initialConversationId) return;
    lastInitializedConversationIdRef.current = initialConversationId;
    routeHandoffRef.current = initialConversationId
      ? { conversationId: initialConversationId, mode: 'selection' }
      : {
          conversationId: null,
          mode: isDraftAIChatConversationId(activeConversationId)
            ? 'draft-persistence'
            : 'new-chat',
        };
    initControllerRef.current(initialConversationId);
    if (
      shouldStartNewConversationForRoute(
        initialConversationId,
        activeConversationId,
        isDraftAIChatConversationId(activeConversationId)
      )
    ) {
      startNewConversationRef.current();
    }
  }, [activeConversationId, initialConversationId]);

  useEffect(() => {
    const decision = resolveConversationRouteSync({
      activeConversationId,
      currentConversationId: initialConversationId,
      routeHandoff: routeHandoffRef.current,
      activeConversationIsDraft: isDraftAIChatConversationId(activeConversationId),
    });
    routeHandoffRef.current = decision.routeHandoff;

    if (decision.action.type === 'replace') {
      replaceConversationParam(decision.action.conversationId);
    } else if (decision.action.type === 'clear') {
      replaceConversationParam(null);
    }
  }, [activeConversationId, initialConversationId, replaceConversationParam]);

  const selectConversation = useCallback(
    (conversationId: string) => {
      if (!conversationId) return;
      if (conversationId === initialConversationId) {
        initControllerRef.current(conversationId);
        return;
      }
      replaceConversationParam(conversationId);
    },
    [initialConversationId, replaceConversationParam]
  );

  const handleStartNewConversation = useCallback(() => {
    startNewConversationRef.current();
    replaceConversationParam(null, 'draft-persistence');
  }, [replaceConversationParam]);

  if (!published) {
    return (
      <div className="mx-auto flex h-full max-w-5xl items-center justify-center px-4 py-10 md:px-8">
        <div className="max-w-lg rounded-2xl border bg-background p-8 text-center shadow-sm">
          <div className="mx-auto flex size-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
            <Bot className="size-6" />
          </div>
          <h2 className="mt-5 text-xl font-semibold">{t('chat.publishTitle')}</h2>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            {t('configuration.notPublished')}
          </p>
          <Button className="mt-6" asChild>
            <Link href={`/console/digital-humans/${id}/configuration?section=identity`}>
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
    <div className="h-full min-h-0 overflow-hidden bg-background">
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
        variant="embedded"
        embeddedConversationMode="drawer"
        embeddedConversationControlsMode="external"
        embeddedConversationControlsPortalId={controlsPortalId}
        renderEmbeddedConversationControls={controls => (
          <AIChatEmbeddedConversationControls
            openConversations={controls.openConversations}
            startNewConversation={controls.startNewConversation}
            conversationsLabel={t('chat.conversations')}
            newConversationLabel={t('chat.newConversation')}
          />
        )}
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
        defaultSidebarOpen={false}
        routeConversationId={initialConversationId}
        isConversationTransitioning={isRestoringConversationRoute}
        conversationListTitle={t('chat.conversations')}
        conversationListDescription={t('chat.conversationListDescription')}
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
        onStartNewConversation={handleStartNewConversation}
      />
    </div>
  );
}
