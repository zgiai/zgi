'use client';

import { useMemo, useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { workflowService } from '@/services/workflow.service';
import type { ApiResponseData } from '@/services/types/common';
import type { WorkflowNodeExecution } from '@/services/types/workflow';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { normalizeToastDescription } from '@/utils/error-notifications';

const WORKFLOW_RUN_NODE_EXECUTIONS_KEY = 'workflow-run-node-executions';

export function getWorkflowRunNodeExecutionsKey(agentId?: string, runId?: string) {
  return [WORKFLOW_RUN_NODE_EXECUTIONS_KEY, agentId ?? 'none', runId ?? 'none'];
}

export interface UseWorkflowRunNodeExecutionsParams {
  agentId: string | null | undefined;
  runId: string | null | undefined;
}

export interface UseWorkflowRunNodeExecutionsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
}

export interface UseWorkflowRunNodeExecutionsReturn {
  records: WorkflowNodeExecution[];
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  reload: () => Promise<void>;
}

export function useWorkflowRunNodeExecutions(
  { agentId, runId }: UseWorkflowRunNodeExecutionsParams,
  {
    enabled = true,
    staleTime = 60_000,
    gcTime = 10 * 60_000,
    refetchOnWindowFocus = false,
  }: UseWorkflowRunNodeExecutionsOptions = {}
): UseWorkflowRunNodeExecutionsReturn {
  const t = useT('agents');
  const queryClient = useQueryClient();

  const isEnabled = Boolean(agentId && runId) && enabled;
  const queryKey = useMemo(
    () => getWorkflowRunNodeExecutionsKey(agentId ?? undefined, runId ?? undefined),
    [agentId, runId]
  );

  const { data, isLoading, isFetching, error } = useQuery<
    ApiResponseData<{ data: WorkflowNodeExecution[] }>
  >({
    queryKey,
    queryFn: () => workflowService.getWorkflowRunNodeExecutions(agentId as string, runId as string),
    enabled: isEnabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
  });

  useEffect(() => {
    if (error) {
      const title = t('workflow.errors.loadNodeExecutionsFailed');
      toast.error(title, {
        description: normalizeToastDescription(title, (error as Error)?.message),
      });
    }
  }, [error, t]);

  return {
    records: data?.data?.data ?? [],
    isLoading,
    isFetching,
    error: (error as Error | null)?.message ?? null,
    reload: async () => {
      await queryClient.invalidateQueries({ queryKey });
    },
  };
}
