'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { agentService } from '@/services/agent.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  AgentRuntimeSurfaceAuthorizationResponse,
  UpdateAgentRuntimeSurfacesRequest,
} from '@/services/types/agent';
import { getErrorMessage } from '@/utils/error-notifications';

export function useAgentRuntimeSurfaces(agentId: string | null) {
  return useQuery<ApiResponseData<AgentRuntimeSurfaceAuthorizationResponse>>({
    queryKey: AGENT_KEYS.runtimeSurfaces(agentId ?? 'none'),
    queryFn: () => agentService.getAgentRuntimeSurfaces(agentId ?? ''),
    enabled: Boolean(agentId),
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    retry: false,
  });
}

export function useUpdateAgentRuntimeSurfaces() {
  const queryClient = useQueryClient();
  const t = useT('agents.runtimeAccess');

  return useMutation({
    mutationFn: ({
      agentId,
      payload,
    }: {
      agentId: string;
      payload: UpdateAgentRuntimeSurfacesRequest;
    }) => agentService.updateAgentRuntimeSurfaces(agentId, payload),
    onSuccess: data => {
      const agentId = data.data.agent_id;
      queryClient.setQueryData(AGENT_KEYS.runtimeSurfaces(agentId), data);
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: AGENT_KEYS.detail(agentId) }),
        queryClient.invalidateQueries({ queryKey: AGENT_KEYS.lists() }),
        queryClient.invalidateQueries({ queryKey: [...AGENT_KEYS.all, 'runnable-webapps'] }),
      ]);
      toast.success(t('saveSuccess'));
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('saveError'));
    },
  });
}
