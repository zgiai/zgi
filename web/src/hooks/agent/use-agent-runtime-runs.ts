'use client';

import { useCallback, useEffect, useMemo } from 'react';
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { reloadInfiniteQuery } from '@/hooks/query-utils';
import { agentRuntimeLogsService } from '@/services/agent-runtime-logs.service';
import type {
  AgentRuntimeRunItem,
  AgentRuntimeRunsList,
  AgentRuntimeRunsQuery,
} from '@/services/types/agent-runtime-log';
import type { ApiResponseData } from '@/services/types/common';
import { normalizeToastDescription } from '@/utils/error-notifications';

export interface UseAgentRuntimeRunsParams {
  agentId: string | null;
  limit?: number;
  query?: Omit<AgentRuntimeRunsQuery, 'page' | 'limit'>;
}

export interface UseAgentRuntimeRunsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
}

export function useAgentRuntimeRunsInfinite(
  { agentId, limit = 50, query }: UseAgentRuntimeRunsParams,
  {
    enabled = true,
    staleTime = 30_000,
    gcTime = 10 * 60_000,
    refetchOnWindowFocus = false,
  }: UseAgentRuntimeRunsOptions = {}
) {
  const t = useT('agents');
  const queryClient = useQueryClient();
  const querySignature = JSON.stringify(query ?? {});
  const normalizedParams = useMemo(() => {
    const parsedQuery = JSON.parse(querySignature) as Omit<AgentRuntimeRunsQuery, 'page' | 'limit'>;
    return {
      limit,
      ...parsedQuery,
    };
  }, [limit, querySignature]);
  const queryKey = useMemo(
    () => AGENT_KEYS.runtimeRuns(agentId ?? 'none', normalizedParams),
    [agentId, normalizedParams]
  );

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading, isFetching, error } =
    useInfiniteQuery<ApiResponseData<AgentRuntimeRunsList>>({
      queryKey,
      queryFn: ({ pageParam = 1 }) =>
        agentRuntimeLogsService.getRuntimeRuns(agentId || '', {
          page: pageParam as number,
          ...normalizedParams,
        }),
      initialPageParam: 1,
      getNextPageParam: (lastPage, allPages) =>
        lastPage.data?.has_more ? allPages.length + 1 : undefined,
      enabled: Boolean(agentId) && enabled,
      staleTime,
      gcTime,
      refetchOnWindowFocus,
      retry: false,
    });

  useEffect(() => {
    if (!error) return;
    const title = t('workflow.errors.loadRunsFailed');
    toast.error(title, {
      description: normalizeToastDescription(title, (error as Error).message),
    });
  }, [error, t]);

  const pages = useMemo<AgentRuntimeRunItem[][]>(() => {
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
