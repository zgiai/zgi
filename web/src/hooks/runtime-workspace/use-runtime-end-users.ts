'use client';

import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { runtimeWorkspaceService } from '@/services/runtime-workspace.service';
import type {
  RuntimeEndUserListParams,
  RuntimeEndUserPatch,
} from '@/services/types/runtime-workspace';
import { RUNTIME_WORKSPACE_KEYS } from '@/hooks/query-keys';
import { getErrorMessage } from '@/utils/error-notifications';

/** End users of a Runtime Workspace, with their usage and quotas. */
export function useRuntimeEndUsers(
  runtimeWorkspaceId: string | null,
  params: RuntimeEndUserListParams
) {
  const queryClient = useQueryClient();
  const list = useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.endUserList(runtimeWorkspaceId, params),
    queryFn: () => runtimeWorkspaceService.listEndUsers(runtimeWorkspaceId as string, params),
    enabled: Boolean(runtimeWorkspaceId),
    placeholderData: keepPreviousData,
    retry: false,
  });
  const update = useMutation({
    mutationFn: ({ subjectId, patch }: { subjectId: string; patch: RuntimeEndUserPatch }) =>
      runtimeWorkspaceService.updateEndUser(runtimeWorkspaceId as string, subjectId, patch),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.endUsers(runtimeWorkspaceId),
      });
    },
    onError: error => {
      toast.error(getErrorMessage(error));
    },
  });
  return { list, update };
}

/** The recorded changes to one end user. */
export function useRuntimeEndUserEvents(
  runtimeWorkspaceId: string | null,
  subjectId: string | null
) {
  return useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.endUserEvents(runtimeWorkspaceId, subjectId),
    queryFn: () =>
      runtimeWorkspaceService.listEndUserEvents(runtimeWorkspaceId as string, subjectId as string),
    enabled: Boolean(runtimeWorkspaceId && subjectId),
    retry: false,
  });
}
