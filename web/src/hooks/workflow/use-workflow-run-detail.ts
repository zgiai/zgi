'use client';

import { useMemo, useEffect, useCallback } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { workflowService } from '@/services/workflow.service';
import type { WorkflowRunDetail } from '@/services/types/workflow';
import type { ApiResponseData } from '@/services/types/common';
import { toast } from 'sonner';
import { useT } from '@/i18n';

import { WORKFLOW_KEYS } from '@/hooks/query-keys';
import { normalizeToastDescription } from '@/utils/error-notifications';

/* --------------------------------- types -------------------------------- */
export interface UseWorkflowRunDetailParams {
  agentId: string | null;
  runId: string | null;
}

export interface UseWorkflowRunDetailOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  suppressErrorToast?: boolean;
}

export interface UseWorkflowRunDetailReturn {
  detail: WorkflowRunDetail | null;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  reload: () => Promise<void>;
  prefetch: () => Promise<void>;
}

/* --------------------------------- hook --------------------------------- */
export function useWorkflowRunDetail(
  { agentId, runId }: UseWorkflowRunDetailParams,
  {
    enabled = true,
    staleTime = 60_000,
    gcTime = 10 * 60_000,
    refetchOnWindowFocus = false,
    suppressErrorToast = false,
  }: UseWorkflowRunDetailOptions = {}
): UseWorkflowRunDetailReturn {
  const t = useT('agents');
  const queryClient = useQueryClient();

  const isEnabled = Boolean(agentId && runId) && enabled;
  const queryKey = useMemo(
    () => WORKFLOW_KEYS.runDetail(agentId ?? 'none', runId ?? 'none'),
    [agentId, runId]
  );

  const { data, isLoading, isFetching, error } = useQuery<ApiResponseData<WorkflowRunDetail>>({
    queryKey,
    queryFn: () => workflowService.getWorkflowRunDetail(agentId as string, runId as string),
    enabled: isEnabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
  });

  useEffect(() => {
    if (error && !suppressErrorToast) {
      const title = t('workflow.errors.loadRunDetailFailed');
      toast.error(title, {
        description: normalizeToastDescription(title, (error as Error)?.message),
      });
    }
  }, [error, suppressErrorToast, t]);

  const reload = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey });
  }, [queryClient, queryKey]);

  const prefetch = useCallback(async () => {
    if (!agentId || !runId) return;
    await queryClient.prefetchQuery({
      queryKey,
      queryFn: () => workflowService.getWorkflowRunDetail(agentId, runId),
    });
  }, [agentId, runId, queryClient, queryKey]);

  return {
    detail: data?.data ?? null,
    isLoading,
    isFetching,
    error: (error as Error | null)?.message ?? null,
    reload,
    prefetch,
  };
}
