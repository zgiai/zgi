'use client';

import { useEffect, useMemo } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { agentRuntimeLogsService } from '@/services/agent-runtime-logs.service';
import type { AgentRuntimeStep } from '@/services/types/agent-runtime-log';
import type { ApiResponseData } from '@/services/types/common';
import { normalizeToastDescription } from '@/utils/error-notifications';

export interface UseAgentRuntimeRunStepsParams {
  agentId: string | null;
  messageId: string | null;
}

export interface UseAgentRuntimeRunStepsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
}

export function useAgentRuntimeRunSteps(
  { agentId, messageId }: UseAgentRuntimeRunStepsParams,
  {
    enabled = true,
    staleTime = 60_000,
    gcTime = 10 * 60_000,
    refetchOnWindowFocus = false,
  }: UseAgentRuntimeRunStepsOptions = {}
) {
  const t = useT('agents');
  const queryClient = useQueryClient();
  const queryKey = useMemo(
    () => AGENT_KEYS.runtimeRunSteps(agentId ?? 'none', messageId ?? 'none'),
    [agentId, messageId]
  );

  const { data, isLoading, isFetching, error } = useQuery<
    ApiResponseData<{ data: AgentRuntimeStep[] }>
  >({
    queryKey,
    queryFn: () =>
      agentRuntimeLogsService.getRuntimeRunSteps(agentId as string, messageId as string),
    enabled: Boolean(agentId && messageId) && enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
  });

  useEffect(() => {
    if (!error) return;
    const title = t('workflow.errors.loadNodeExecutionsFailed');
    toast.error(title, {
      description: normalizeToastDescription(title, (error as Error).message),
    });
  }, [error, t]);

  return {
    steps: data?.data?.data ?? [],
    isLoading,
    isFetching,
    error: (error as Error | null)?.message ?? null,
    reload: async () => {
      await queryClient.invalidateQueries({ queryKey });
    },
  };
}
