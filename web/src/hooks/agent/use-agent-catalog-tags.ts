'use client';

import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { agentService } from '@/services/agent.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  AgentAssetKind,
  AgentCatalogTagFacetsData,
  AgentCatalogTagsData,
  ReplaceAgentCatalogTagsRequest,
} from '@/services/types/agent';
import { getErrorMessage } from '@/utils/error-notifications';

export function useAgentCatalogTags(agentId: string | null) {
  return useQuery<ApiResponseData<AgentCatalogTagsData>>({
    queryKey: AGENT_KEYS.catalogTags(agentId ?? 'none'),
    queryFn: () => agentService.getAgentCatalogTags(agentId ?? ''),
    enabled: Boolean(agentId),
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    retry: false,
  });
}

export function useAgentCatalogTagFacets({
  workspaceId,
  assetKind,
  enabled = true,
}: {
  workspaceId?: string | null;
  assetKind?: AgentAssetKind;
  enabled?: boolean;
}) {
  const queryEnabled = enabled && Boolean(workspaceId);
  const query = useQuery<ApiResponseData<AgentCatalogTagFacetsData>>({
    queryKey: AGENT_KEYS.catalogTagFacets(workspaceId, assetKind),
    queryFn: () =>
      agentService.getAgentCatalogTagFacets({
        workspace_id: workspaceId || undefined,
        asset_kind: assetKind,
      }),
    enabled: queryEnabled,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    retry: false,
  });

  useEffect(() => {
    if (!queryEnabled || !query.isError) return;
    toast.error(getErrorMessage(query.error));
  }, [query.error, query.isError, queryEnabled]);

  return {
    ...query,
    items: query.data?.data?.items ?? [],
  };
}

export function useReplaceAgentCatalogTags() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      agentId,
      payload,
    }: {
      agentId: string;
      payload: ReplaceAgentCatalogTagsRequest;
    }) => agentService.replaceAgentCatalogTags(agentId, payload),
    onSuccess: (data, variables) => {
      queryClient.setQueryData(AGENT_KEYS.catalogTags(variables.agentId), data);
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: AGENT_KEYS.lists() }),
        queryClient.invalidateQueries({ queryKey: [...AGENT_KEYS.all, 'runnable-webapps'] }),
        queryClient.invalidateQueries({
          queryKey: [...AGENT_KEYS.all, 'runnable-webapp-tag-facets'],
        }),
        queryClient.invalidateQueries({
          queryKey: [...AGENT_KEYS.all, 'catalog-tag-facets'],
        }),
      ]);
    },
    onError: error => {
      toast.error(getErrorMessage(error));
    },
  });
}
