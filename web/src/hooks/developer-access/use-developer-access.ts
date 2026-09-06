'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import {
  denormalizeDeveloperAccessPolicy,
  denormalizeDeveloperAccessRequest,
  denormalizeDeveloperAccessReview,
  normalizeDeveloperAccessAudit,
  normalizeDeveloperAccessMe,
  normalizeDeveloperAccessPolicy,
  normalizeDeveloperAccessRequest,
  normalizeDeveloperAccessRequests,
} from '@/utils/ai-credits';
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
  requests: (
    workspaceId: string,
    scope: 'mine' | 'members',
    page: number,
    pageSize: number,
    status?: AccessRequestStatus
  ) =>
    [
      ...DEVELOPER_ACCESS_KEYS.workspace(workspaceId),
      'requests',
      scope,
      page,
      pageSize,
      status ?? 'all',
    ] as const,
  keys: (workspaceId: string, scope: 'mine' | 'members', page: number, pageSize: number) =>
    [...DEVELOPER_ACCESS_KEYS.workspace(workspaceId), 'keys', scope, page, pageSize] as const,
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

export function useDeveloperAccess(
  workspaceId?: string,
  keyPage = 1,
  memberKeyPage = 1,
  requestPage = 1,
  memberRequestPage = 1,
  auditPage = 1,
  keyPageSize = 20,
  requestPageSize = 20,
  auditPageSize = 50
) {
  const queryClient = useQueryClient();
  const enabled = Boolean(workspaceId);
  const me = useQuery({
    queryKey: DEVELOPER_ACCESS_KEYS.me(workspaceId ?? ''),
    queryFn: async () => (await developerAccessService.getMe(workspaceId ?? '')).data,
    select: normalizeDeveloperAccessMe,
    enabled,
    staleTime: 30_000,
  });
  const keys = useQuery({
    queryKey: DEVELOPER_ACCESS_KEYS.keys(workspaceId ?? '', 'mine', keyPage, keyPageSize),
    queryFn: async () =>
      (
        await developerAccessService.listKeys(workspaceId ?? '', {
          scope: 'mine',
          page: keyPage,
          page_size: keyPageSize,
        })
      ).data,
    enabled,
    staleTime: 15_000,
  });
  const memberKeys = useQuery({
    queryKey: DEVELOPER_ACCESS_KEYS.keys(workspaceId ?? '', 'members', memberKeyPage, keyPageSize),
    queryFn: async () =>
      (
        await developerAccessService.listKeys(workspaceId ?? '', {
          scope: 'members',
          page: memberKeyPage,
          page_size: keyPageSize,
        })
      ).data,
    enabled: enabled && me.data?.can_manage === true,
    staleTime: 15_000,
  });
  const requests = useQuery({
    select: normalizeDeveloperAccessRequests,
    queryKey: DEVELOPER_ACCESS_KEYS.requests(
      workspaceId ?? '',
      'mine',
      requestPage,
      requestPageSize
    ),
    queryFn: async () =>
      (
        await developerAccessService.listRequests(workspaceId ?? '', {
          scope: 'mine',
          page: requestPage,
          page_size: requestPageSize,
        })
      ).data,
    enabled,
    staleTime: 15_000,
  });
  const memberRequests = useQuery({
    select: normalizeDeveloperAccessRequests,
    queryKey: DEVELOPER_ACCESS_KEYS.requests(
      workspaceId ?? '',
      'members',
      memberRequestPage,
      requestPageSize
    ),
    queryFn: async () =>
      (
        await developerAccessService.listRequests(workspaceId ?? '', {
          scope: 'members',
          page: memberRequestPage,
          page_size: requestPageSize,
        })
      ).data,
    enabled: enabled && me.data?.can_manage === true,
    staleTime: 15_000,
  });
  const audit = useQuery({
    select: normalizeDeveloperAccessAudit,
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
  // Only mounted, enabled queries are refreshed. Disabled administrator queries
  // must not be triggered when an ordinary member refreshes this page.
  const refresh = async () => {
    if (!workspaceId) return;
    await queryClient.refetchQueries({
      queryKey: DEVELOPER_ACCESS_KEYS.workspace(workspaceId),
      type: 'active',
    });
  };
  const isRefreshing = [me, keys, memberKeys, requests, memberRequests, audit].some(
    query => query.isFetching
  );
  return { me, keys, memberKeys, requests, memberRequests, audit, refresh, isRefreshing };
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
      normalizeDeveloperAccessRequest(
        (
          await developerAccessService.createRequest(
            workspaceId ?? '',
            denormalizeDeveloperAccessRequest(input)
          )
        ).data
      ),
    t('requestSubmitted'),
    t('requestFailed')
  );
  const cancelRequest = useWorkspaceMutation(
    workspaceId,
    async (input: { requestId: string }) =>
      normalizeDeveloperAccessRequest(
        (await developerAccessService.cancelRequest(workspaceId ?? '', input.requestId)).data
      ),
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
      normalizeDeveloperAccessRequest(
        (
          await developerAccessService.reviewRequest(
            workspaceId ?? '',
            input.requestId,
            input.decision,
            denormalizeDeveloperAccessReview(input.review)
          )
        ).data
      ),
    t('requestUpdated'),
    t('requestFailed')
  );
  const updatePolicy = useWorkspaceMutation(
    workspaceId,
    async (
      input: Omit<DeveloperAccessPolicy, 'id' | 'workspace_id' | 'organization_id' | 'version'>
    ) =>
      normalizeDeveloperAccessPolicy(
        (
          await developerAccessService.updatePolicy(
            workspaceId ?? '',
            denormalizeDeveloperAccessPolicy(input)
          )
        ).data
      ),
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
