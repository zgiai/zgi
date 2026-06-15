'use client';

import { useCallback, useEffect, useMemo } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { agentRuntimeLogsService } from '@/services/agent-runtime-logs.service';
import type { AgentRuntimeRunDetail } from '@/services/types/agent-runtime-log';
import type { ApiResponseData } from '@/services/types/common';
import { normalizeToastDescription } from '@/utils/error-notifications';

export interface UseAgentRuntimeRunDetailParams {
  agentId: string | null;
  messageId: string | null;
}

export interface UseAgentRuntimeRunDetailOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  suppressErrorToast?: boolean;
}

export function useAgentRuntimeRunDetail(
  { agentId, messageId }: UseAgentRuntimeRunDetailParams,
  {
    enabled = true,
    staleTime = 60_000,
    gcTime = 10 * 60_000,
    refetchOnWindowFocus = false,
    suppressErrorToast = false,
  }: UseAgentRuntimeRunDetailOptions = {}
) {
  const t = useT('agents');
  const queryClient = useQueryClient();
  const queryKey = useMemo(
    () => AGENT_KEYS.runtimeRunDetail(agentId ?? 'none', messageId ?? 'none'),
    [agentId, messageId]
  );

  const { data, isLoading, isFetching, error } = useQuery<ApiResponseData<AgentRuntimeRunDetail>>({
    queryKey,
    queryFn: () =>
      agentRuntimeLogsService.getRuntimeRunDetail(agentId as string, messageId as string),
    enabled: Boolean(agentId && messageId) && enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
  });

  useEffect(() => {
    if (!error || suppressErrorToast) return;
    const title = t('workflow.errors.loadRunDetailFailed');
    toast.error(title, {
      description: normalizeToastDescription(title, (error as Error).message),
    });
  }, [error, suppressErrorToast, t]);

  const reload = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey });
  }, [queryClient, queryKey]);

  return {
    detail: data?.data ?? null,
    isLoading,
    isFetching,
    error: (error as Error | null)?.message ?? null,
    reload,
  };
}
