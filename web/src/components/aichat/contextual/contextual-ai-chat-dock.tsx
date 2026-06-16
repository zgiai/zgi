'use client';

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react';
import { Sparkles, X } from 'lucide-react';
import { useQueryClient, type QueryKey } from '@tanstack/react-query';
import Chat, { useAIChatController, type AIChatModelValue } from '@/components/chat';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetDescription, SheetTitle } from '@/components/ui/sheet';
import {
  AGENT_KEYS,
  AUTOMATION_KEYS,
  DATASET_KEYS,
  DB_KEYS,
  FILE_KEYS,
  PROMPT_KEYS,
  WORKFLOW_KEYS,
  WORKSPACE_KEYS,
} from '@/hooks/query-keys';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { FILES_QUERY_KEY, FILE_FOLDERS_KEY, STORAGE_USAGE_KEY } from '@/hooks/use-files';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import { cn } from '@/lib/utils';
import {
  createContextualAIChatTransport,
  type ContextualAIChatAssetOperation,
} from './context-envelope';
import { AIChatContextChips } from './context-chips';
import { useContextualAIChat } from './contextual-ai-chat-context';

const LOCAL_STORAGE_KEY = 'consoleChat';
const DESKTOP_PANEL_MEDIA_QUERY = '(min-width: 1024px)';
const DESKTOP_PANEL_WIDTH_STORAGE_KEY = 'consoleChat.aiChatDockWidth';
const DEFAULT_DESKTOP_PANEL_WIDTH_RATIO = 0.3;
const MIN_DESKTOP_PANEL_WIDTH = 640;
const MAX_DESKTOP_PANEL_WIDTH_RATIO = 0.72;
const MIN_DESKTOP_CONTENT_WIDTH = 360;
const ASSET_OPERATION_REFRESH_DEDUPE_MS = 1800;

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

function clampDesktopPanelWidth(width: number) {
  if (typeof window === 'undefined') return Math.max(MIN_DESKTOP_PANEL_WIDTH, width);
  const viewportWidth = window.innerWidth;
  const viewportMax = Math.max(
    MIN_DESKTOP_PANEL_WIDTH,
    viewportWidth - MIN_DESKTOP_CONTENT_WIDTH
  );
  const ratioMax = Math.max(
    MIN_DESKTOP_PANEL_WIDTH,
    Math.round(viewportWidth * MAX_DESKTOP_PANEL_WIDTH_RATIO)
  );
  const maxWidth = Math.min(viewportMax, ratioMax);
  return Math.min(Math.max(Math.round(width), MIN_DESKTOP_PANEL_WIDTH), maxWidth);
}

function getDefaultDesktopPanelWidth() {
  if (typeof window === 'undefined') return MIN_DESKTOP_PANEL_WIDTH;
  return clampDesktopPanelWidth(window.innerWidth * DEFAULT_DESKTOP_PANEL_WIDTH_RATIO);
}

function getStoredDesktopPanelWidth() {
  if (typeof window === 'undefined') return null;
  const stored = window.localStorage.getItem(DESKTOP_PANEL_WIDTH_STORAGE_KEY);
  if (!stored) return null;
  const width = Number.parseInt(stored, 10);
  return Number.isFinite(width) ? clampDesktopPanelWidth(width) : null;
}

function storeDesktopPanelWidth(width: number) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(
    DESKTOP_PANEL_WIDTH_STORAGE_KEY,
    String(clampDesktopPanelWidth(width))
  );
}

function getAssetOperationRefreshKey(operation: ContextualAIChatAssetOperation) {
  return [
    operation.assetType,
    operation.effect,
    operation.toolId ?? operation.toolName,
    operation.assetId ?? operation.assetName ?? 'unknown',
  ].join('|');
}

function pruneAssetOperationRefreshDedupe(cache: Map<string, number>, now: number) {
  for (const [key, timestamp] of cache.entries()) {
    if (now - timestamp > ASSET_OPERATION_REFRESH_DEDUPE_MS * 4) {
      cache.delete(key);
    }
  }
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
  const [desktopPanelWidth, setDesktopPanelWidth] = useState<number | null>(null);
  const itemsRef = useRef(items);
  const assetOperationRefreshRef = useRef<Map<string, number>>(new Map());

  useEffect(() => {
    itemsRef.current = items;
  }, [items]);

  const invalidateQueries = useCallback(
    (queryKey: QueryKey) => {
      void queryClient.invalidateQueries({ queryKey });
    },
    [queryClient]
  );

  const handleAssetOperationSuccess = useCallback(
    (operation: ContextualAIChatAssetOperation) => {
      const now = Date.now();
      const dedupeKey = getAssetOperationRefreshKey(operation);
      const lastRefreshAt = assetOperationRefreshRef.current.get(dedupeKey);
      if (lastRefreshAt && now - lastRefreshAt < ASSET_OPERATION_REFRESH_DEDUPE_MS) {
        return;
      }
      pruneAssetOperationRefreshDedupe(assetOperationRefreshRef.current, now);
      assetOperationRefreshRef.current.set(dedupeKey, now);

      switch (operation.assetType) {
        case 'file':
          invalidateQueries([FILES_QUERY_KEY]);
          invalidateQueries([FILE_FOLDERS_KEY]);
          invalidateQueries([STORAGE_USAGE_KEY]);
          invalidateQueries(FILE_KEYS.all);
          break;
        case 'agent':
          invalidateQueries(AGENT_KEYS.all);
          break;
        case 'workflow':
        case 'workflow_run':
          invalidateQueries(WORKFLOW_KEYS.all);
          invalidateQueries(WORKFLOW_KEYS.runDetails());
          break;
        case 'automation':
          invalidateQueries(AUTOMATION_KEYS.all);
          break;
        case 'knowledge':
        case 'dataset':
        case 'document':
          invalidateQueries(DATASET_KEYS.all);
          break;
        case 'database':
        case 'db':
        case 'database_table':
          invalidateQueries(DB_KEYS.all);
          break;
        case 'prompt':
          invalidateQueries(PROMPT_KEYS.all);
          break;
        case 'workspace':
          invalidateQueries(WORKSPACE_KEYS.all);
          break;
        default:
          break;
      }
    },
    [invalidateQueries]
  );

  const transport = useMemo(
    () =>
      createContextualAIChatTransport(() => itemsRef.current, {
        onAssetOperationSuccess: handleAssetOperationSuccess,
      }),
    [handleAssetOperationSuccess]
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
  useEffect(() => {
    if (!isDesktopPanelViewport) return;
    const resolveWidth = () => {
      setDesktopPanelWidth(previous =>
        clampDesktopPanelWidth(previous ?? getStoredDesktopPanelWidth() ?? getDefaultDesktopPanelWidth())
      );
    };

    resolveWidth();
    window.addEventListener('resize', resolveWidth);
    return () => window.removeEventListener('resize', resolveWidth);
  }, [isDesktopPanelViewport]);

  const handleResizePointerDown = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      if (!isDesktopPanelViewport) return;
      event.preventDefault();
      const startX = event.clientX;
      const startWidth = desktopPanelWidth ?? getDefaultDesktopPanelWidth();
      const previousCursor = document.body.style.cursor;
      const previousUserSelect = document.body.style.userSelect;
      document.body.style.cursor = 'col-resize';
      document.body.style.userSelect = 'none';

      const handlePointerMove = (moveEvent: PointerEvent) => {
        const nextWidth = clampDesktopPanelWidth(startWidth + startX - moveEvent.clientX);
        setDesktopPanelWidth(nextWidth);
        storeDesktopPanelWidth(nextWidth);
      };

      const handlePointerUp = () => {
        document.body.style.cursor = previousCursor;
        document.body.style.userSelect = previousUserSelect;
        window.removeEventListener('pointermove', handlePointerMove);
        window.removeEventListener('pointerup', handlePointerUp);
        window.removeEventListener('pointercancel', handlePointerUp);
      };

      window.addEventListener('pointermove', handlePointerMove);
      window.addEventListener('pointerup', handlePointerUp);
      window.addEventListener('pointercancel', handlePointerUp);
    },
    [desktopPanelWidth, isDesktopPanelViewport]
  );

  const handleResizeKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (!isDesktopPanelViewport) return;
      const currentWidth = desktopPanelWidth ?? getDefaultDesktopPanelWidth();
      let nextWidth: number | null = null;
      if (event.key === 'ArrowLeft') {
        nextWidth = currentWidth + 32;
      } else if (event.key === 'ArrowRight') {
        nextWidth = currentWidth - 32;
      } else if (event.key === 'Home') {
        nextWidth = MIN_DESKTOP_PANEL_WIDTH;
      } else if (event.key === 'End') {
        nextWidth = Number.MAX_SAFE_INTEGER;
      }
      if (nextWidth === null) return;
      event.preventDefault();
      const clampedWidth = clampDesktopPanelWidth(nextWidth);
      setDesktopPanelWidth(clampedWidth);
      storeDesktopPanelWidth(clampedWidth);
    },
    [desktopPanelWidth, isDesktopPanelViewport]
  );

  const desktopPanelStyle = useMemo<CSSProperties>(
    () => ({
      width: desktopPanelWidth ?? `max(${DEFAULT_DESKTOP_PANEL_WIDTH_RATIO * 100}vw, ${MIN_DESKTOP_PANEL_WIDTH}px)`,
    }),
    [desktopPanelWidth]
  );

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
        className="relative hidden h-full min-h-0 min-w-[640px] shrink-0 border-l border-border/70 bg-background shadow-sm lg:flex"
        style={desktopPanelStyle}
      >
        <div
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize AIChat assistant"
          tabIndex={0}
          title="Drag to resize AIChat assistant"
          className="group absolute inset-y-0 left-0 z-50 flex w-3 -translate-x-1/2 cursor-col-resize items-center justify-center outline-none"
          onPointerDown={handleResizePointerDown}
          onKeyDown={handleResizeKeyDown}
        >
          <span className="h-12 w-1 rounded-full bg-border opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100" />
        </div>
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
