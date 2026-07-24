'use client';

import { useQuery } from '@tanstack/react-query';
import { WEBAPP_KEYS } from '@/hooks/query-keys';
import { WebAppService } from '@/services/webapp.service';

export function usePublishedAgentModelPrecheck(webAppId: string, options?: { enabled?: boolean }) {
  const normalizedWebAppId = webAppId.trim();

  return useQuery({
    queryKey: WEBAPP_KEYS.agentModelPrecheck(normalizedWebAppId),
    queryFn: async () => {
      const response = await WebAppService.precheckPublishedAgentModel(normalizedWebAppId);
      return response.data;
    },
    enabled: (options?.enabled ?? true) && normalizedWebAppId.length > 0,
    retry: false,
    staleTime: 0,
    refetchOnMount: true,
    refetchOnReconnect: true,
    refetchOnWindowFocus: true,
  });
}
