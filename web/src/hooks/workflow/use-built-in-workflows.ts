'use client';

// Hook for fetching and caching built-in workflows
// Uses client local cache with 1-week expiration; only fetches when cache is empty
// English comments only. Strict TypeScript (no any).

import { useEffect, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { toast } from 'sonner';
import { getErrorMessage } from '@/utils/error-notifications';
import { workflowService } from '@/services/workflow.service';
import type { ApiResponseData } from '@/services/types/common';
import type { BuiltInWorkflowList, BuiltInWorkflow } from '@/services/types/workflow';
import {
  getCachedBuiltInWorkflows,
  saveBuiltInWorkflows,
  findBuiltInWorkflowByScenario,
} from '@/utils/workflow/built-in-workflows';
import { WORKFLOW_KEYS } from '@/hooks/query-keys';

/* -------------------------------------------------------------------------- */
/* Query-key helpers                                                          */
/* -------------------------------------------------------------------------- */

// BUILT_IN_WORKFLOWS_QUERY_KEY is now centralized in WORKFLOW_KEYS.builtIn
const getBuiltInWorkflowsKey = () => WORKFLOW_KEYS.builtIn();

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseBuiltInWorkflowsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
}

export interface UseBuiltInWorkflowsReturn {
  workflows: BuiltInWorkflowList;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<unknown>;
  /** Get a workflow by scenario (e.g., 'bi_chat') */
  getByScenario: (scenario: string) => BuiltInWorkflow | undefined;
  /** Get bi_chat workflow */
  biChatWorkflow: BuiltInWorkflow | undefined;
  /** Get global_chat workflow */
  globalChatWorkflow: BuiltInWorkflow | undefined;
  /** Get imagegen_chat workflow */
  imageGenChatWorkflow: BuiltInWorkflow | undefined;
}

/* -------------------------------------------------------------------------- */
/* Hook: useBuiltInWorkflows                                                  */
/* -------------------------------------------------------------------------- */

export function useBuiltInWorkflows({
  enabled = true,
  staleTime = 7 * 24 * 60 * 60 * 1000, // 1 week
  gcTime = 7 * 24 * 60 * 60 * 1000, // 1 week
  refetchOnWindowFocus = false,
}: UseBuiltInWorkflowsOptions = {}): UseBuiltInWorkflowsReturn {
  // Check if cache exists
  const cachedData = useMemo(() => getCachedBuiltInWorkflows(), []);
  // Validate cache: must not be empty and must contain workflows required by console entrypoints.
  const hasCachedData = useMemo(() => {
    if (!cachedData) return false;
    if (cachedData.length === 0) return false;
    return ['bi_chat', 'imagegen_chat'].every(scenario =>
      cachedData.some(w => w.scenario === scenario)
    );
  }, [cachedData]);

  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<BuiltInWorkflowList>,
    unknown
  >({
    queryKey: getBuiltInWorkflowsKey(),
    queryFn: () => workflowService.getBuiltInWorkflows(),
    enabled: enabled && !hasCachedData,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    retry: false,
    // Initialize with cached data ONLY if valid
    initialData:
      hasCachedData && cachedData
        ? ({
            code: '0',
            message: 'success',
            data: cachedData,
          } as ApiResponseData<BuiltInWorkflowList>)
        : undefined,
  });

  // Cache successful API response
  useEffect(() => {
    const workflows = data?.data;
    if (workflows && Array.isArray(workflows) && workflows.length > 0 && !hasCachedData) {
      saveBuiltInWorkflows(workflows);
    }
  }, [data, hasCachedData]);

  // Side-effect error toast
  useEffect(() => {
    if (!error) return;
    const message = getErrorMessage(error);
    toast.error(message || 'loadFailed');
  }, [error]);

  const workflows: BuiltInWorkflowList = useMemo(
    () => data?.data ?? cachedData ?? [],
    [data, cachedData]
  );

  const getByScenario = useMemo(
    () => (scenario: string) => findBuiltInWorkflowByScenario(workflows, scenario),
    [workflows]
  );

  const biChatWorkflow = useMemo(() => getByScenario('bi_chat'), [getByScenario]);
  const globalChatWorkflow = useMemo(() => getByScenario('global_chat'), [getByScenario]);
  const imageGenChatWorkflow = useMemo(() => getByScenario('imagegen_chat'), [getByScenario]);

  return {
    workflows,
    isLoading: isLoading && !hasCachedData,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch,
    getByScenario,
    biChatWorkflow,
    globalChatWorkflow,
    imageGenChatWorkflow,
  };
}
