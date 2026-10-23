'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { runtimeWorkspaceService } from '@/services/runtime-workspace.service';
import type {
  RuntimeWorkspaceAgentSelectionInput,
  RuntimeWorkspaceCredentialInput,
  RuntimeWorkspaceStatus,
} from '@/services/types/runtime-workspace';
import { RUNTIME_WORKSPACE_KEYS } from '@/hooks/query-keys';
import { getErrorMessage } from '@/utils/error-notifications';
import { useT } from '@/i18n';

/** Every access key, revoked and expired ones included, loaded on request. */
export function useRuntimeWorkspaceAllCredentials(runtimeWorkspaceId: string, enabled: boolean) {
  return useQuery({
    queryKey: [...RUNTIME_WORKSPACE_KEYS.credentials(runtimeWorkspaceId), 'all'],
    queryFn: () => runtimeWorkspaceService.listCredentials(runtimeWorkspaceId, 'all'),
    enabled: enabled && Boolean(runtimeWorkspaceId),
    retry: false,
  });
}

export function useRuntimeWorkspace(workspaceId: string | null) {
  const queryClient = useQueryClient();
  const t = useT('workspace');
  const enabled = Boolean(workspaceId);

  const current = useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.current(workspaceId),
    queryFn: () => runtimeWorkspaceService.getCurrent(workspaceId as string),
    enabled,
    retry: false,
    staleTime: 30_000,
  });

  const apps = useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.apps(current.data?.workspace_id ?? null),
    queryFn: () => runtimeWorkspaceService.listApps(current.data?.workspace_id as string),
    enabled: Boolean(current.data?.workspace_id),
    retry: false,
  });

  const candidates = useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.candidates(current.data?.workspace_id ?? null, 'current'),
    queryFn: () =>
      runtimeWorkspaceService.listAgentCandidates(current.data?.workspace_id as string, 'current'),
    enabled: Boolean(current.data?.workspace_id),
    retry: false,
    staleTime: 60_000,
  });

  const crossWorkspaceCandidates = useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.candidates(current.data?.workspace_id ?? null, 'authorized'),
    queryFn: () =>
      runtimeWorkspaceService.listAgentCandidates(
        current.data?.workspace_id as string,
        'authorized'
      ),
    enabled: Boolean(current.data?.workspace_id && candidates.data?.cross_workspace_enabled),
    retry: false,
    staleTime: 60_000,
  });

  const credentials = useQuery({
    queryKey: RUNTIME_WORKSPACE_KEYS.credentials(current.data?.workspace_id ?? null),
    queryFn: () =>
      runtimeWorkspaceService.listCredentials(current.data?.workspace_id as string, 'active'),
    enabled: Boolean(current.data?.workspace_id),
    retry: false,
  });

  const reportError = (error: unknown) => {
    toast.error(getErrorMessage(error));
  };

  const create = useMutation({
    mutationFn: () => runtimeWorkspaceService.create(workspaceId as string),
    onSuccess: data => {
      queryClient.setQueryData(RUNTIME_WORKSPACE_KEYS.current(workspaceId), data);
      toast.success(t('runtime.messages.enabled'));
    },
    onError: reportError,
  });

  const update = useMutation({
    mutationFn: (data: {
      status?: RuntimeWorkspaceStatus;
      policy_json?: Record<string, unknown>;
    }) => runtimeWorkspaceService.update(current.data?.workspace_id as string, data),
    onSuccess: (data, variables) => {
      queryClient.setQueryData(RUNTIME_WORKSPACE_KEYS.current(workspaceId), data);
      if (variables.policy_json && !variables.status) {
        toast.success(t('runtime.messages.quotaSaved'));
        return;
      }
      toast.success(
        data.status === 'active' ? t('runtime.messages.enabled') : t('runtime.messages.disabled')
      );
    },
    onError: reportError,
  });

  const createCredential = useMutation({
    mutationFn: (data: RuntimeWorkspaceCredentialInput) =>
      runtimeWorkspaceService.createCredential(current.data?.workspace_id as string, data),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.credentials(current.data?.workspace_id ?? null),
      });
    },
    onError: reportError,
  });

  const revokeCredential = useMutation({
    mutationFn: (credentialId: string) =>
      runtimeWorkspaceService.revokeCredential(current.data?.workspace_id as string, credentialId),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.credentials(current.data?.workspace_id ?? null),
      });
      toast.success(t('runtime.messages.keyRevoked'));
    },
    onError: reportError,
  });

  const syncAgents = useMutation({
    mutationFn: (data: RuntimeWorkspaceAgentSelectionInput) =>
      runtimeWorkspaceService.syncAgents(current.data?.workspace_id as string, data),
    onSuccess: data => {
      queryClient.setQueryData(
        RUNTIME_WORKSPACE_KEYS.apps(current.data?.workspace_id ?? null),
        data
      );
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.candidateScopes(current.data?.workspace_id ?? null),
      });
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.apps(current.data?.workspace_id ?? null),
      });
    },
    onError: error => {
      reportError(error);
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.apps(current.data?.workspace_id ?? null),
      });
      void queryClient.invalidateQueries({
        queryKey: RUNTIME_WORKSPACE_KEYS.candidateScopes(current.data?.workspace_id ?? null),
      });
    },
  });

  return {
    current,
    apps,
    candidates,
    crossWorkspaceCandidates,
    credentials,
    create,
    update,
    createCredential,
    revokeCredential,
    syncAgents,
  };
}
