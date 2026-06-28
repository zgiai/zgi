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
import { usePathname, useRouter } from 'next/navigation';
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
import {
  AGENT_KEYS,
  AUTOMATION_KEYS,
  DATASET_KEYS,
  DB_KEYS,
  PROMPT_KEYS,
  WORKFLOW_KEYS,
  WORKSPACE_KEYS,
} from '@/hooks/query-keys';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useT } from '@/i18n/translations';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import { embeddedControlButtonClassName } from '@/components/chat/variants/aichat/embedded-conversation-controls';
import {
  createContextualAIChatTransport,
  normalizeZGIConsoleNavigationHref,
  type ContextualAIChatAssetOperation,
  type ContextualAIChatClientActionRequest,
} from './context-envelope';
import { useContextualAIChat } from './contextual-ai-chat-context';
import type {
  AIChatClientActionRequiredEventData,
  AIChatClientActionResultRequest,
  AIChatMessage,
} from '@/services/types/aichat';
import type {
  AIChatContextItem,
  AIChatContextPresentationHint,
  AIChatContextRefreshHint,
} from './types';

const LOCAL_STORAGE_KEY = 'consoleChat';
const DESKTOP_PANEL_MEDIA_QUERY = '(min-width: 1280px)';
const DESKTOP_PANEL_WIDTH_STORAGE_KEY = 'consoleChat.aiChatDockWidth';
const DEFAULT_DESKTOP_PANEL_WIDTH_RATIO = 0.3;
const MIN_DESKTOP_PANEL_WIDTH = 640;
const MAX_DESKTOP_PANEL_WIDTH_RATIO = 0.72;
const MIN_DESKTOP_CONTENT_WIDTH = 360;
const ASSET_OPERATION_REFRESH_DEDUPE_MS = 1800;
const CLIENT_ACTION_ROUTE_TIMEOUT_MS = 10_000;
const CLIENT_ACTION_ROUTE_CONTEXT_SETTLE_MS = 140;
const CLIENT_ACTION_ROUTE_FALLBACK_SETTLE_MS = 460;
const CLIENT_ACTION_ROUTE_CONTEXT_POLL_MS = 120;
const CLIENT_ACTION_OBSERVATION_SETTLE_MS = 900;
const CLIENT_ACTION_DEDUPE_TTL_MS = 60_000;
const CLIENT_ACTION_FAILURE_DEDUPE_TTL_MS = 5_000;

interface PendingClientActionContinuation {
  key: string;
  conversationId: string;
  messageId: string;
  actionId: string;
  actionType: string;
  href?: string;
  label?: string;
  reason?: string;
  effect?: string;
  assetType?: string;
  assets?: Array<Record<string, unknown>>;
  requestedAt: number;
  completed: boolean;
  timeoutId?: number;
  resuming?: boolean;
}

function clientActionContinuationKey(
  conversationId: string,
  messageId: string,
  actionId: string
) {
  return `${conversationId}:${messageId}:${actionId}`;
}

function clientActionRequestKey(request: ContextualAIChatClientActionRequest) {
  return clientActionContinuationKey(
    request.conversationId,
    request.messageId,
    request.actionId
  );
}

function routeClientActionRequestKey(request: ContextualAIChatClientActionRequest, href: string) {
  return `${request.conversationId}:${request.messageId}:route_navigation:${href}`;
}

function pruneClientActionDedupe(cache: Map<string, number>, now: number) {
  for (const [key, expiresAt] of cache.entries()) {
    if (expiresAt <= now) {
      cache.delete(key);
    }
  }
}

function markClientActionDedupe(
  cache: Map<string, number>,
  key: string,
  ttlMs = CLIENT_ACTION_DEDUPE_TTL_MS
) {
  cache.set(key, Date.now() + ttlMs);
}

function hasClientActionDedupe(cache: Map<string, number>, key: string, now: number) {
  const expiresAt = cache.get(key);
  return Boolean(expiresAt && expiresAt > now);
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

function clampDesktopPanelWidth(width: number) {
  if (typeof window === 'undefined') return Math.max(MIN_DESKTOP_PANEL_WIDTH, width);
  const viewportWidth = window.innerWidth;
  const viewportMax = Math.max(MIN_DESKTOP_PANEL_WIDTH, viewportWidth - MIN_DESKTOP_CONTENT_WIDTH);
  const ratioMax = Math.max(
    MIN_DESKTOP_PANEL_WIDTH,
    Math.round(viewportWidth * MAX_DESKTOP_PANEL_WIDTH_RATIO)
  );
  const maxWidth = Math.min(viewportMax, ratioMax);
  return Math.min(Math.max(Math.round(width), MIN_DESKTOP_PANEL_WIDTH), maxWidth);
}

function agentIdFromAgentDetailPath(pathname?: string | null) {
  const href = normalizeZGIConsoleNavigationHref(pathname ?? undefined);
  if (!href) return null;
  const match = href.match(/^\/console\/agents\/([^/]+)\/agent(?:\/.*)?$/);
  return match?.[1] ?? null;
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

function normalizeHintToken(value: string | undefined) {
  return (
    value
      ?.trim()
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '_')
      .replace(/^_+|_+$/g, '') || ''
  );
}

function getRefreshHintResolution(
  items: AIChatContextItem[],
  operation: ContextualAIChatAssetOperation
): { handledByAdapter: boolean; refreshHints: AIChatContextRefreshHint[] } {
  const assetType = normalizeHintToken(operation.assetType);
  const effect = normalizeHintToken(operation.effect);
  const assetId = operation.assetId?.trim();
  const seen = new Set<string>();
  const refreshHints: AIChatContextRefreshHint[] = [];
  let handledByAdapter = false;

  items.forEach(item => {
    item.hints?.handledAssetTypes?.forEach(handledAssetType => {
      if (normalizeHintToken(handledAssetType) === assetType) {
        handledByAdapter = true;
      }
    });

    item.hints?.refreshHints?.forEach(hint => {
      if (normalizeHintToken(hint.assetType) !== assetType) return;
      handledByAdapter = true;
      if (hint.effect && normalizeHintToken(hint.effect) !== effect) return;
      if (hint.resourceId && hint.resourceId !== assetId) return;
      if (!hint.queryKey || hint.queryKey.length === 0) return;

      const key = JSON.stringify(hint.queryKey);
      if (seen.has(key)) return;
      seen.add(key);
      refreshHints.push(hint);
    });
  });

  return { handledByAdapter, refreshHints };
}

function pickPresentationHint(
  items: AIChatContextItem[]
): AIChatContextPresentationHint | undefined {
  return items.find(item => item.hints?.presentation)?.hints?.presentation;
}

function hasToolGovernanceHint(items: AIChatContextItem[]) {
  return items.some(item => item.hints?.toolGovernance?.enabled === true);
}

function getContextTypeLabel(type: AIChatContextItem['type'], t: ContextualDockTranslator) {
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

function hasPageAdapterSignal(item: AIChatContextItem) {
  if (item.type !== 'page') return false;
  const metadataPage = metadataText(item, 'page');
  return Boolean(
    metadataPage ||
      item.capabilities?.length ||
      item.hints?.handledAssetTypes?.length ||
      item.hints?.presentation ||
      item.hints?.toolGovernance
  );
}

function isGenericRoutePageItem(item: AIChatContextItem) {
  if (item.type !== 'page') return false;
  if (hasPageAdapterSignal(item)) return false;

  const title = item.title.trim();
  return Boolean(
    title.startsWith('/') ||
      normalizeZGIConsoleNavigationHref(item.id) ||
      normalizeZGIConsoleNavigationHref(item.href) ||
      normalizeZGIConsoleNavigationHref(metadataText(item, 'route')) ||
      normalizeZGIConsoleNavigationHref(metadataText(item, 'pathname')) ||
      normalizeZGIConsoleNavigationHref(metadataText(item, 'path'))
  );
}

function pickPrimaryContextItem(items: AIChatContextItem[]) {
  const selectedItem = items.find(item => item.metadata?.selected === true);
  if (selectedItem && !isGenericRoutePageItem(selectedItem)) return selectedItem;

  const adaptedPageItem = items.find(item => item.type === 'page' && hasPageAdapterSignal(item));
  return (
    adaptedPageItem ??
    selectedItem ??
    items.find(item => !isGenericRoutePageItem(item)) ??
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
  const presentationSuggestions = contextItems.flatMap(
    item => item.hints?.presentation?.suggestions ?? []
  );
  if (presentationSuggestions.length > 0) {
    return Array.from(new Set(presentationSuggestions));
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

function metadataText(item: AIChatContextItem, key: string) {
  const value = item.metadata?.[key];
  return typeof value === 'string' ? value : undefined;
}

function metadataBoolean(item: AIChatContextItem, key: string) {
  const value = item.metadata?.[key];
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (['true', '1', 'yes', 'ready'].includes(normalized)) return true;
    if (['false', '0', 'no', 'loading', 'pending'].includes(normalized)) return false;
  }
  return undefined;
}

function isRouteContextItemReady(item: AIChatContextItem) {
  return metadataBoolean(item, 'context_ready') !== false;
}

function routeContextQueryReady(
  item: AIChatContextItem,
  statusKey: string,
  settledKey: string,
  countKeys: string[]
) {
  const queryStatus = metadataText(item, statusKey)?.trim().toLowerCase();
  if (queryStatus && ['error', 'unavailable', 'forbidden'].includes(queryStatus)) {
    return true;
  }

  if (!isRouteContextItemReady(item)) return false;

  if (queryStatus && queryStatus !== 'ready') return false;

  const querySettled = metadataBoolean(item, settledKey);
  if (querySettled === false) return false;

  if (queryStatus === 'ready' || querySettled === true) return true;

  return countKeys.some(key => metadataText(item, key) !== undefined);
}

function filesRouteContextItemReady(item: AIChatContextItem) {
  return routeContextQueryReady(item, 'files_query_status', 'files_query_settled', [
    'total_file_count',
    'visible_file_count',
    'indexed_visible_files',
  ]);
}

function agentsRouteContextItemReady(item: AIChatContextItem) {
  return routeContextQueryReady(item, 'agents_query_status', 'agents_query_settled', [
    'loaded_agent_count',
    'visible_agent_count',
    'visible_runtime_agent_count',
  ]);
}

function routeHrefFromContextItem(item: AIChatContextItem) {
  const candidates = [
    item.href,
    metadataText(item, 'href'),
    metadataText(item, 'route'),
    metadataText(item, 'pathname'),
    metadataText(item, 'path'),
    item.type === 'page' ? item.id : undefined,
    item.type === 'page' ? item.title : undefined,
  ];
  for (const candidate of candidates) {
    const href = normalizeZGIConsoleNavigationHref(candidate);
    if (href) return href;
  }
  return '';
}

function routeSpecificReadyContextItem(items: AIChatContextItem[], href: string) {
  if (href === '/console/files') {
    return items.find(item => {
      const metadataPage = metadataText(item, 'page');
      const metadataRoute = normalizeZGIConsoleNavigationHref(metadataText(item, 'route'));
      return (
        item.type === 'page' &&
        filesRouteContextItemReady(item) &&
        (item.id === 'console.files' ||
          metadataPage === 'console.files' ||
          (metadataRoute === href &&
            item.hints?.handledAssetTypes?.some(
              assetType => normalizeHintToken(assetType) === 'file'
            )))
      );
    });
  }

  if (href === '/console/agents') {
    return items.find(item => {
      const metadataPage = metadataText(item, 'page');
      const metadataRoute = normalizeZGIConsoleNavigationHref(metadataText(item, 'route'));
      return (
        item.type === 'page' &&
        agentsRouteContextItemReady(item) &&
        (item.id === 'console.agents' ||
          metadataPage === 'console.agents' ||
          (metadataRoute === href &&
            item.hints?.handledAssetTypes?.some(
              assetType => normalizeHintToken(assetType) === 'agent'
            )))
      );
    });
  }

  if (/^\/console\/agents\/[A-Za-z0-9_-]+\/agent$/.test(href)) {
    return items.find(
      item =>
        item.type === 'agent' &&
        isRouteContextItemReady(item) &&
        routeHrefFromContextItem(item) === href &&
        item.hints?.handledAssetTypes?.some(assetType => normalizeHintToken(assetType) === 'agent')
    );
  }

  return items.find(
    item =>
      isRouteContextItemReady(item) &&
      routeHrefFromContextItem(item) === href &&
      !isGenericRoutePageItem(item)
  );
}

function routeObservationContextItem(item: AIChatContextItem) {
  return {
    id: item.id,
    type: item.type,
    title: item.title,
    href: item.href,
    context_ready: metadataBoolean(item, 'context_ready'),
    files_query_status: metadataText(item, 'files_query_status'),
    agents_query_status: metadataText(item, 'agents_query_status'),
    visible_file_count: metadataText(item, 'visible_file_count'),
    total_file_count: metadataText(item, 'total_file_count'),
    visible_agent_count: metadataText(item, 'visible_agent_count'),
    loaded_agent_count: metadataText(item, 'loaded_agent_count'),
  };
}

function routeContextObservation(items: AIChatContextItem[], href: string) {
  const matchedItem = items.find(item => routeHrefFromContextItem(item) === href);
  const readyItem = routeRequiresPageContextReady(href)
    ? routeSpecificReadyContextItem(items, href)
    : matchedItem;
  return {
    page_context_ready: Boolean(readyItem),
    matched_context_item_id: matchedItem ? `${matchedItem.type}:${matchedItem.id}` : undefined,
    matched_context_title: matchedItem?.title,
    ready_context_item_id: readyItem ? `${readyItem.type}:${readyItem.id}` : undefined,
    ready_context_title: readyItem?.title,
    context_item_count: items.length,
    context_items: items.slice(0, 6).map(routeObservationContextItem),
  };
}

function routeRequiresPageContextReady(href: string) {
  return (
    href === '/console/files' ||
    href === '/console/agents' ||
    /^\/console\/agents\/[A-Za-z0-9_-]+\/agent$/.test(href)
  );
}

function recordValue(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  return value as Record<string, unknown>;
}

function recordListValue(value: unknown): Array<Record<string, unknown>> {
  if (!Array.isArray(value)) return [];
  return value.flatMap(item => {
    const record = recordValue(item);
    return record ? [record] : [];
  });
}

function waitingClientActionStatus(value: unknown) {
  if (typeof value !== 'string') return false;
  const status = value.trim().toLowerCase();
  return status === 'waiting_client_action' || status === 'waiting';
}

function pendingClientActionRecordFromMessage(
  message: AIChatMessage | null | undefined
): Record<string, unknown> | null {
  if (!message || message.status !== 'waiting_client_action') return null;

  const metadata = message.metadata;
  const continuation = recordValue(metadata?.client_action_continuation);
  const continuationActionId = textFromRecord(continuation, ['action_id']);
  const records = recordListValue(metadata?.client_actions);
  const invocations = recordListValue(metadata?.skill_invocations).filter(
    invocation => textFromRecord(invocation, ['kind']) === 'client_action'
  );

  const matchActionId = (record: Record<string, unknown>) =>
    continuationActionId && textFromRecord(record, ['action_id']) === continuationActionId;
  const waitingRecord = (record: Record<string, unknown>) =>
    waitingClientActionStatus(record.status);

  const matchedEvent = records.find(record => matchActionId(record) && waitingRecord(record));
  const matchedInvocation = invocations.find(
    record => matchActionId(record) && waitingRecord(record)
  );
  if (continuation && waitingRecord(continuation)) {
    return {
      ...continuation,
      ...(matchedInvocation ?? {}),
      ...(matchedEvent ?? {}),
    };
  }

  return records.find(waitingRecord) ?? invocations.find(waitingRecord) ?? null;
}

function clientActionRequestFromWaitingMessage(
  message: AIChatMessage | null | undefined
): ContextualAIChatClientActionRequest | null {
  const record = pendingClientActionRecordFromMessage(message);
  if (!message || !record) return null;

  const actionId = textFromRecord(record, ['action_id']);
  const actionType = textFromRecord(record, ['action_type']);
  if (!actionId || !actionType) return null;

  const href =
    actionType === 'route_navigation'
      ? normalizeZGIConsoleNavigationHref(textFromRecord(record, ['href']))
      : textFromRecord(record, ['href']);

  const payload: AIChatClientActionRequiredEventData = {
    conversation_id: message.conversation_id,
    message_id: message.id,
    action_id: actionId,
    action_type: actionType,
    status: 'waiting_client_action',
    skill_id: textFromRecord(record, ['skill_id']),
    tool_name: textFromRecord(record, ['tool_name']),
    href: href || undefined,
    label: textFromRecord(record, ['label']) || undefined,
    reason: textFromRecord(record, ['reason']) || undefined,
    effect: textFromRecord(record, ['effect']) || undefined,
    asset_type: textFromRecord(record, ['asset_type']) || undefined,
    assets: recordListValue(record.assets) as AIChatClientActionRequiredEventData['assets'],
  };

  return {
    actionId,
    actionType,
    conversationId: message.conversation_id,
    messageId: message.id,
    href: href || undefined,
    label: payload.label,
    reason: payload.reason,
    payload,
  };
}

function textFromRecord(record: Record<string, unknown> | null | undefined, keys: string[]) {
  if (!record) return '';
  for (const key of keys) {
    const value = record[key];
    if (typeof value === 'string' && value.trim()) return value.trim();
    if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  }
  return '';
}

function textFromContextMetadata(item: AIChatContextItem, keys: string[]) {
  for (const key of keys) {
    const value = item.metadata?.[key];
    if (typeof value === 'string' && value.trim()) return value.trim();
    if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  }
  return '';
}

function normalizeObservationToken(value: string | undefined) {
  return (
    value
      ?.trim()
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '_')
      .replace(/^_+|_+$/g, '') || ''
  );
}

function assetIdentityCandidates(asset: Record<string, unknown>) {
  return [
    textFromRecord(asset, ['id', 'file_id', 'asset_id', 'resource_id']),
    textFromRecord(asset, ['name', 'filename', 'file_name', 'title', 'label']),
  ].filter(Boolean);
}

function contextItemIdentityCandidates(item: AIChatContextItem) {
  return [
    item.id,
    item.title,
    textFromContextMetadata(item, ['id', 'file_id', 'asset_id', 'resource_id']),
    textFromContextMetadata(item, ['name', 'filename', 'file_name', 'title', 'label']),
  ].filter(Boolean);
}

function contextItemMatchesAsset(
  item: AIChatContextItem,
  asset: Record<string, unknown>,
  assetType: string
) {
  const normalizedAssetType = normalizeObservationToken(assetType);
  if (
    normalizedAssetType &&
    normalizeObservationToken(item.type) !== normalizedAssetType &&
    normalizeObservationToken(textFromContextMetadata(item, ['resource_kind', 'kind', 'type'])) !==
      normalizedAssetType
  ) {
    return false;
  }

  const assetCandidates = assetIdentityCandidates(asset);
  if (assetCandidates.length === 0) return false;
  const itemCandidates = contextItemIdentityCandidates(item);
  return assetCandidates.some(assetCandidate =>
    itemCandidates.some(itemCandidate => itemCandidate === assetCandidate)
  );
}

function assetObservationFromContextItems(
  items: AIChatContextItem[],
  pending: PendingClientActionContinuation
) {
  const assetType = pending.assetType || 'asset';
  const assets = pending.assets ?? [];
  const observed = assets.map(asset => {
    const match = items.find(item => contextItemMatchesAsset(item, asset, assetType));
    return {
      id: textFromRecord(asset, ['id', 'file_id', 'asset_id', 'resource_id']),
      name: textFromRecord(asset, ['name', 'filename', 'file_name', 'title', 'label']),
      type: textFromRecord(asset, ['type', 'asset_type']) || assetType,
      visible: Boolean(match),
      matched_context_item_id: match ? `${match.type}:${match.id}` : undefined,
      matched_context_title: match?.title,
    };
  });
  return {
    event_type: 'asset_observed',
    action_type: pending.actionType,
    effect: pending.effect,
    asset_type: assetType,
    asset_count: assets.length,
    visible_count: observed.filter(item => item.visible).length,
    context_item_count: items.length,
    observation_available: assets.length > 0,
    observed_assets: observed,
  };
}

function assetOperationFromClientAction(
  request: ContextualAIChatClientActionRequest
): ContextualAIChatAssetOperation | null {
  const payload = request.payload;
  const assetType = normalizeObservationToken(payload.asset_type);
  const effect = normalizeObservationToken(
    typeof payload.effect === 'string' ? payload.effect : undefined
  ) as ContextualAIChatAssetOperation['effect'];
  if (!assetType || !effect) return null;
  const assets = recordListValue(payload.assets);
  const firstAsset = assets[0];
  return {
    assetType,
    effect,
    source: 'skill_call',
    skillId: typeof payload.skill_id === 'string' ? payload.skill_id : '',
    toolName: typeof payload.tool_name === 'string' ? payload.tool_name : '',
    toolId: typeof payload.tool_id === 'string' ? payload.tool_id : undefined,
    assetId: textFromRecord(firstAsset, ['id', 'file_id', 'asset_id', 'resource_id']) || undefined,
    assetName:
      textFromRecord(firstAsset, ['name', 'filename', 'file_name', 'title', 'label']) || undefined,
    payload: payload as ContextualAIChatAssetOperation['payload'],
  };
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
          className="min-w-0 max-w-full basis-0 flex-1 shrink overflow-hidden rounded-full border border-border/70 bg-muted/30 px-3 text-left font-normal text-foreground hover:bg-muted/60"
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
              <div key={`${item.type}:${item.id}`} className="min-w-0 rounded-md px-2 py-2 text-sm">
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
  const hasContext = items.length > 0;
  const presentation = pickPresentationHint(items);
  const homeTitle =
    presentation?.homeTitle ??
    (hasContext
      ? t('consoleChat.contextual.home.contextTitle')
      : t('consoleChat.contextual.home.emptyTitle'));
  const homeDescription =
    presentation?.homeDescription ??
    (hasContext
      ? t('consoleChat.contextual.home.contextDescription')
      : t('consoleChat.contextual.home.emptyDescription'));
  const inputPlaceholder =
    presentation?.inputPlaceholder ?? t('consoleChat.contextual.input.placeholder');

  return (
    <div className="relative flex min-h-0 min-w-0 max-w-full flex-1 flex-col overflow-hidden">
      <div className="flex min-h-14 min-w-0 shrink-0 items-center gap-2 overflow-hidden border-b border-border/70 bg-background/95 px-3 py-2">
        <ContextSummaryMenu items={items} t={t} />
        <div className="ml-auto flex shrink-0 items-center gap-1">
          <div id={controlsPortalId} className="flex shrink-0 items-center" />
          <Button
            type="button"
            variant="ghost"
            isIcon
            className={embeddedControlButtonClassName}
            onClick={onClose}
            title={t('consoleChat.contextual.close')}
          >
            <X className="size-3.5" />
            <span className="sr-only">{t('consoleChat.contextual.close')}</span>
          </Button>
        </div>
      </div>
      <div className="min-h-0 flex-1">
        <Chat
          mode="aichat"
          controller={controller}
          modelSelectorValue={modelSelectorValue}
          isModelInitializing={isModelInitializing}
          onModelChange={onModelChange}
          variant="embedded"
          runtimeSurface="contextual_sidebar"
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
  const router = useRouter();
  const pathname = usePathname();
  const queryClient = useQueryClient();
  const { isOpen, setOpen, items } = useContextualAIChat();
  const isDesktopPanelViewport = useIsDesktopPanelViewport();
  const [desktopPanelWidth, setDesktopPanelWidth] = useState<number | null>(null);
  const itemsRef = useRef(items);
  const assetOperationRefreshRef = useRef<Map<string, number>>(new Map());
  const pendingClientActionsRef = useRef<Map<string, PendingClientActionContinuation>>(new Map());
  const processedClientActionsRef = useRef<Map<string, number>>(new Map());
  const clientActionContinuationRef = useRef<
    ReturnType<typeof useAIChatController>['continueClientAction'] | null
  >(null);
  const [pendingClientActionVersion, setPendingClientActionVersion] = useState(0);

  useEffect(() => {
    itemsRef.current = items;
  }, [items]);

  const clearClientActionTimeout = useCallback((pending: PendingClientActionContinuation) => {
    if (typeof window === 'undefined') return;
    if (pending.timeoutId === undefined) return;
    window.clearTimeout(pending.timeoutId);
    pending.timeoutId = undefined;
  }, []);

  const clearAllClientActionTimeouts = useCallback(() => {
    pendingClientActionsRef.current.forEach(pending => clearClientActionTimeout(pending));
  }, [clearClientActionTimeout]);

  const completePendingClientAction = useCallback(
    (
      pending: PendingClientActionContinuation,
      payload: AIChatClientActionResultRequest
    ) => {
      const currentPending = pendingClientActionsRef.current.get(pending.key);
      if (currentPending !== pending || pending.completed) {
        return;
      }

      if (pending.resuming) return;
      pending.resuming = true;
      setPendingClientActionVersion(version => version + 1);

      const continueClientAction = clientActionContinuationRef.current;
      if (!continueClientAction) {
        pending.resuming = false;
        return;
      }

      void continueClientAction(
        pending.conversationId,
        pending.messageId,
        pending.actionId,
        payload
      )
        .then(() => {
          if (pendingClientActionsRef.current.get(pending.key) !== pending) return;
          markClientActionDedupe(processedClientActionsRef.current, pending.key);
          pending.completed = true;
          pendingClientActionsRef.current.delete(pending.key);
          clearClientActionTimeout(pending);
          setPendingClientActionVersion(version => version + 1);
        })
        .catch(error => {
          markClientActionDedupe(
            processedClientActionsRef.current,
            pending.key,
            CLIENT_ACTION_FAILURE_DEDUPE_TTL_MS
          );
          pending.completed = true;
          pendingClientActionsRef.current.delete(pending.key);
          clearClientActionTimeout(pending);
          setPendingClientActionVersion(version => version + 1);
          console.error('AIChat client action continuation failed', error);
        });
    },
    [clearClientActionTimeout]
  );

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

      if (operation.assetType === 'agent' && operation.effect === 'delete') {
        const deletedAgentId = operation.assetId?.trim();
        if (deletedAgentId && agentIdFromAgentDetailPath(pathname) === deletedAgentId) {
          router.replace('/console/agents');
        }
        if (deletedAgentId) {
          void queryClient.cancelQueries({ queryKey: AGENT_KEYS.detail(deletedAgentId) });
          queryClient.removeQueries({ queryKey: AGENT_KEYS.detail(deletedAgentId) });
        }
        invalidateQueries(AGENT_KEYS.lists());
        invalidateQueries([...AGENT_KEYS.all, 'runnable-webapps']);
        return;
      }

      const { handledByAdapter, refreshHints } = getRefreshHintResolution(
        itemsRef.current,
        operation
      );
      if (refreshHints.length > 0) {
        refreshHints.forEach(hint => {
          if (hint.queryKey) {
            invalidateQueries(hint.queryKey);
          }
        });
        return;
      }
      if (handledByAdapter) return;

      switch (operation.assetType) {
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
    [invalidateQueries, pathname, queryClient, router]
  );

  const handleClientActionRequired = useCallback(
    (request: ContextualAIChatClientActionRequest) => {
      const now = Date.now();
      const key = clientActionRequestKey(request);
      pruneClientActionDedupe(processedClientActionsRef.current, now);
      if (pendingClientActionsRef.current.has(key)) return;
      if (hasClientActionDedupe(processedClientActionsRef.current, key, now)) return;

      if (request.actionType === 'asset_observation') {
        const operation = assetOperationFromClientAction(request);
        if (operation) {
          handleAssetOperationSuccess(operation);
        }

        const pending: PendingClientActionContinuation = {
          key,
          conversationId: request.conversationId,
          messageId: request.messageId,
          actionId: request.actionId,
          actionType: request.actionType,
          effect: typeof request.payload.effect === 'string' ? request.payload.effect : undefined,
          assetType:
            typeof request.payload.asset_type === 'string'
              ? request.payload.asset_type
              : undefined,
          assets: recordListValue(request.payload.assets),
          requestedAt: Date.now(),
          completed: false,
        };
        pendingClientActionsRef.current.set(key, pending);
        setPendingClientActionVersion(version => version + 1);

        if (typeof window !== 'undefined') {
          pending.timeoutId = window.setTimeout(() => {
            completePendingClientAction(pending, {
              status: 'succeeded',
              result: {
                refresh_requested: Boolean(operation),
                elapsed_ms: Date.now() - pending.requestedAt,
                ...assetObservationFromContextItems(itemsRef.current, pending),
              },
            });
          }, CLIENT_ACTION_OBSERVATION_SETTLE_MS);
        }
        return;
      }

      if (request.actionType !== 'route_navigation') return;
      const href = normalizeZGIConsoleNavigationHref(request.href);
      if (!href) return;
      const routeKey = routeClientActionRequestKey(request, href);
      if (pendingClientActionsRef.current.has(routeKey)) return;
      const currentHref = normalizeZGIConsoleNavigationHref(pathname ?? undefined);
      if (
        hasClientActionDedupe(processedClientActionsRef.current, routeKey, now) &&
        currentHref === href
      ) {
        return;
      }

      const pending: PendingClientActionContinuation = {
        key: routeKey,
        conversationId: request.conversationId,
        messageId: request.messageId,
        actionId: request.actionId,
        actionType: request.actionType,
        href,
        label: request.label,
        reason: request.reason,
        requestedAt: Date.now(),
        completed: false,
      };
      pendingClientActionsRef.current.set(routeKey, pending);
      setPendingClientActionVersion(version => version + 1);

      if (typeof window !== 'undefined') {
        pending.timeoutId = window.setTimeout(() => {
          completePendingClientAction(pending, {
            status: 'failed',
            error: `Route navigation to ${href} timed out.`,
            result: {
              event_type: 'route_load_timeout',
              action_type: pending.actionType,
              href,
              observed_path: normalizeZGIConsoleNavigationHref(pathname ?? undefined),
              elapsed_ms: Date.now() - pending.requestedAt,
              ...routeContextObservation(itemsRef.current, href),
            },
          });
        }, CLIENT_ACTION_ROUTE_TIMEOUT_MS);
      }

      if (currentHref === href) {
        const observation = routeContextObservation(itemsRef.current, href);
        if (!routeRequiresPageContextReady(href) || observation.page_context_ready) {
          completePendingClientAction(pending, {
            status: 'succeeded',
            result: {
              event_type: 'route_already_loaded',
              action_type: pending.actionType,
              href,
              label: pending.label,
              reason: pending.reason,
              observed_path: currentHref,
              elapsed_ms: Date.now() - pending.requestedAt,
              ...observation,
            },
          });
        }
        return;
      }

      try {
        router.push(href);
      } catch (error) {
        completePendingClientAction(pending, {
          status: 'failed',
          error: error instanceof Error ? error.message : `Route navigation to ${href} failed.`,
          result: {
            event_type: 'route_load_failed',
            action_type: pending.actionType,
            href,
            observed_path: currentHref,
          },
        });
      }
    },
    [
      completePendingClientAction,
      handleAssetOperationSuccess,
      pathname,
      router,
    ]
  );

  const transport = useMemo(
    () =>
      createContextualAIChatTransport(() => itemsRef.current, {
        onAssetOperationSuccess: handleAssetOperationSuccess,
        onClientActionRequired: handleClientActionRequired,
      }),
    [handleAssetOperationSuccess, handleClientActionRequired]
  );
  const controller = useAIChatController({ transport });
  const { init: initController } = controller;
  const waitingClientActionRequest = useMemo(() => {
    const activeConversation = controller.activeConversation;
    if (!activeConversation) return null;

    const leafMessageId =
      activeConversation.current_leaf_message_id ?? activeConversation.active_message_id;
    if (!leafMessageId) return null;

    const leafMessage = controller.messages.find(message => message.id === leafMessageId);
    return clientActionRequestFromWaitingMessage(leafMessage);
  }, [controller.activeConversation, controller.messages]);

  useEffect(() => {
    if (!waitingClientActionRequest) return;
    handleClientActionRequired(waitingClientActionRequest);
  }, [handleClientActionRequired, waitingClientActionRequest]);

  useEffect(() => {
    clientActionContinuationRef.current = controller.continueClientAction ?? null;
    const hasContinuablePending = Array.from(pendingClientActionsRef.current.values()).some(
      pending => !pending.completed && !pending.resuming
    );
    if (hasContinuablePending && controller.continueClientAction) {
      setPendingClientActionVersion(version => version + 1);
    }
  }, [controller.continueClientAction]);

  useEffect(() => {
    return () => clearAllClientActionTimeouts();
  }, [clearAllClientActionTimeouts]);

  useEffect(() => {
    const currentHref = normalizeZGIConsoleNavigationHref(pathname ?? undefined);
    const routePendings = Array.from(pendingClientActionsRef.current.values()).filter(
      pending =>
        !pending.completed &&
        pending.actionType === 'route_navigation' &&
        Boolean(pending.href) &&
        pending.href === currentHref
    );
    if (routePendings.length === 0) return;

    const pendingTimers: number[] = [];
    let needsPoll = false;

    routePendings.forEach(pending => {
      const href = pending.href;
      if (!href) return;

      const observation = routeContextObservation(itemsRef.current, href);
      if (routeRequiresPageContextReady(href) && !observation.page_context_ready) {
        needsPoll = true;
        return;
      }

      const delay = observation.page_context_ready
        ? CLIENT_ACTION_ROUTE_CONTEXT_SETTLE_MS
        : CLIENT_ACTION_ROUTE_FALLBACK_SETTLE_MS;

      const timer = window.setTimeout(() => {
        const latestObservation = routeContextObservation(itemsRef.current, href);
        completePendingClientAction(pending, {
          status: 'succeeded',
          result: {
            event_type: 'route_loaded',
            action_type: pending.actionType,
            href,
            label: pending.label,
            reason: pending.reason,
            observed_path: currentHref,
            elapsed_ms: Date.now() - pending.requestedAt,
            ...latestObservation,
          },
        });
      }, delay);
      pendingTimers.push(timer);
    });

    if (needsPoll) {
      const pollTimer = window.setTimeout(() => {
        setPendingClientActionVersion(version => version + 1);
      }, CLIENT_ACTION_ROUTE_CONTEXT_POLL_MS);
      pendingTimers.push(pollTimer);
    }

    return () => pendingTimers.forEach(timer => window.clearTimeout(timer));
  }, [
    completePendingClientAction,
    items,
    pathname,
    pendingClientActionVersion,
  ]);

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

  const enableToolGovernance = useMemo(() => hasToolGovernanceHint(items), [items]);
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

  return isOpen ? (
    <aside
      role="dialog"
      aria-modal="false"
      aria-label={t('consoleChat.contextual.assistantLabel')}
      className="fixed inset-y-0 right-0 z-50 flex h-full min-h-0 w-[min(720px,100vw)] max-w-full flex-col overflow-hidden border-l border-border/70 bg-background shadow-lg"
    >
      {panel}
    </aside>
  ) : null;
}
