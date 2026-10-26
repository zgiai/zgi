'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useRef } from 'react';
import {
  socialOperationsQueryKey,
  type SocialOperationsScopeKey,
} from '@/components/digital-human/digital-human-social-operations-model';
import { digitalHumanService } from '@/services/digital-human.service';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import type {
  SaveSocialOperationsConfigurationRequest,
  SetupSocialOperationsRequest,
} from '@/services/types/social-operations';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useWorkspaceStore } from '@/store/workspace-store';

const disabledSocialOperationsKey = ['digital-humans', 'social-operations', 'disabled'] as const;

function currentScope(employeeId: string, versionId: string): SocialOperationsScopeKey | undefined {
  const accountId = useAuthStore.getState().user?.id;
  const organizationId = useOrganizationStore.getState().currentOrganization?.id;
  const workspaceId = useWorkspaceStore.getState().currentWorkspace?.id;
  return accountId && organizationId && workspaceId
    ? { accountId, organizationId, workspaceId, employeeId, versionId }
    : undefined;
}

function sameScope(left: SocialOperationsScopeKey, right: SocialOperationsScopeKey | undefined) {
  return Boolean(
    right &&
      left.accountId === right.accountId &&
      left.organizationId === right.organizationId &&
      left.workspaceId === right.workspaceId &&
      left.employeeId === right.employeeId &&
      left.versionId === right.versionId
  );
}

export function useSocialOperations(
  employeeId: string,
  versionId: string | undefined,
  enabled = true
) {
  const queryClient = useQueryClient();
  const account = useAuthStore.use.user();
  const organization = useOrganizationStore.use.currentOrganization();
  const workspace = useWorkspaceStore.use.currentWorkspace();
  const scope =
    account?.id && organization?.id && workspace?.id && versionId
      ? {
          accountId: account.id,
          organizationId: organization.id,
          workspaceId: workspace.id,
          employeeId,
          versionId,
        }
      : undefined;
  const queryKey = scope ? socialOperationsQueryKey(scope) : disabledSocialOperationsKey;
  const activeScope = useRef(scope);
  activeScope.current = scope;
  const scopeIsCurrent = () => {
    if (!scope || !sameScope(scope, activeScope.current)) return false;
    if (!sameScope(scope, currentScope(employeeId, versionId ?? ''))) return false;
    const detail = queryClient.getQueryData<{ data?: { published?: { id?: string } } }>(
      DIGITAL_HUMAN_KEYS.detail(employeeId)
    );
    return detail?.data?.published?.id === scope.versionId;
  };
  const query = useQuery({
    queryKey,
    queryFn: async () => {
      if (!scope) throw new Error('social operations scope is unavailable');
      const response = await digitalHumanService.getSocialOperationsConfiguration(employeeId);
      if (!sameScope(scope, currentScope(employeeId, versionId ?? ''))) {
        throw new Error('social operations scope changed');
      }
      return response;
    },
    enabled: enabled && Boolean(scope),
    staleTime: 10_000,
    retry: false,
  });
  const accounts = useQuery({
    queryKey: [...queryKey, 'accounts'],
    queryFn: async () => {
      if (!scopeIsCurrent()) throw new Error('social operations scope changed');
      const response = await digitalHumanService.getSocialOperationsAccounts(employeeId);
      if (!scopeIsCurrent()) throw new Error('social operations scope changed');
      return response;
    },
    enabled: enabled && Boolean(scope),
    staleTime: 10_000,
    retry: false,
  });
  const save = useMutation({
    mutationFn: async (request: SaveSocialOperationsConfigurationRequest) => {
      if (!scopeIsCurrent()) throw new Error('social operations scope changed');
      const response = await digitalHumanService.saveSocialOperationsConfiguration(
        employeeId,
        request
      );
      if (
        !scopeIsCurrent() ||
        response.data.configuration?.employee_version_id !== scope?.versionId
      ) {
        throw new Error('social operations scope changed');
      }
      return response;
    },
    retry: false,
    onSuccess: response => {
      if (scopeIsCurrent()) {
        queryClient.setQueryData(queryKey, response);
      }
    },
  });
  const preflight = useMutation({
    mutationFn: async () => {
      if (!scopeIsCurrent()) throw new Error('social operations scope changed');
      const response = await digitalHumanService.preflightSocialOperations(employeeId);
      if (!scopeIsCurrent() || response.data.published_employee_version_id !== scope?.versionId) {
        throw new Error('social operations scope changed');
      }
      return response;
    },
    retry: false,
  });
  const setup = useMutation({
    mutationFn: async (request: SetupSocialOperationsRequest) => {
      if (!scopeIsCurrent()) throw new Error('social operations scope changed');
      const response = await digitalHumanService.setupSocialOperations(employeeId, request);
      if (
        !scopeIsCurrent() ||
        response.data.preflight.published_employee_version_id !== scope?.versionId
      ) {
        throw new Error('social operations scope changed');
      }
      return response;
    },
    retry: false,
    onSuccess: () => {
      if (!scopeIsCurrent()) return;
      void queryClient.invalidateQueries({
        queryKey: ['digital-humans', 'detail', employeeId, 'tasks'],
      });
    },
  });
  const uploadImage = useMutation({
    mutationFn: async (file: File) => {
      if (!scopeIsCurrent()) throw new Error('social operations scope changed');
      const response = await digitalHumanService.uploadSocialOperationsImage(employeeId, file);
      if (!scopeIsCurrent()) throw new Error('social operations scope changed');
      return response;
    },
    retry: false,
  });
  return { scope, queryKey, query, accounts, save, preflight, setup, uploadImage };
}
