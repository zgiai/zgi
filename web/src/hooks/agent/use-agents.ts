'use client';

import { useCallback, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient, useInfiniteQuery } from '@tanstack/react-query';
import { agentService } from '@/services';
import { toast } from 'sonner';
import type {
  AgentAssetKind,
  AgentList,
  CreateAgentRequest,
  UpdateAgentRequest,
  UpdateWebAppStatusRequest,
  UpdateWebAppStatusResponse,
  Agent,
  AgentDetail,
  WebAppStatus,
  AgentRuntimeConfig,
  UpdateAgentRuntimeConfigRequest,
  PublishAgentResponse,
} from '@/services/types/agent';
import type { ApiResponseData } from '@/services/types/common';
import { getErrorMessage } from '@/utils/error-notifications';
import { useT } from '@/i18n';
import { useCurrentWorkspace } from '@/store/workspace-store';

import { AGENT_KEYS } from '@/hooks/query-keys';
import {
  workspaceInvalidatePredicate,
  reloadInfiniteQuery,
  infiniteQueryUtils,
} from '@/hooks/query-utils';

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseAgentsParams {
  limit?: number;
  keyword?: string;
  workspace_id?: string;
  asset_kind?: AgentAssetKind;
}

export interface UseAgentsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

export interface UseAgentsReturn {
  pages: Agent[][]; // paged data structure [[...], [...]]
  fetchNextPage: () => Promise<unknown>;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  reload: () => Promise<void>;
  refetchFromPage: (startIndex: number) => Promise<unknown>;
  refetchFromPageAndAfter: (startIndex: number) => Promise<unknown>;
}

/**
 * Hook for managing agent list with infinite scroll
 */
export function useAgents(
  params: UseAgentsParams = {},
  {
    enabled = true,
    staleTime = 5 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
  }: UseAgentsOptions = {}
): UseAgentsReturn {
  const queryClient = useQueryClient();

  // Normalize params so that queryKey is stable and does not include page
  const normalizedParams = useMemo(
    () => ({
      limit: params.limit,
      keyword: params.keyword,
      workspace_id: params.workspace_id,
      asset_kind: params.asset_kind,
    }),
    [params.limit, params.keyword, params.workspace_id, params.asset_kind]
  );

  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useInfiniteQuery({
    queryKey: AGENT_KEYS.list(normalizedParams),
    queryFn: ({ pageParam = 1 }) =>
      agentService.getAgents({ ...normalizedParams, page: pageParam }),
    getNextPageParam: (lastPage, allPages) => {
      const currentPage = allPages.length;
      const hasMore = lastPage.data?.has_more ?? false;
      return hasMore ? currentPage + 1 : undefined;
    },
    initialPageParam: 1,
    enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
  });

  // Transform pages into flat array structure for easier consumption
  const pages = useMemo(() => {
    if (!data?.pages) return [];
    return data.pages.map(page => page.data?.data ?? []);
  }, [data?.pages]);

  // Reload from the beginning
  const reload = useCallback(async () => {
    await reloadInfiniteQuery(queryClient, AGENT_KEYS.list(normalizedParams));
  }, [normalizedParams, queryClient]);

  // Refetch from a specific page index
  const refetchFromPage = useCallback(
    async (startIndex: number) => {
      await infiniteQueryUtils.refetchFromPage(
        queryClient,
        AGENT_KEYS.list(normalizedParams),
        startIndex,
        fetchNextPage
      );
    },
    [normalizedParams, fetchNextPage, queryClient]
  );

  // Refetch from startIndex to the end of currently cached pages without touching earlier pages
  const refetchFromPageAndAfter = useCallback(
    async (startIndex: number) => {
      const key = AGENT_KEYS.list(normalizedParams);
      const cached = queryClient.getQueryData<{
        pages: Array<ApiResponseData<AgentList>>;
        pageParams: unknown[];
      }>(key);
      if (cached && Array.isArray(cached.pages)) {
        const start = Math.max(0, startIndex);
        for (let i = start; i < cached.pages.length; i++) {
          try {
            const fresh = await agentService.getAgents({ ...normalizedParams, page: i + 1 });
            queryClient.setQueryData(
              key,
              (
                oldData:
                  | { pages: Array<ApiResponseData<AgentList>>; pageParams: unknown[] }
                  | undefined
              ) => {
                if (!oldData) return oldData;
                const updatedPages = [...oldData.pages];
                if (i < updatedPages.length) {
                  updatedPages[i] = fresh;
                }
                return { ...oldData, pages: updatedPages };
              }
            );
          } catch {
            // Ignore per-page refresh errors
          }
        }
      } else {
        await refetch();
      }
    },
    [normalizedParams, queryClient, refetch]
  );

  return {
    pages,
    fetchNextPage,
    hasNextPage: hasNextPage ?? false,
    isFetchingNextPage,
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    reload,
    refetchFromPage,
    refetchFromPageAndAfter,
  };
}

/**
 * Hook for deleting agents with optimistic updates
 */
export function useDeleteAgent() {
  const queryClient = useQueryClient();
  const t = useT('agents');
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  return useMutation({
    mutationFn: (agentId: string) => agentService.deleteAgent(agentId),
    onMutate: async agentId => {
      // Cancel any outgoing refetches
      await queryClient.cancelQueries({ queryKey: AGENT_KEYS.lists() });

      // Snapshot the previous value
      const previousQueries = queryClient.getQueriesData({ queryKey: AGENT_KEYS.lists() });

      // Optimistically update to remove the agent
      queryClient.setQueriesData(
        { queryKey: AGENT_KEYS.lists() },
        (old: { pages: Array<ApiResponseData<AgentList>> } | undefined) => {
          if (!old?.pages) return old;
          return {
            ...old,
            pages: old.pages.map((page: ApiResponseData<AgentList>) => ({
              ...page,
              data: {
                ...page.data,
                data: page.data?.data?.filter((agent: Agent) => agent.id !== agentId) ?? [],
              },
            })),
          };
        }
      );

      return { previousQueries };
    },
    onError: (err, agentId, context) => {
      // If the mutation fails, use the context returned from onMutate to roll back
      if (context?.previousQueries) {
        context.previousQueries.forEach(([queryKey, queryData]) => {
          queryClient.setQueryData(queryKey, queryData);
        });
      }
      toast.error(getErrorMessage(err) || t('toasts.deleteFailed'));
    },
    onSuccess: () => {
      toast.success(t('toasts.deleteSuccess'));
    },
    onSettled: (_data, _error, agentId) => {
      // Always refetch after error or success
      queryClient.invalidateQueries({
        queryKey: AGENT_KEYS.all,
        predicate: workspaceInvalidatePredicate(AGENT_KEYS.all[0], currentWorkspaceId),
      });
      // Invalidate the CURRENT workspace's list specifically too
      queryClient.invalidateQueries({ queryKey: AGENT_KEYS.lists() });

      if (agentId) {
        queryClient.removeQueries({ queryKey: AGENT_KEYS.detail(agentId) });
      }
    },
  });
}

/**
 * Hook for creating agents
 */
export function useCreateAgent() {
  const queryClient = useQueryClient();
  const t = useT('agents');
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  return useMutation({
    mutationFn: (data: CreateAgentRequest) => agentService.createAgent(data),
    onSuccess: () => {
      // Invalidate both paginated and basic lists that are either unfiltered or belong to current workspace
      queryClient.invalidateQueries({
        queryKey: AGENT_KEYS.all,
        predicate: workspaceInvalidatePredicate(AGENT_KEYS.all[0], currentWorkspaceId),
      });
      toast.success(t('toasts.createSuccess'));
    },
    onError: (error: unknown) => {
      toast.error(getErrorMessage(error) || t('toasts.createFailed'));
    },
  });
}

/**
 * Hook for updating agents
 */
export function useUpdateAgent() {
  const queryClient = useQueryClient();
  const t = useT('agents');
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  return useMutation<
    ApiResponseData<Agent>,
    unknown,
    { agentId: string; data: UpdateAgentRequest },
    {
      previousLists: Array<[readonly unknown[], unknown]>;
      previousDetail?: ApiResponseData<AgentDetail> | undefined;
    }
  >({
    mutationFn: ({ agentId, data }) => agentService.updateAgent(agentId, data),
    onMutate: async ({ agentId, data }) => {
      await Promise.all([
        queryClient.cancelQueries({ queryKey: AGENT_KEYS.lists() }),
        queryClient.cancelQueries({ queryKey: AGENT_KEYS.detail(agentId) }),
      ]);

      const previousLists = queryClient.getQueriesData<{
        pages: Array<ApiResponseData<AgentList>>;
      }>({ queryKey: AGENT_KEYS.lists() });
      const previousDetail = queryClient.getQueryData<ApiResponseData<AgentDetail>>(
        AGENT_KEYS.detail(agentId)
      );

      queryClient.setQueriesData(
        { queryKey: AGENT_KEYS.lists() },
        (old: { pages: Array<ApiResponseData<AgentList>> } | undefined) => {
          if (!old?.pages) return old;
          return {
            ...old,
            pages: old.pages.map((page: ApiResponseData<AgentList>) => {
              const list = page.data?.data ?? [];
              const updated = list.map((item: Agent) =>
                item.id === agentId
                  ? {
                      ...item,
                      name: data.name ?? item.name,
                      description: data.description ?? item.description,
                      icon_type: data.icon_type ?? item.icon_type,
                      icon: data.icon ?? item.icon,
                    }
                  : item
              );
              return page.data ? { ...page, data: { ...page.data, data: updated } } : page;
            }),
          };
        }
      );

      if (previousDetail?.data) {
        const prev = previousDetail;
        queryClient.setQueryData(AGENT_KEYS.detail(agentId), {
          ...prev,
          data: {
            ...prev.data,
            name: data.name ?? prev.data.name,
            description: data.description ?? prev.data.description,
            icon_type: data.icon_type ?? prev.data.icon_type,
            icon: data.icon ?? prev.data.icon,
            workspace:
              data.workspace_id && prev.data.workspace
                ? { ...prev.data.workspace, id: data.workspace_id }
                : prev.data.workspace,
          },
        });
      }

      return { previousLists, previousDetail };
    },
    onError: (error, variables, context) => {
      if (context?.previousLists) {
        context.previousLists.forEach(([key, data]) => {
          queryClient.setQueryData(key, data);
        });
      }
      if (context?.previousDetail && variables?.agentId) {
        queryClient.setQueryData(AGENT_KEYS.detail(variables.agentId), context.previousDetail);
      }
      toast.error(getErrorMessage(error) || t('toasts.updateFailed'));
    },
    onSuccess: () => {
      toast.success(t('toasts.updateSuccess'));
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({
        queryKey: AGENT_KEYS.all,
        predicate: workspaceInvalidatePredicate(AGENT_KEYS.all[0], currentWorkspaceId),
      });
      // Always invalidate the active prefix too
      queryClient.invalidateQueries({ queryKey: AGENT_KEYS.lists() });

      if (variables?.agentId) {
        queryClient.invalidateQueries({
          queryKey: AGENT_KEYS.detail(variables.agentId),
        });
      }
    },
  });
}

export function useAgentConfig(agentId: string | null, enabled: boolean = true) {
  return useQuery<ApiResponseData<AgentRuntimeConfig>, Error>({
    queryKey: agentId ? AGENT_KEYS.config(agentId) : AGENT_KEYS.config(''),
    queryFn: () => agentService.getAgentConfig(agentId ?? ''),
    enabled: enabled && Boolean(agentId),
  });
}

export function useUpdateAgentConfig() {
  const queryClient = useQueryClient();
  const t = useT('agents');

  return useMutation<
    ApiResponseData<AgentRuntimeConfig>,
    unknown,
    { agentId: string; data: UpdateAgentRuntimeConfigRequest }
  >({
    mutationFn: ({ agentId, data }) => agentService.updateAgentConfig(agentId, data),
    onSuccess: (_data, { agentId }) => {
      queryClient.invalidateQueries({ queryKey: AGENT_KEYS.config(agentId) });
      queryClient.invalidateQueries({ queryKey: AGENT_KEYS.detail(agentId) });
      queryClient.invalidateQueries({ queryKey: AGENT_KEYS.candidates(agentId) });
      toast.success(t('toasts.saveSuccess'));
    },
    onError: (error: unknown) => {
      toast.error(getErrorMessage(error) || t('toasts.saveFailed'));
    },
  });
}

export function usePublishAgent() {
  const queryClient = useQueryClient();
  const t = useT('agents');

  return useMutation<
    ApiResponseData<PublishAgentResponse>,
    unknown,
    { agentId: string; silent?: boolean }
  >({
    mutationFn: ({ agentId }) => agentService.publishAgent(agentId),
    onSuccess: (data, { agentId, silent }) => {
      const published = data?.data;
      queryClient.setQueryData<ApiResponseData<AgentDetail>>(
        AGENT_KEYS.detail(agentId),
        previous =>
          previous?.data
            ? {
                ...previous,
                data: {
                  ...previous.data,
                  is_published: true,
                  web_app_id: published?.web_app_id || previous.data.web_app_id,
                  web_app_status: previous.data.web_app_status ?? 'active',
                },
              }
            : previous
      );
      queryClient.invalidateQueries({ queryKey: AGENT_KEYS.detail(agentId) });
      queryClient.invalidateQueries({ queryKey: AGENT_KEYS.lists() });
      queryClient.invalidateQueries({ queryKey: [...AGENT_KEYS.all, 'runnable-webapps'] });
      if (!silent) {
        toast.success(t('toasts.publishSuccess'));
      }
    },
    onError: (error: unknown, variables) => {
      if (!variables?.silent) {
        toast.error(getErrorMessage(error) || t('toasts.publishFailed'));
      }
    },
  });
}

/**
 * Hook for toggling a published web app between online and offline states.
 */
export function useUpdateWebAppStatus() {
  const queryClient = useQueryClient();
  const t = useT('agents');
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  return useMutation<
    ApiResponseData<UpdateWebAppStatusResponse>,
    unknown,
    { agentId: string; data: UpdateWebAppStatusRequest },
    {
      previousLists: Array<[readonly unknown[], unknown]>;
      previousDetail?: ApiResponseData<AgentDetail> | undefined;
    }
  >({
    mutationFn: ({ agentId, data }) => agentService.updateWebAppStatus(agentId, data),
    onMutate: async ({ agentId, data }) => {
      await Promise.all([
        queryClient.cancelQueries({ queryKey: AGENT_KEYS.lists() }),
        queryClient.cancelQueries({ queryKey: AGENT_KEYS.detail(agentId) }),
      ]);

      const previousLists = queryClient.getQueriesData<{
        pages: Array<ApiResponseData<AgentList>>;
      }>({ queryKey: AGENT_KEYS.lists() });
      const previousDetail = queryClient.getQueryData<ApiResponseData<AgentDetail>>(
        AGENT_KEYS.detail(agentId)
      );

      const setStatus = <T extends { web_app_status?: WebAppStatus }>(item: T): T => ({
        ...item,
        web_app_status: data.status,
      });

      queryClient.setQueriesData(
        { queryKey: AGENT_KEYS.lists() },
        (old: { pages: Array<ApiResponseData<AgentList>> } | undefined) => {
          if (!old?.pages) return old;
          return {
            ...old,
            pages: old.pages.map((page: ApiResponseData<AgentList>) => {
              const list = page.data?.data ?? [];
              const updated = list.map((item: Agent) =>
                item.id === agentId ? setStatus(item) : item
              );
              return page.data ? { ...page, data: { ...page.data, data: updated } } : page;
            }),
          };
        }
      );

      if (previousDetail?.data) {
        queryClient.setQueryData(AGENT_KEYS.detail(agentId), {
          ...previousDetail,
          data: setStatus(previousDetail.data),
        });
      }

      return { previousLists, previousDetail };
    },
    onError: (error, variables, context) => {
      if (context?.previousLists) {
        context.previousLists.forEach(([key, data]) => {
          queryClient.setQueryData(key, data);
        });
      }
      if (context?.previousDetail && variables?.agentId) {
        queryClient.setQueryData(AGENT_KEYS.detail(variables.agentId), context.previousDetail);
      }
      toast.error(getErrorMessage(error) || t('toasts.webAppStatusUpdateFailed'));
    },
    onSuccess: (_data, variables) => {
      toast.success(
        variables.data.status === 'active'
          ? t('toasts.webAppOnlineSuccess')
          : t('toasts.webAppOfflineSuccess')
      );
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({
        queryKey: AGENT_KEYS.all,
        predicate: workspaceInvalidatePredicate(AGENT_KEYS.all[0], currentWorkspaceId),
      });
      queryClient.invalidateQueries({ queryKey: AGENT_KEYS.lists() });
      queryClient.invalidateQueries({ queryKey: [...AGENT_KEYS.all, 'runnable-webapps'] });

      if (variables?.agentId) {
        queryClient.invalidateQueries({
          queryKey: AGENT_KEYS.detail(variables.agentId),
        });
      }
    },
  });
}

/**
 * Hook for getting a single agent by ID
 * Useful when you only need one agent's details
 */
export function useAgent(agentId: string | null, enabled: boolean = true) {
  const query = useQuery({
    queryKey: AGENT_KEYS.detail(agentId || ''),
    queryFn: () => (agentId ? agentService.getAgent(agentId) : Promise.resolve(null)),
    enabled: !!agentId && enabled,
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  });

  return {
    agent: query.data,
    isLoading: query.isLoading,
    error: query.error,
    refetch: query.refetch,
  };
}
