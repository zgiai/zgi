'use client';

import { useMemo, useCallback, useEffect } from 'react';
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query';
import { workflowService } from '@/services/workflow.service';
import type {
  WorkflowRunList,
  WorkflowRunItem,
  WorkflowRunsQuery,
} from '@/services/types/workflow';
import type { ApiResponseData } from '@/services/types/common';
import { toast } from 'sonner';
import { useT } from '@/i18n';

import { WORKFLOW_KEYS } from '@/hooks/query-keys';
import { reloadInfiniteQuery } from '@/hooks/query-utils';
import { normalizeToastDescription } from '@/utils/error-notifications';

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseWorkflowRunsParams {
  agentId: string | null;
  limit?: number;
  query?: Omit<WorkflowRunsQuery, 'page' | 'limit'>;
}

export interface UseWorkflowRunsOptions {
  enabled?: boolean; // controlled by dropdown open state
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

export interface UseWorkflowRunsReturn {
  pages: WorkflowRunItem[][];
  fetchNextPage: () => Promise<unknown>;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  reload: () => Promise<void>;
}

/**
 * Hook for workflow runs list with infinite pagination
 * Lazy-enabled and optimized for dropdown usage.
 */
export function useWorkflowRunsInfinite(
  { agentId, limit = 10, query }: UseWorkflowRunsParams,
  {
    enabled = true,
    staleTime = 60 * 1000,
    gcTime = 10 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
  }: UseWorkflowRunsOptions = {}
): UseWorkflowRunsReturn {
  const t = useT('agents');
  const queryClient = useQueryClient();
  const querySignature = JSON.stringify(query ?? {});
  const normalizedParams = useMemo(() => {
    const parsedQuery = JSON.parse(querySignature) as Omit<WorkflowRunsQuery, 'page' | 'limit'>;
    return {
      limit,
      ...parsedQuery,
    };
  }, [limit, querySignature]);
  const isEnabled = Boolean(agentId) && enabled;

  const queryKey = useMemo(
    () => WORKFLOW_KEYS.runList(agentId ?? 'none', normalizedParams),
    [agentId, normalizedParams]
  );

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading, isFetching, error } =
    useInfiniteQuery<ApiResponseData<WorkflowRunList>>({
      queryKey,
      queryFn: ({ pageParam = 1 }) =>
        workflowService.getWorkflowRuns(agentId || '', {
          page: pageParam as number,
          ...normalizedParams,
        }),
      initialPageParam: 1,
      getNextPageParam: (lastPage, allPages) => {
        const currentPage = allPages.length;
        const hasMore = lastPage.data?.has_more ?? false;
        return hasMore ? currentPage + 1 : undefined;
      },
      enabled: isEnabled,
      staleTime,
      gcTime,
      refetchOnWindowFocus,
      refetchInterval,
      retry: false,
    });

  // Toast error feedback (avoid in component) - fire only when error changes
  useEffect(() => {
    if (error) {
      const title = t('workflow.errors.loadRunsFailed');
      toast.error(title, {
        description: normalizeToastDescription(title, (error as Error)?.message),
      });
    }
  }, [error, t]);

  const pages = useMemo(() => {
    if (!data?.pages) return [];
    return data.pages.map(page => page.data?.data ?? []);
  }, [data?.pages]);

  const reload = useCallback(async () => {
    if (!agentId) return;
    await reloadInfiniteQuery(queryClient, queryKey);
  }, [agentId, queryClient, queryKey]);

  return {
    pages,
    fetchNextPage,
    hasNextPage: hasNextPage ?? false,
    isFetchingNextPage,
    isLoading,
    isFetching,
    error: (error as Error | null)?.message ?? null,
    reload,
  };
}
