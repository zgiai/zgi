'use client';

// UI local storage helpers for persistent user preferences
// Strictly typed and client-only (localStorage)

export type InteractionMode = 'mouse' | 'trackpad';

const UI_LOCAL_KEYS = {
  workflowInteractionMode: 'ui:workflow:interactionMode' as const,
};

/**
 * Get last saved workflow interaction mode from localStorage.
 * Returns null when no valid value is stored.
 */
export function getSavedWorkflowInteractionMode(): InteractionMode | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = window.localStorage.getItem(UI_LOCAL_KEYS.workflowInteractionMode);
    if (raw === 'mouse' || raw === 'trackpad') return raw;
    if (raw === 'hand') return 'mouse';
    if (raw === 'pointer') return 'trackpad';
    return null;
  } catch {
    return null;
  }
}

/**
 * Persist workflow interaction mode into localStorage.
 */
export function saveWorkflowInteractionMode(mode: InteractionMode): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(UI_LOCAL_KEYS.workflowInteractionMode, mode);
  } catch {
    // Silently ignore storage failures
  }
}

// Local UI preferences utility
// Stores a single JSON object under `zgi_ui_local` to share UI flags across the app.
// English comments only, strict types, no `any` usage.

import { deepMerge } from '@/utils/object';
import type { ChannelLatencyMap } from '@/utils/channel-latency';

export const UI_LOCAL_STORAGE_KEY = 'zgi_ui_local' as const;

// Banner keys used across the app. Extend as needed in future.
export enum BannerKey {
  TableColumnsDeleteWarning = 'tableColumnsDeleteWarning',
  WorkflowRunErrorsWarning = 'workflowRunErrorsWarning',
}

// Banners visibility map: true means hidden (do not show).
export type UiLocalBanners = Partial<Record<BannerKey, boolean>>;

// AI model selection for DB operations (per account, per scope)
export interface AiModelSelection {
  provider: string;
  model: string;
}

// Scope types for AI model selection
export type AiModelScope =
  | 'create'
  | 'excelImport'
  | 'ingest'
  | 'biSearch'
  | 'workChat'
  | 'contextualSidebar'
  // Legacy shared scope. New chat surfaces must use a surface-specific scope.
  | 'consoleChat'
  | 'imageGenChat'
  | 'workflowTestScenario';

export interface UiLocal {
  banners: UiLocalBanners;
  workflowPrecheck?: {
    collapsedBySurface?: Record<string, boolean>;
  };
  // Account-scoped model selections: accountId -> scope -> model
  models?: {
    byAccount?: Record<string, Partial<Record<AiModelScope, AiModelSelection>>>;
  };
  sidebar?: {
    consoleCollapsed?: boolean;
    agentCollapsed?: boolean;
    dbCollapsed?: boolean;
    datasetCollapsed?: boolean;
    appCollapsed?: boolean;
    dashboardCollapsed?: boolean;
  };
  // Channel connectivity latency cache (per channel, per model)
  channelsLatency?: ChannelLatencyMap;
}

const DEFAULT_UI_LOCAL: UiLocal = {
  banners: {},
  models: { byAccount: {} },
};

function readFromStorage(): UiLocal {
  if (typeof window === 'undefined') return DEFAULT_UI_LOCAL;
  try {
    const raw = window.localStorage.getItem(UI_LOCAL_STORAGE_KEY);
    if (!raw) return DEFAULT_UI_LOCAL;
    const parsed = JSON.parse(raw) as unknown;
    // Basic runtime shape guard
    if (
      parsed &&
      typeof parsed === 'object' &&
      'banners' in (parsed as Record<string, unknown>) &&
      typeof (parsed as { banners?: unknown }).banners === 'object'
    ) {
      return deepMerge(DEFAULT_UI_LOCAL, parsed as UiLocal);
    }
    return DEFAULT_UI_LOCAL;
  } catch {
    return DEFAULT_UI_LOCAL;
  }
}

function writeToStorage(value: UiLocal): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(UI_LOCAL_STORAGE_KEY, JSON.stringify(value));
  } catch {
    // Silently ignore storage failures (Safari private mode, quota exceeded, etc.)
  }
}

export function getUiLocal(): UiLocal {
  return readFromStorage();
}

export function setUiLocal(next: UiLocal): void {
  writeToStorage(next);
}

export function updateUiLocal(patch: Partial<UiLocal>): UiLocal {
  const current = readFromStorage();
  const next = deepMerge(current, patch);
  writeToStorage(next);
  return next;
}

export function isBannerHidden(key: BannerKey): boolean {
  const ui = readFromStorage();
  return Boolean(ui.banners?.[key]);
}

export function hideBanner(key: BannerKey): UiLocal {
  return updateUiLocal({ banners: { [key]: true } });
}

export function showBanner(key: BannerKey): UiLocal {
  return updateUiLocal({ banners: { [key]: false } });
}

export function getWorkflowPrecheckNoticeCollapsed(surfaceKey: string, fallback = false): boolean {
  if (!surfaceKey) return fallback;
  const ui = readFromStorage();
  const value = ui.workflowPrecheck?.collapsedBySurface?.[surfaceKey];
  return typeof value === 'boolean' ? value : fallback;
}

export function setWorkflowPrecheckNoticeCollapsed(
  surfaceKey: string,
  collapsed: boolean
): UiLocal {
  const current = readFromStorage();
  const next: UiLocal = {
    ...current,
    workflowPrecheck: {
      collapsedBySurface: {
        ...(current.workflowPrecheck?.collapsedBySurface ?? {}),
        [surfaceKey]: collapsed,
      },
    },
  };
  writeToStorage(next);
  return next;
}

/* -------------------------------------------------------------------------- */
/* AI Model Selection Helpers (account-scoped)                               */
/* -------------------------------------------------------------------------- */

/**
 * Get the last selected AI model for a given account and scope.
 * Returns null if no selection exists.
 */
export function getLastSelectedAiModel(
  accountId: string,
  scope: AiModelScope
): AiModelSelection | null {
  const ui = readFromStorage();
  const selection = ui.models?.byAccount?.[accountId]?.[scope];
  return selection ?? null;
}

/**
 * Save the last selected AI model for a given account and scope.
 */
export function saveLastSelectedAiModel(
  accountId: string,
  scope: AiModelScope,
  value: AiModelSelection
): void {
  const current = readFromStorage();
  const updated: UiLocal = {
    ...current,
    models: {
      byAccount: {
        ...(current.models?.byAccount ?? {}),
        [accountId]: {
          ...(current.models?.byAccount?.[accountId] ?? {}),
          [scope]: value,
        },
      },
    },
  };
  writeToStorage(updated);
}

/**
 * Clear AI model selections for a specific account or all accounts.
 * If accountId is provided, only that account's selections are cleared.
 * Otherwise, all model selections are cleared while preserving other UI state.
 */
export function clearAiModelSelections(accountId?: string): void {
  const current = readFromStorage();
  if (accountId) {
    // Clear only the specified account
    const { [accountId]: _, ...rest } = current.models?.byAccount ?? {};
    const updated: UiLocal = {
      ...current,
      models: { byAccount: rest },
    };
    writeToStorage(updated);
  } else {
    // Clear all model selections
    const updated: UiLocal = {
      ...current,
      models: { byAccount: {} },
    };
    writeToStorage(updated);
  }
}

export type SidebarId = 'console' | 'agent' | 'db' | 'dataset' | 'app' | 'dashboard';

export function getSidebarCollapsed(id: SidebarId, fallback: boolean): boolean {
  const ui = readFromStorage();
  let v: boolean | undefined;
  switch (id) {
    case 'console':
      v = ui.sidebar?.consoleCollapsed;
      break;
    case 'agent':
      v = ui.sidebar?.agentCollapsed;
      break;
    case 'dataset':
      v = ui.sidebar?.datasetCollapsed;
      break;
    case 'app':
      v = ui.sidebar?.appCollapsed;
      break;
    case 'dashboard':
      v = ui.sidebar?.dashboardCollapsed;
      break;
    case 'db':
    default:
      v = ui.sidebar?.dbCollapsed;
      break;
  }
  return typeof v === 'boolean' ? v : fallback;
}

export function saveSidebarCollapsed(id: SidebarId, collapsed: boolean): void {
  const key =
    id === 'console'
      ? 'consoleCollapsed'
      : id === 'agent'
        ? 'agentCollapsed'
        : id === 'dataset'
          ? 'datasetCollapsed'
          : id === 'app'
            ? 'appCollapsed'
            : id === 'dashboard'
              ? 'dashboardCollapsed'
              : 'dbCollapsed';
  const current = readFromStorage();
  const next: UiLocal = {
    ...current,
    sidebar: {
      ...(current.sidebar ?? {}),
      [key]: collapsed,
    },
  };
  writeToStorage(next);
}
