'use client';

import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react';
import { ChevronDown, Sparkles, X } from 'lucide-react';
import { useQueryClient, type QueryKey } from '@tanstack/react-query';
import Chat, { useAIChatController, type AIChatModelValue } from '@/components/chat';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
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
import { useT } from '@/i18n/translations';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import {
  createContextualAIChatTransport,
  type ContextualAIChatAssetOperation,
} from './context-envelope';
import { useContextualAIChat } from './contextual-ai-chat-context';
import type { AIChatContextItem } from './types';

const LOCAL_STORAGE_KEY = 'consoleChat';
const DESKTOP_PANEL_MEDIA_QUERY = '(min-width: 1440px)';
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

type ContextualDockTranslator = ReturnType<typeof useT<'webapp'>>;

function isFilesPageContext(item: AIChatContextItem) {
  return (
    item.type === 'page' &&
    (item.id === 'console.files' || item.metadata?.page === 'console.files')
  );
}

function getNumberMetadata(item: AIChatContextItem | undefined, key: string) {
  const value = item?.metadata?.[key];
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function hasCapability(item: AIChatContextItem | undefined, capabilityId: string) {
  return Boolean(item?.capabilities?.some(capability => capability.id === capabilityId));
}

function getContextTypeLabel(
  type: AIChatContextItem['type'],
  t: ContextualDockTranslator
) {
  switch (type) {
    case 'agent':
      return t('consoleChat.contextual.contextTypes.agent');
    case 'workflow':
      return t('consoleChat.contextual.contextTypes.workflow');
    case 'file':
      return t('consoleChat.contextual.contextTypes.file');
    case 'task':
      return t('consoleChat.contextual.contextTypes.task');
    case 'dataset':
      return t('consoleChat.contextual.contextTypes.dataset');
    case 'database':
      return t('consoleChat.contextual.contextTypes.database');
    case 'log':
      return t('consoleChat.contextual.contextTypes.log');
    case 'selection':
      return t('consoleChat.contextual.contextTypes.selection');
    case 'page':
      return t('consoleChat.contextual.contextTypes.page');
    default:
      return t('consoleChat.contextual.contextTypes.context');
  }
}

function pickPrimaryContextItem(items: AIChatContextItem[]) {
  return (
    items.find(item => item.type === 'file') ??
    items.find(item => item.type === 'agent') ??
    items.find(item => item.type === 'workflow') ??
    items.find(item => item.type === 'page') ??
    items[0]
  );
}

function buildContextSummary(items: AIChatContextItem[], t: ContextualDockTranslator) {
  const primaryItem = pickPrimaryContextItem(items);
  if (!primaryItem) return t('consoleChat.contextual.contextSummaryEmpty');
  return t('consoleChat.contextual.contextSummaryItem', {
    type: getContextTypeLabel(primaryItem.type, t),
    title: primaryItem.title,
  });
}

function buildSuggestions(
  contextItems: ReturnType<typeof useContextualAIChat>['items'],
  t: ContextualDockTranslator
) {
  const filesPage = contextItems.find(isFilesPageContext);
  if (filesPage) {
    const visibleFileCount = getNumberMetadata(filesPage, 'visible_file_count');
    const selectedFileCount = getNumberMetadata(filesPage, 'selected_file_count');
    const canDelete = hasCapability(filesPage, 'file.delete');

    const suggestions = [
      t('consoleChat.contextual.suggestions.filesListVisible'),
      selectedFileCount > 0
        ? t('consoleChat.contextual.suggestions.filesSummarizeSelected')
        : visibleFileCount > 0
          ? t('consoleChat.contextual.suggestions.filesSummarizeFirst')
          : t('consoleChat.contextual.suggestions.filesExplainEmpty'),
      t('consoleChat.contextual.suggestions.filesOrganizeVisible'),
    ];

    if (selectedFileCount === 1 && canDelete) {
      suggestions.push(t('consoleChat.contextual.suggestions.filesDeleteSelected'));
    }

    return suggestions;
  }

  const firstAgent = contextItems.find(item => item.type === 'agent');
  if (firstAgent) {
    return [
      t('consoleChat.contextual.suggestions.agentReview', { title: firstAgent.title }),
      t('consoleChat.contextual.suggestions.agentTestQuestions', { title: firstAgent.title }),
      t('consoleChat.contextual.suggestions.agentRisks', { title: firstAgent.title }),
    ];
  }

  if (contextItems.length > 0) {
    return [
      t('consoleChat.contextual.suggestions.pageSummarize'),
      t('consoleChat.contextual.suggestions.pageNextSteps'),
      t('consoleChat.contextual.suggestions.pageExplainContext'),
    ];
  }

  return [
    t('consoleChat.contextual.suggestions.emptySummarize'),
    t('consoleChat.contextual.suggestions.emptyNextSteps'),
    t('consoleChat.contextual.suggestions.emptyReview'),
  ];
}

interface ContextSummaryMenuProps {
  items: AIChatContextItem[];
  t: ContextualDockTranslator;
}

function ContextSummaryMenu({ items, t }: ContextSummaryMenuProps) {
  const summary = buildContextSummary(items, t);
  const hasContext = items.length > 0;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="min-w-0 flex-1 !shrink justify-start rounded-full border border-border/70 bg-muted/30 px-3 text-left font-normal text-foreground hover:bg-muted/60"
          title={summary}
        >
          <span className="min-w-0 flex-1 truncate">{summary}</span>
          {hasContext ? (
            <span className="ml-2 shrink-0 rounded-full bg-background px-2 py-0.5 text-[11px] leading-none text-muted-foreground">
              {t('consoleChat.contextual.contextItems', { count: items.length })}
            </span>
          ) : null}
          <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-80 max-w-[calc(100vw-2rem)] p-2">
        <div className="px-2 pb-1 text-xs font-medium text-muted-foreground">
          {t('consoleChat.contextual.contextSummaryDetails')}
        </div>
        <DropdownMenuSeparator />
        {hasContext ? (
          <div className="max-h-72 space-y-1 overflow-y-auto">
            {items.map(item => (
              <div
                key={`${item.type}:${item.id}`}
                className="min-w-0 rounded-md px-2 py-2 text-sm"
              >
                <div className="flex min-w-0 items-start gap-2">
                  <span className="mt-0.5 shrink-0 rounded bg-muted px-1.5 py-0.5 text-[11px] font-medium leading-none text-muted-foreground">
                    {getContextTypeLabel(item.type, t)}
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium text-foreground" title={item.title}>
                      {item.title}
                    </div>
                    {item.subtitle ? (
                      <div className="mt-0.5 truncate text-xs text-muted-foreground">
                        {item.subtitle}
                      </div>
                    ) : null}
                  </div>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="px-2 py-3 text-sm text-muted-foreground">
            {t('consoleChat.contextual.contextSummaryEmpty')}
          </div>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ContextBrand({ label }: { label: string }) {
  return (
    <div className="flex size-10 items-center justify-center rounded-full border border-primary/20 bg-primary/10 text-primary">
      <Sparkles className="size-5" />
      <span className="sr-only">{label}</span>
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
  t: ContextualDockTranslator;
  enableToolGovernance: boolean;
}

function ContextualAIChatPanel({
  controller,
  enableToolGovernance,
  isModelInitializing,
  items,
  modelSelectorValue,
  onClose,
  onModelChange,
  suggestions,
  t,
}: ContextualAIChatPanelProps) {
  const controlsPortalId = useId();
  const filesPage = items.find(isFilesPageContext);
  const hasContext = items.length > 0;
  const homeTitle = filesPage
    ? t('consoleChat.contextual.home.filesTitle')
    : hasContext
      ? t('consoleChat.contextual.home.contextTitle')
      : t('consoleChat.contextual.home.emptyTitle');
  const homeDescription = filesPage
    ? t('consoleChat.contextual.home.filesDescription')
    : hasContext
      ? t('consoleChat.contextual.home.contextDescription')
      : t('consoleChat.contextual.home.emptyDescription');
  const inputPlaceholder = filesPage
    ? t('consoleChat.contextual.input.filesPlaceholder')
    : t('consoleChat.contextual.input.placeholder');

  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-14 shrink-0 items-center gap-2 border-b border-border/70 bg-background/95 px-3 py-2">
        <div id={controlsPortalId} className="flex shrink-0 items-center" />
        <ContextSummaryMenu items={items} t={t} />
        <Button
          type="button"
          variant="ghost"
          size="default"
          isIcon
          className="size-8 text-muted-foreground hover:bg-muted hover:text-foreground"
          onClick={onClose}
          title={t('consoleChat.contextual.close')}
        >
          <X className="size-4" />
          <span className="sr-only">{t('consoleChat.contextual.close')}</span>
        </Button>
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
          embeddedConversationControlsMode="external"
          embeddedConversationControlsPortalId={controlsPortalId}
          allowWorkspaceSwitch
          homeBrand={
            <ContextBrand
              label={t('consoleChat.contextual.contextItems', { count: items.length })}
            />
          }
          homeTitle={homeTitle}
          homeDescription={homeDescription}
          inputPlaceholder={inputPlaceholder}
          suggestions={suggestions}
          enableToolGovernance={enableToolGovernance}
        />
      </div>
    </div>
  );
}

export function ContextualAIChatDock() {
  const t = useT('webapp');
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

  const enableToolGovernance = useMemo(() => items.some(isFilesPageContext), [items]);
  const suggestions = useMemo(() => buildSuggestions(items, t), [items, t]);
  useEffect(() => {
    if (!isDesktopPanelViewport) return;
    const resolveWidth = () => {
      setDesktopPanelWidth(previous =>
        clampDesktopPanelWidth(
          previous ?? getStoredDesktopPanelWidth() ?? getDefaultDesktopPanelWidth()
        )
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
      width:
        desktopPanelWidth ??
        `max(${DEFAULT_DESKTOP_PANEL_WIDTH_RATIO * 100}vw, ${MIN_DESKTOP_PANEL_WIDTH}px)`,
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
      t={t}
      enableToolGovernance={enableToolGovernance}
    />
  );

  if (isDesktopPanelViewport === null) {
    return null;
  }

  if (isDesktopPanelViewport) {
    return isOpen ? (
      <aside
        aria-label={t('consoleChat.contextual.assistantLabel')}
        className="relative hidden h-full min-h-0 min-w-[640px] shrink-0 overflow-hidden border-l border-border/70 bg-background shadow-sm lg:flex"
        style={desktopPanelStyle}
      >
        <div
          role="separator"
          aria-orientation="vertical"
          aria-label={t('consoleChat.contextual.resize')}
          tabIndex={0}
          title={t('consoleChat.contextual.resizeHint')}
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
        <SheetTitle className="sr-only">{t('consoleChat.contextual.assistantLabel')}</SheetTitle>
        <SheetDescription className="sr-only">
          {t('consoleChat.contextual.sheetDescription')}
        </SheetDescription>
        {panel}
      </SheetContent>
    </Sheet>
  );
}
