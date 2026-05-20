'use client';

import { useCallback, useMemo } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import type { AutomationTaskStatus } from '@/services/types/automation';
import { TASK_STATUS_FILTERS } from './registry';
import type { TaskPanelMode, TaskPanelTab, TaskRouteState, TaskStatusFilterKey } from './types';

const ALL_STATUSES: AutomationTaskStatus[] = ['draft', 'active', 'paused', 'completed', 'archived'];

function normalizePage(pageParam: string | null): number {
  const page = Number(pageParam);
  if (!Number.isFinite(page) || page < 1) {
    return 1;
  }
  return Math.floor(page);
}

function normalizeMode(modeParam: string | null): TaskPanelMode {
  if (modeParam === 'create' || modeParam === 'edit') {
    return modeParam;
  }
  return null;
}

function normalizeTab(tabParam: string | null): TaskPanelTab {
  return tabParam === 'runs' ? 'runs' : 'overview';
}

function resolveFilterKey(statusQuery: string): TaskStatusFilterKey {
  const match = TASK_STATUS_FILTERS.find(filter => filter.query === statusQuery);
  return match?.key ?? 'active';
}

function getSelectedStatuses(filterKey: TaskStatusFilterKey): AutomationTaskStatus[] {
  if (filterKey === 'all') {
    return ALL_STATUSES;
  }

  return [filterKey];
}

/**
 * @hook useTaskRouteState
 * @category Feature
 * @status Stable
 * @description Synchronizes automation workbench state with URL query parameters.
 * @usage Use inside the task workbench to keep selection, panel mode, and list filters shareable.
 */
export function useTaskRouteState() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const routeState = useMemo<TaskRouteState>(() => {
    const taskId = searchParams.get('taskId');
    const mode = normalizeMode(searchParams.get('mode'));
    const tab = normalizeTab(searchParams.get('tab'));
    const page = normalizePage(searchParams.get('page'));
    const rawStatusQuery = searchParams.get('status');
    const filterKey = rawStatusQuery === null ? 'active' : resolveFilterKey(rawStatusQuery);
    const statusQuery = filterKey === 'all' ? '' : filterKey;
    const selectedStatuses = getSelectedStatuses(filterKey);
    const panelView =
      mode === 'create' ? 'create' : mode === 'edit' ? 'edit' : taskId ? 'view' : null;

    return {
      taskId,
      mode,
      tab,
      page,
      statusQuery,
      selectedStatuses,
      filterKey,
      panelView,
      panelOpen: panelView !== null,
    };
  }, [searchParams]);

  const replaceParams = useCallback(
    (updates: Record<string, string | null>) => {
      const next = new URLSearchParams(searchParams.toString());

      Object.entries(updates).forEach(([key, value]) => {
        if (!value) {
          next.delete(key);
          return;
        }
        next.set(key, value);
      });

      const query = next.toString();
      router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams]
  );

  const openCreate = useCallback(() => {
    replaceParams({
      taskId: null,
      mode: 'create',
      tab: 'overview',
    });
  }, [replaceParams]);

  const selectTask = useCallback(
    (taskId: string, tab?: TaskPanelTab) => {
      replaceParams({
        taskId,
        mode: null,
        tab: tab ?? routeState.tab,
      });
    },
    [replaceParams, routeState.tab]
  );

  const openEdit = useCallback(
    (taskId: string) => {
      replaceParams({
        taskId,
        mode: 'edit',
        tab: 'overview',
      });
    },
    [replaceParams]
  );

  const closePanel = useCallback(() => {
    replaceParams({
      taskId: null,
      mode: null,
      tab: null,
    });
  }, [replaceParams]);

  const setTab = useCallback(
    (tab: TaskPanelTab) => {
      replaceParams({
        tab,
      });
    },
    [replaceParams]
  );

  const setPage = useCallback(
    (page: number) => {
      replaceParams({
        page: page > 1 ? String(page) : null,
      });
    },
    [replaceParams]
  );

  const setStatusFilter = useCallback(
    (statusQuery: string) => {
      replaceParams({
        status: statusQuery === 'active' ? null : statusQuery || null,
        page: null,
      });
    },
    [replaceParams]
  );

  return {
    ...routeState,
    openCreate,
    selectTask,
    openEdit,
    closePanel,
    setTab,
    setPage,
    setStatusFilter,
  };
}
