'use client';

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { Sparkles, X } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import Chat, { useAIChatController, type AIChatModelValue } from '@/components/chat';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetDescription, SheetTitle } from '@/components/ui/sheet';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { FILES_QUERY_KEY, STORAGE_USAGE_KEY } from '@/hooks/use-files';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import { cn } from '@/lib/utils';
import { createContextualAIChatTransport } from './context-envelope';
import { AIChatContextChips } from './context-chips';
import { useContextualAIChat } from './contextual-ai-chat-context';

const LOCAL_STORAGE_KEY = 'consoleChat';
const DESKTOP_PANEL_MEDIA_QUERY = '(min-width: 1024px)';

function getIsDesktopPanelViewport() {
  return (
    typeof window !== 'undefined' && window.matchMedia(DESKTOP_PANEL_MEDIA_QUERY).matches
  );
}

function useIsDesktopPanelViewport() {
  const [isDesktopPanelViewport, setIsDesktopPanelViewport] = useState<boolean | null>(null);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const mediaQuery = window.matchMedia(DESKTOP_PANEL_MEDIA_QUERY);
    const handleChange = () => setIsDesktopPanelViewport(mediaQuery.matches);

    handleChange();
    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, []);

  return isDesktopPanelViewport;
}

function buildSuggestions(contextItems: ReturnType<typeof useContextualAIChat>['items']) {
  const firstAgent = contextItems.find(item => item.type === 'agent');
  if (firstAgent) {
    return [
      `Check whether ${firstAgent.title} is ready to publish`,
      `Generate test questions for ${firstAgent.title}`,
      `Explain the main risks for ${firstAgent.title}`,
    ];
  }

  if (contextItems.length > 0) {
    return [
      'Summarize the current page context',
      'Tell me what I can do next',
      'Create a safe action plan for this task',
    ];
  }

  return ['Help me create an Agent', 'Review what I am working on', 'Plan a task I can automate'];
}

function ContextBrand({ itemCount }: { itemCount: number }) {
  return (
    <div className="flex size-10 items-center justify-center rounded-full border border-primary/20 bg-primary/10 text-primary">
      <Sparkles className="size-5" />
      <span className="sr-only">{itemCount} context items</span>
    </div>
  );
}

interface ContextualAIChatPanelProps {
  controller: ReturnType<typeof useAIChatController>;
  isModelInitializing: boolean;
  items: ReturnType<typeof useContextualAIChat>['items'];
  modelSelectorValue: AIChatModelValue;
  onClose: () => void;
  onModelChange: (value: ModelSelectorValue) => void;
  suggestions: string[];
}

function ContextualAIChatPanel({
  controller,
  isModelInitializing,
  items,
  modelSelectorValue,
  onClose,
  onModelChange,
  suggestions,
}: ContextualAIChatPanelProps) {
  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      <Button
        type="button"
        variant="ghost"
        size="default"
        isIcon
        className="absolute right-3 top-3 z-40 size-8 bg-background/90 text-muted-foreground shadow-sm hover:bg-muted hover:text-foreground"
        onClick={onClose}
        title="Close AIChat assistant"
      >
        <X className="size-4" />
        <span className="sr-only">Close AIChat assistant</span>
      </Button>
      <div
        className={cn(
          'border-b border-border/70 bg-background/95 px-4 py-3 pr-14',
          items.length === 0 && 'sr-only'
        )}
      >
        <AIChatContextChips items={items} maxVisible={3} />
      </div>
      <div className="min-h-0 flex-1">
        <Chat
          mode="aichat"
          controller={controller}
          modelSelectorValue={modelSelectorValue}
          isModelInitializing={isModelInitializing}
          onModelChange={onModelChange}
          variant="embedded"
          embeddedConversationMode="drawer"
          allowWorkspaceSwitch
          homeBrand={<ContextBrand itemCount={items.length} />}
          homeTitle={items.length > 0 ? 'Work with the current context' : 'AIChat assistant'}
          homeDescription={
            items.length > 0
              ? 'AIChat will include the visible context chips in this turn.'
              : 'Ask AIChat to help create, review, or plan work in ZGI.'
          }
          inputPlaceholder="Ask about this page or tell AIChat what to do..."
          suggestions={suggestions}
        />
      </div>
    </div>
  );
}

export function ContextualAIChatDock() {
  const user = useCurrentUser();
  const queryClient = useQueryClient();
  const { isOpen, setOpen, items } = useContextualAIChat();
  const isDesktopPanelViewport = useIsDesktopPanelViewport();
  const itemsRef = useRef(items);

  useEffect(() => {
    itemsRef.current = items;
  }, [items]);

  const handleAssetToolSuccess = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] });
    void queryClient.invalidateQueries({ queryKey: [STORAGE_USAGE_KEY] });
  }, [queryClient]);

  const transport = useMemo(
    () =>
      createContextualAIChatTransport(() => itemsRef.current, {
        onAssetToolSuccess: handleAssetToolSuccess,
      }),
    [handleAssetToolSuccess]
  );
  const controller = useAIChatController({ transport });
  const { init: initController } = controller;

  useEffect(() => {
    initController();
  }, [initController]);

  const [modelSelectorValue, setModelSelectorValue] = useState<AIChatModelValue>(() => {
    if (!user?.id) return { provider: '', model: '', params: {} };
    const saved = getLastSelectedAiModel(user.id, LOCAL_STORAGE_KEY);
    return saved
      ? { provider: saved.provider, model: saved.model, params: {} }
      : { provider: '', model: '', params: {} };
  });
  const [isInitialModelResolved, setIsInitialModelResolved] = useState(() => {
    if (!user?.id) return false;
    return Boolean(getLastSelectedAiModel(user.id, LOCAL_STORAGE_KEY));
  });

  const shouldInitializeDefaultModel = Boolean(
    user?.id && !getLastSelectedAiModel(user.id, LOCAL_STORAGE_KEY)
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
    const saved = getLastSelectedAiModel(user.id, LOCAL_STORAGE_KEY);
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
        saveLastSelectedAiModel(user.id, LOCAL_STORAGE_KEY, {
          provider: value.provider,
          model: value.model,
        });
      }
      setIsInitialModelResolved(true);
    },
    [user?.id]
  );

  const suggestions = useMemo(() => buildSuggestions(items), [items]);
  const panel = (
    <ContextualAIChatPanel
      controller={controller}
      isModelInitializing={isModelInitializing}
      items={items}
      modelSelectorValue={modelSelectorValue}
      onClose={() => setOpen(false)}
      onModelChange={handleModelChange}
      suggestions={suggestions}
    />
  );

  if (isDesktopPanelViewport === null) {
    return null;
  }

  if (isDesktopPanelViewport) {
    return isOpen ? (
      <aside
        aria-label="AIChat assistant"
        className="hidden h-full min-h-0 w-[440px] shrink-0 border-l border-border/70 bg-background shadow-sm lg:flex"
      >
        {panel}
      </aside>
    ) : null;
  }

  return (
    <Sheet open={isOpen} onOpenChange={setOpen}>
      <SheetContent
        side="right"
        showClose={false}
        overlayClassName="bg-transparent backdrop-blur-none"
        className="flex h-full min-h-0 w-[min(720px,100vw)] max-w-none flex-col overflow-hidden p-0 sm:max-w-none"
      >
        <SheetTitle className="sr-only">AIChat assistant</SheetTitle>
        <SheetDescription className="sr-only">
          Ask AIChat to help with the current ZGI page.
        </SheetDescription>
        {panel}
      </SheetContent>
    </Sheet>
  );
}
