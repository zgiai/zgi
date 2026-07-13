'use client';

import { useEffect, useMemo } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { toast } from 'sonner';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import { agentService } from '@/services';
import type { ApiResponseData } from '@/services/types/common';
import type {
  RunnableWebAppItem,
  RunnableWebAppsData,
  RunnableWebAppMetaData,
  RunnableWebAppsParams,
} from '@/services/types/agent';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { getErrorMessage } from '@/utils/error-notifications';

export type RunnableWebAppIconType = 'image' | 'text';

export interface RunnableWebAppResolvedMetaData {
  title: string;
  icon: string;
  icon_type: RunnableWebAppIconType;
  icon_url: string;
  desc: string | null;
  agent_type: RunnableWebAppMetaData['agent_type'];
}

export type RunnableWebAppResolvedItem = Omit<RunnableWebAppItem, 'meta_data'> & {
  meta_data: RunnableWebAppResolvedMetaData;
  icon_type: RunnableWebAppIconType;
};

interface UseRunnableWebAppsOptions {
  workspaceId?: string | null;
  enabled?: boolean;
  staleTime?: number;
  webAppId?: string | null;
  webAppIds?: string[];
  keyword?: string;
  page?: number;
  pageSize?: number;
}

interface UseInfiniteRunnableWebAppsOptions {
  workspaceId?: string | null;
  enabled?: boolean;
  staleTime?: number;
  keyword?: string;
  pageSize?: number;
}

function useRunnableWebAppQueryScope(workspaceId?: string | null) {
  const currentWorkspace = useCurrentWorkspace();
  const {
    canUseRuntimeResourceList,
    isLoading: isCapabilitiesLoading,
    isFetching: isCapabilitiesFetching,
  } = useAccountCapabilities();
  const canUseAppCenterResourceList = canUseRuntimeResourceList('app_center');
  const resolvedWorkspaceId = useMemo(() => {
    if (typeof workspaceId !== 'undefined') {
      return workspaceId;
    }
    return currentWorkspace?.id ?? null;
  }, [workspaceId, currentWorkspace?.id]);

  return {
    resolvedWorkspaceId,
    canUseAppCenterResourceList,
    isCapabilitiesLoading,
    isCapabilitiesFetching,
  };
}

export function useRunnableWebApps({
  workspaceId,
  enabled = true,
  staleTime = 60 * 1000,
  webAppId,
  webAppIds,
  keyword,
  page,
  pageSize,
}: UseRunnableWebAppsOptions = {}) {
  const {
    resolvedWorkspaceId,
    canUseAppCenterResourceList,
    isCapabilitiesLoading,
    isCapabilitiesFetching,
  } = useRunnableWebAppQueryScope(workspaceId);
  const queryEnabled = enabled && canUseAppCenterResourceList;

  const webAppIDsParam = useMemo(
    () =>
      webAppIds
        ?.map(id => id.trim())
        .filter(Boolean)
        .join(',') || undefined,
    [webAppIds]
  );

  const requestParams = useMemo<RunnableWebAppsParams>(
    () => ({
      workspace_id: resolvedWorkspaceId || undefined,
      web_app_id: webAppId?.trim() || undefined,
      web_app_ids: webAppIDsParam,
      keyword: keyword?.trim() || undefined,
      page,
      page_size: pageSize,
    }),
    [keyword, page, pageSize, resolvedWorkspaceId, webAppId, webAppIDsParam]
  );

  const query = useQuery<ApiResponseData<RunnableWebAppsData>>({
    queryKey: AGENT_KEYS.runnable(resolvedWorkspaceId, requestParams),
    queryFn: () => agentService.getRunnableWebApps(requestParams),
    enabled: queryEnabled,
    staleTime,
    gcTime: 10 * 60 * 1000,
    retry: false,
  });

  const rawItems = useMemo<RunnableWebAppItem[]>(
    () => query.data?.data?.items ?? [],
    [query.data?.data?.items]
  );
  const items = useMemo(() => resolveRunnableWebAppItems(rawItems), [rawItems]);

  useEffect(() => {
    if (!queryEnabled || !query.isError) return;
    toast.error(getErrorMessage(query.error));
  }, [queryEnabled, query.isError, query.error]);

  return {
    ...query,
    items: queryEnabled ? items : [],
    page: query.data?.data?.page ?? page ?? 1,
    pageSize: query.data?.data?.page_size ?? pageSize ?? items.length,
    total: query.data?.data?.total ?? items.length,
    hasMore: query.data?.data?.has_more ?? false,
    isLoading: enabled && isCapabilitiesLoading ? true : query.isLoading,
    isFetching: (enabled && isCapabilitiesFetching) || query.isFetching,
    canUseResourceList: canUseAppCenterResourceList,
  };
}

export function useInfiniteRunnableWebApps({
  workspaceId,
  enabled = true,
  staleTime = 60 * 1000,
  keyword,
  pageSize = 20,
}: UseInfiniteRunnableWebAppsOptions = {}) {
  const {
    resolvedWorkspaceId,
    canUseAppCenterResourceList,
    isCapabilitiesLoading,
    isCapabilitiesFetching,
  } = useRunnableWebAppQueryScope(workspaceId);
  const queryEnabled = enabled && canUseAppCenterResourceList;
  const requestParams = useMemo<RunnableWebAppsParams>(
    () => ({
      workspace_id: resolvedWorkspaceId || undefined,
      keyword: keyword?.trim() || undefined,
      page_size: pageSize,
    }),
    [keyword, pageSize, resolvedWorkspaceId]
  );

  const query = useInfiniteQuery<ApiResponseData<RunnableWebAppsData>>({
    queryKey: AGENT_KEYS.runnableInfinite(resolvedWorkspaceId, requestParams),
    queryFn: ({ pageParam }) =>
      agentService.getRunnableWebApps({ ...requestParams, page: Number(pageParam) }),
    initialPageParam: 1,
    getNextPageParam: (lastPage, allPages) => {
      const pageData = lastPage.data;
      if (!pageData?.has_more) return undefined;
      return (pageData.page ?? allPages.length) + 1;
    },
    enabled: queryEnabled,
    staleTime,
    gcTime: 10 * 60 * 1000,
    retry: false,
  });

  const rawItems = useMemo(
    () => query.data?.pages.flatMap(page => page.data?.items ?? []) ?? [],
    [query.data?.pages]
  );
  const items = useMemo(() => resolveRunnableWebAppItems(rawItems), [rawItems]);

  useEffect(() => {
    if (!queryEnabled || !query.isError) return;
    toast.error(getErrorMessage(query.error));
  }, [queryEnabled, query.isError, query.error]);

  return {
    ...query,
    items: queryEnabled ? items : [],
    hasMore: query.hasNextPage ?? false,
    isLoading: enabled && isCapabilitiesLoading ? true : query.isLoading,
    isFetching: (enabled && isCapabilitiesFetching) || query.isFetching,
    canUseResourceList: canUseAppCenterResourceList,
  };
}

function resolveRunnableWebAppItems(rawItems: RunnableWebAppItem[]): RunnableWebAppResolvedItem[] {
  return rawItems.map((item, index) => {
    const apiMeta = item.meta_data;
    const title = apiMeta?.name?.trim() || `App ${index + 1}`;
    const icon = apiMeta?.icon || '';
    const iconType = (apiMeta?.icon_type || inferIconType(icon)) as RunnableWebAppIconType;

    return {
      ...item,
      meta_data: {
        title,
        icon,
        icon_type: iconType,
        icon_url: apiMeta?.icon_url || '',
        desc: apiMeta?.desc ?? null,
        agent_type: apiMeta?.agent_type || 'CONVERSATIONAL_WORKFLOW',
      },
      icon_type: iconType,
    };
  });
}

function inferIconType(icon: string): RunnableWebAppIconType {
  if (!icon) return 'text';

  if (icon.startsWith('http://') || icon.startsWith('https://') || icon.startsWith('/')) {
    return 'image';
  }

  try {
    const parsed = JSON.parse(icon) as { icon?: string; icon_background?: string };
    if (parsed.icon || parsed.icon_background) {
      return 'text';
    }
  } catch {
    return 'text';
  }

  return 'text';
}
