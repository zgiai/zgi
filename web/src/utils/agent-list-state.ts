import type { ApiResponseData } from '@/services/types/common';
import type { AgentList } from '@/services/types/agent';

const AGENT_LIST_STATE_KEY = 'zgi:console:agents:list-state';
const AGENT_LIST_PAGES_KEY = 'zgi:console:agents:list-pages';
const AGENT_LIST_RESTORE_INTENT_KEY = 'zgi:console:agents:restore-intent';
const AGENT_LIST_DETAIL_ENTRY_KEY = 'zgi:console:agents:detail-entry';
const AGENT_LIST_STATE_MAX_AGE_MS = 30 * 60 * 1000;
const AGENT_LIST_MAX_SNAPSHOT_PAGES = 5;

export interface AgentListNavigationState {
  keyword: string;
  loadedPageCount: number;
  scrollTop: number;
  workspaceId?: string;
  pages?: Array<ApiResponseData<AgentList>>;
  updatedAt: number;
}

interface AgentListRestoreIntent {
  updatedAt: number;
}

interface AgentListDetailEntry {
  agentId: string;
  updatedAt: number;
}

interface AgentListPagesSnapshot {
  keyword: string;
  workspaceId?: string;
  pages: Array<ApiResponseData<AgentList>>;
  updatedAt: number;
}

interface WriteAgentListNavigationStateOptions {
  includePages?: boolean;
}

function getSessionStorage(): Storage | null {
  if (typeof window === 'undefined') return null;

  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

function normalizeState(value: unknown): AgentListNavigationState | null {
  if (!value || typeof value !== 'object') return null;

  const candidate = value as Partial<AgentListNavigationState>;
  const updatedAt = Number(candidate.updatedAt);
  if (!Number.isFinite(updatedAt) || Date.now() - updatedAt > AGENT_LIST_STATE_MAX_AGE_MS) {
    return null;
  }

  const loadedPageCount = Math.max(0, Number(candidate.loadedPageCount) || 0);
  const scrollTop = Math.max(0, Number(candidate.scrollTop) || 0);

  return {
    keyword: typeof candidate.keyword === 'string' ? candidate.keyword : '',
    loadedPageCount,
    scrollTop,
    workspaceId: typeof candidate.workspaceId === 'string' ? candidate.workspaceId : undefined,
    pages: readAgentListPagesSnapshot(candidate) ?? readLegacyPages(candidate),
    updatedAt,
  };
}

export function readAgentListNavigationState(): AgentListNavigationState | null {
  const storage = getSessionStorage();
  if (!storage) return null;

  try {
    const raw = storage.getItem(AGENT_LIST_STATE_KEY);
    if (!raw) return null;
    return normalizeState(JSON.parse(raw));
  } catch {
    return null;
  }
}

export function readAgentListInitialKeyword(): string {
  if (!hasAgentListRestoreIntent()) return '';
  return readAgentListNavigationState()?.keyword ?? '';
}

export function markAgentListRestoreIntent(): void {
  const storage = getSessionStorage();
  if (!storage) return;

  try {
    storage.setItem(AGENT_LIST_RESTORE_INTENT_KEY, JSON.stringify({ updatedAt: Date.now() }));
  } catch {
    // Ignore storage errors. Restoration is best-effort.
  }
}

export function markAgentListDetailEntry(agentId: string): void {
  const storage = getSessionStorage();
  if (!storage || !agentId) return;

  try {
    storage.setItem(
      AGENT_LIST_DETAIL_ENTRY_KEY,
      JSON.stringify({ agentId, updatedAt: Date.now() })
    );
  } catch {
    // Ignore storage errors. Restoration is best-effort.
  }
}

export function markAgentListRestoreIntentFromDetail(agentId: string): void {
  if (!getSessionStorage() || !hasMatchingAgentListDetailEntry(agentId)) return;

  markAgentListRestoreIntent();
}

export function consumeAgentListRestoreIntent(): boolean {
  const storage = getSessionStorage();
  if (!storage) return false;

  try {
    const raw = storage.getItem(AGENT_LIST_RESTORE_INTENT_KEY);
    storage.removeItem(AGENT_LIST_RESTORE_INTENT_KEY);
    return isFreshRestoreIntent(raw);
  } catch {
    return false;
  }
}

function hasAgentListRestoreIntent(): boolean {
  const storage = getSessionStorage();
  if (!storage) return false;

  try {
    return isFreshRestoreIntent(storage.getItem(AGENT_LIST_RESTORE_INTENT_KEY));
  } catch {
    return false;
  }
}

function isFreshRestoreIntent(raw: string | null): boolean {
  if (!raw) return false;

  try {
    const intent = JSON.parse(raw) as Partial<AgentListRestoreIntent>;
    const updatedAt = Number(intent.updatedAt);
    return Number.isFinite(updatedAt) && Date.now() - updatedAt <= AGENT_LIST_STATE_MAX_AGE_MS;
  } catch {
    return false;
  }
}

function hasMatchingAgentListDetailEntry(agentId: string): boolean {
  const storage = getSessionStorage();
  if (!agentId) return false;
  if (!storage) return false;

  try {
    const raw = storage.getItem(AGENT_LIST_DETAIL_ENTRY_KEY);
    if (!raw) return false;

    const entry = JSON.parse(raw) as Partial<AgentListDetailEntry>;
    const updatedAt = Number(entry.updatedAt);
    return (
      entry.agentId === agentId &&
      Number.isFinite(updatedAt) &&
      Date.now() - updatedAt <= AGENT_LIST_STATE_MAX_AGE_MS
    );
  } catch {
    return false;
  }
}

function readLegacyPages(
  candidate: Partial<AgentListNavigationState>
): Array<ApiResponseData<AgentList>> | undefined {
  return Array.isArray(candidate.pages) ? candidate.pages : undefined;
}

function readAgentListPagesSnapshot(
  state: Partial<AgentListNavigationState>
): Array<ApiResponseData<AgentList>> | undefined {
  const storage = getSessionStorage();
  if (!storage) return undefined;

  try {
    const raw = storage.getItem(AGENT_LIST_PAGES_KEY);
    if (!raw) return undefined;

    const snapshot = JSON.parse(raw) as Partial<AgentListPagesSnapshot>;
    const updatedAt = Number(snapshot.updatedAt);
    const stateKeyword = typeof state.keyword === 'string' ? state.keyword : '';
    const stateWorkspaceId = typeof state.workspaceId === 'string' ? state.workspaceId : undefined;
    if (
      !Number.isFinite(updatedAt) ||
      Date.now() - updatedAt > AGENT_LIST_STATE_MAX_AGE_MS ||
      snapshot.keyword !== stateKeyword ||
      snapshot.workspaceId !== stateWorkspaceId ||
      !Array.isArray(snapshot.pages)
    ) {
      return undefined;
    }

    return snapshot.pages;
  } catch {
    return undefined;
  }
}

export function writeAgentListNavigationState(
  state: AgentListNavigationState,
  options: WriteAgentListNavigationStateOptions = {}
): void {
  const storage = getSessionStorage();
  if (!storage) return;

  const updatedAt = Date.now();

  try {
    storage.setItem(
      AGENT_LIST_STATE_KEY,
      JSON.stringify({
        keyword: state.keyword,
        loadedPageCount: Math.max(0, state.loadedPageCount),
        scrollTop: Math.max(0, state.scrollTop),
        workspaceId: state.workspaceId,
        updatedAt,
      })
    );

    if (options.includePages && state.pages) {
      storage.setItem(
        AGENT_LIST_PAGES_KEY,
        JSON.stringify({
          keyword: state.keyword,
          workspaceId: state.workspaceId,
          pages: state.pages.slice(0, AGENT_LIST_MAX_SNAPSHOT_PAGES),
          updatedAt,
        })
      );
    }
  } catch {
    // Ignore quota and serialization errors. Navigation restoration is best-effort.
  }
}
