'use client';

import { useEffect, useMemo } from 'react';
import { useQueries, useQuery } from '@tanstack/react-query';
import { toast } from 'sonner';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { WEBAPP_KEYS } from '@/hooks/query-keys';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import { agentService } from '@/services';
import type { ApiResponseData } from '@/services/types/common';
import type {
  RunnableWebAppItem,
  RunnableWebAppsData,
  RunnableWebAppMetaData,
} from '@/services/types/agent';
import { WebAppService } from '@/services/webapp.service';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { getErrorMessage } from '@/utils/error-notifications';

export type RunnableWebAppIconType = 'image' | 'text';

export interface RunnableWebAppResolvedItem extends RunnableWebAppItem {
  meta_data: RunnableWebAppMetaData;
  icon_type: RunnableWebAppIconType;
}

interface UseRunnableWebAppsOptions {
  workspaceId?: string | null;
  enabled?: boolean;
  staleTime?: number;
}

export function useRunnableWebApps({
  workspaceId,
  enabled = true,
  staleTime = 60 * 1000,
}: UseRunnableWebAppsOptions = {}) {
  const currentWorkspace = useCurrentWorkspace();
  const {
    canUseRuntimeResourceList,
    isLoading: isCapabilitiesLoading,
    isFetching: isCapabilitiesFetching,
  } = useAccountCapabilities();
  const canUseAppCenterResourceList = canUseRuntimeResourceList('app_center');
  const queryEnabled = enabled && canUseAppCenterResourceList;

  const resolvedWorkspaceId = useMemo(() => {
    if (typeof workspaceId !== 'undefined') {
      return workspaceId;
    }
    return currentWorkspace?.id ?? null;
  }, [workspaceId, currentWorkspace?.id]);

  const query = useQuery<ApiResponseData<RunnableWebAppsData>>({
    queryKey: AGENT_KEYS.runnable(resolvedWorkspaceId),
    queryFn: () =>
      agentService.getRunnableWebApps(
        resolvedWorkspaceId ? { workspace_id: resolvedWorkspaceId } : undefined
      ),
    enabled: queryEnabled,
    staleTime,
    gcTime: 10 * 60 * 1000,
    retry: false,
  });

  const rawItems = query.data?.data?.items ?? [];

  const configQueries = useQueries({
    queries: rawItems.map(item => ({
      queryKey: WEBAPP_KEYS.config(item.web_app_id),
      queryFn: () => WebAppService.getConfig(item.web_app_id),
      staleTime,
      enabled: queryEnabled,
      retry: false,
    })),
  });

  const items = useMemo<RunnableWebAppResolvedItem[]>(() => {
    return rawItems.map((item, index) => {
      const configMeta = configQueries[index]?.data?.data?.config;
      const apiMeta = item.meta_data;
      const title = apiMeta?.title || configMeta?.title || `App ${index + 1}`;
      const icon = apiMeta?.icon || configMeta?.icon || '';
      const iconType = apiMeta?.icon_type || configMeta?.icon_type || inferIconType(icon);
      const iconUrl = apiMeta?.icon_url || configMeta?.icon_url || '';
      const desc = apiMeta?.desc ?? null;
      const agentType = apiMeta?.agent_type || configMeta?.type || 'CONVERSATIONAL_WORKFLOW';

      return {
        ...item,
        meta_data: {
          title,
          icon,
          icon_type: iconType,
          icon_url: iconUrl,
          desc,
          agent_type: agentType,
        },
        icon_type: iconType as RunnableWebAppIconType,
      };
    });
  }, [configQueries, rawItems]);

  useEffect(() => {
    if (!queryEnabled || !query.isError) return;
    toast.error(getErrorMessage(query.error));
  }, [queryEnabled, query.isError, query.error]);

  return {
    ...query,
    items: queryEnabled ? items : [],
    isLoading: enabled && isCapabilitiesLoading ? true : query.isLoading,
    isFetching: (enabled && isCapabilitiesFetching) || query.isFetching,
    canUseResourceList: canUseAppCenterResourceList,
  };
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
