'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { WORKFLOW_KEYS } from '@/hooks/query-keys';
import { workflowService } from '@/services/workflow.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  BuiltInWorkflowRuntimeSurfaceAuthorizationResponse,
  UpdatePublishedRuntimeSurfacesRequest,
} from '@/services/types/workflow';
import { getErrorMessage } from '@/utils/error-notifications';

export function useBuiltInWorkflowRuntimeSurfaces(scenario: string | null) {
  return useQuery<ApiResponseData<BuiltInWorkflowRuntimeSurfaceAuthorizationResponse>>({
    queryKey: WORKFLOW_KEYS.builtInRuntimeSurfaces(scenario ?? 'none'),
    queryFn: () => workflowService.getBuiltInWorkflowRuntimeSurfaces(scenario ?? ''),
    enabled: Boolean(scenario),
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    retry: false,
  });
}

export function useUpdateBuiltInWorkflowRuntimeSurfaces() {
  const queryClient = useQueryClient();
  const t = useT('dashboard.organization.permissions.builtInRuntime');

  return useMutation({
    mutationFn: ({
      scenario,
      payload,
    }: {
      scenario: string;
      payload: UpdatePublishedRuntimeSurfacesRequest;
    }) => workflowService.updateBuiltInWorkflowRuntimeSurfaces(scenario, payload),
    onSuccess: data => {
      queryClient.setQueryData(WORKFLOW_KEYS.builtInRuntimeSurfaces(data.data.scenario), data);
      void queryClient.invalidateQueries({ queryKey: WORKFLOW_KEYS.builtIn() });
      toast.success(t('saveSuccess'));
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('saveError'));
    },
  });
}
