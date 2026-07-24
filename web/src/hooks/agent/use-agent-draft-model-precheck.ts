'use client';

import { useQuery } from '@tanstack/react-query';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { agentService } from '@/services/agent.service';

export function useAgentDraftModelPrecheck(
  agentId: string,
  provider: string,
  model: string,
  options?: { enabled?: boolean }
) {
  const normalizedAgentId = agentId.trim();
  const normalizedProvider = provider.trim();
  const normalizedModel = model.trim();

  return useQuery({
    queryKey: AGENT_KEYS.modelPrecheck(normalizedAgentId, normalizedProvider, normalizedModel),
    queryFn: async () => {
      const response = await agentService.precheckAgentDraftModel(normalizedAgentId, {
        provider: normalizedProvider,
        model: normalizedModel,
      });
      return response.data;
    },
    enabled:
      (options?.enabled ?? true) &&
      normalizedAgentId.length > 0 &&
      normalizedProvider.length > 0 &&
      normalizedModel.length > 0,
    retry: false,
    staleTime: 0,
    refetchOnMount: true,
    refetchOnReconnect: true,
    refetchOnWindowFocus: true,
  });
}
