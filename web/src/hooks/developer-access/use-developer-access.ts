'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import {
  developerAccessService,
  type AccessRequestStatus,
  type CreateAccessRequestInput,
  type CreatePersonalApiKeyInput,
  type DeveloperAccessPolicy,
  type ReviewAccessRequestInput,
} from '@/services/developer-access.service';

export const DEVELOPER_ACCESS_KEYS = {
  all: ['developer-access'] as const,
  workspace: (workspaceId: string) => [...DEVELOPER_ACCESS_KEYS.all, workspaceId] as const,
  me: (workspaceId: string) => [...DEVELOPER_ACCESS_KEYS.workspace(workspaceId), 'me'] as const,
  policy: (workspaceId: string) =>
    [...DEVELOPER_ACCESS_KEYS.workspace(workspaceId), 'policy'] as const,
  requests: (workspaceId: string, status?: AccessRequestStatus) =>
    [...DEVELOPER_ACCESS_KEYS.workspace(workspaceId), 'requests', status ?? 'all'] as const,
  keys: (workspaceId: string) => [...DEVELOPER_ACCESS_KEYS.workspace(workspaceId), 'keys'] as const,
  audit: (workspaceId: string, page: number, pageSize: number) =>
    [...DEVELOPER_ACCESS_KEYS.workspace(workspaceId), 'audit', page, pageSize] as const,
};

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

function useWorkspaceMutation<TInput, TResult>(
  workspaceId: string | undefined,
  mutationFn: (input: TInput) => Promise<TResult>,
  successMessage: string,
  errorFallback: string
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: async () => {
      toast.success(successMessage);
      if (workspaceId) {
        await queryClient.invalidateQueries({
          queryKey: DEVELOPER_ACCESS_KEYS.workspace(workspaceId),
        });
      }
    },
    onError: error => toast.error(errorMessage(error, errorFallback)),
  });
}

export function useDeveloperAccess(workspaceId?: string, auditPage = 1, auditPageSize = 50) {
  const enabled = Boolean(workspaceId);
  const me = useQuery({
    queryKey: DEVELOPER_ACCESS_KEYS.me(workspaceId ?? ''),
    queryFn: async () => (await developerAccessService.getMe(workspaceId ?? '')).data,
    enabled,
    staleTime: 30_000,
  });
  const keys = useQuery({
    queryKey: DEVELOPER_ACCESS_KEYS.keys(workspaceId ?? ''),
    queryFn: async () => (await developerAccessService.listKeys(workspaceId ?? '')).data,
    enabled,
    staleTime: 15_000,
  });
  const requests = useQuery({
    queryKey: DEVELOPER_ACCESS_KEYS.requests(workspaceId ?? ''),
    queryFn: async () => (await developerAccessService.listRequests(workspaceId ?? '')).data,
    enabled,
    staleTime: 15_000,
  });
  const audit = useQuery({
    queryKey: DEVELOPER_ACCESS_KEYS.audit(workspaceId ?? '', auditPage, auditPageSize),
    queryFn: async () =>
      (
        await developerAccessService.listAudit(workspaceId ?? '', {
          page: auditPage,
          page_size: auditPageSize,
        })
      ).data,
    enabled,
    staleTime: 15_000,
  });
  return { me, keys, requests, audit };
}

export function useDeveloperAccessActions(workspaceId?: string) {
  const t = useT('apikeys.developerAccess.feedback');
  const createKey = useWorkspaceMutation(
    workspaceId,
    async (input: CreatePersonalApiKeyInput) =>
      (await developerAccessService.createKey(workspaceId ?? '', input)).data,
    t('keyCreated'),
    t('requestFailed')
  );
  const setKeyStatus = useWorkspaceMutation(
    workspaceId,
    async (input: { keyId: string; action: 'enable' | 'disable' | 'revoke'; reason?: string }) =>
      (
        await developerAccessService.setKeyStatus(
          workspaceId ?? '',
          input.keyId,
          input.action,
          input.reason
        )
      ).data,
    t('keyUpdated'),
    t('requestFailed')
  );
  const rotateKey = useWorkspaceMutation(
    workspaceId,
    async (input: { keyId: string; name?: string }) =>
      (await developerAccessService.rotateKey(workspaceId ?? '', input.keyId, input.name)).data,
    t('keyRotated'),
    t('requestFailed')
  );
  const createRequest = useWorkspaceMutation(
    workspaceId,
    async (input: CreateAccessRequestInput) =>
      (await developerAccessService.createRequest(workspaceId ?? '', input)).data,
    t('requestSubmitted'),
    t('requestFailed')
  );
  const cancelRequest = useWorkspaceMutation(
    workspaceId,
    async (input: { requestId: string }) =>
      (await developerAccessService.cancelRequest(workspaceId ?? '', input.requestId)).data,
    t('requestCancelled'),
    t('requestFailed')
  );
  const reviewRequest = useWorkspaceMutation(
    workspaceId,
    async (input: {
      requestId: string;
      decision: 'approve' | 'reject';
      review: ReviewAccessRequestInput;
    }) =>
      (
        await developerAccessService.reviewRequest(
          workspaceId ?? '',
          input.requestId,
          input.decision,
          input.review
        )
      ).data,
    t('requestUpdated'),
    t('requestFailed')
  );
  const updatePolicy = useWorkspaceMutation(
    workspaceId,
    async (
      input: Omit<DeveloperAccessPolicy, 'id' | 'workspace_id' | 'organization_id' | 'version'>
    ) => (await developerAccessService.updatePolicy(workspaceId ?? '', input)).data,
    t('policyUpdated'),
    t('requestFailed')
  );

  return {
    createKey,
    setKeyStatus,
    rotateKey,
    createRequest,
    cancelRequest,
    reviewRequest,
    updatePolicy,
  };
}
