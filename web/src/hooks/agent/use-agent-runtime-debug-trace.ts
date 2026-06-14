'use client';

import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { agentRuntimeLogsService } from '@/services/agent-runtime-logs.service';
import type { AgentRuntimeDebugTrace } from '@/services/types/agent-runtime-log';
import type { ApiResponseData } from '@/services/types/common';

export interface UseAgentRuntimeDebugTraceParams {
  agentId: string | null;
  messageId: string | null;
  runtimeId: string | null;
}

export interface UseAgentRuntimeDebugTraceOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
}

export function useAgentRuntimeDebugTrace(
  { agentId, messageId, runtimeId }: UseAgentRuntimeDebugTraceParams,
  {
    enabled = true,
    staleTime = 30_000,
    gcTime = 5 * 60_000,
    refetchOnWindowFocus = false,
  }: UseAgentRuntimeDebugTraceOptions = {}
) {
  const queryKey = useMemo(
    () =>
      AGENT_KEYS.runtimeRunDebugTrace(agentId ?? 'none', messageId ?? 'none', runtimeId ?? 'none'),
    [agentId, messageId, runtimeId]
  );

  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<AgentRuntimeDebugTrace>
  >({
    queryKey,
    queryFn: () =>
      agentRuntimeLogsService.getRuntimeRunDebugTrace(
        agentId as string,
        messageId as string,
        runtimeId as string
      ),
    enabled: Boolean(agentId && messageId && runtimeId) && enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
  });

  return {
    trace: data?.data ?? null,
    isLoading,
    isFetching,
    error: (error as Error | null)?.message ?? null,
    reload: refetch,
  };
}
