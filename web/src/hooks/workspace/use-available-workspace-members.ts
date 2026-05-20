'use client';

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { toast } from 'sonner';

import { useT } from '@/i18n';
import { WORKSPACE_KEYS } from '@/hooks/query-keys';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { workspaceService } from '@/services/workspace.service';
import type {
  AvailableWorkspaceMember,
  GetAvailableWorkspaceMembersParams,
} from '@/services/types/workspace';
import { getErrorMessage } from '@/utils/error-notifications';

interface UseAvailableWorkspaceMembersOptions extends GetAvailableWorkspaceMembersParams {
  enabled?: boolean;
}

/**
 * Hook for fetching members that can be added to a workspace.
 */
export function useAvailableWorkspaceMembers(
  workspaceId?: string | null,
  options: UseAvailableWorkspaceMembersOptions = {}
): {
  members: AvailableWorkspaceMember[];
  total: number;
  page: number;
  limit: number;
  hasMore: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<unknown>;
} {
  const {
    enabled = true,
    department_id,
    include_sub_depts,
    keyword,
    page = 1,
    limit = 20,
  } = options;
  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();

  const organizationId = currentOrganization?.id ?? null;
  const normalizedKeyword = keyword?.trim() || undefined;
  const normalizedDepartmentId = department_id?.trim() || undefined;
  const params: GetAvailableWorkspaceMembersParams = {
    department_id: normalizedDepartmentId,
    include_sub_depts,
    keyword: normalizedKeyword,
    page,
    limit,
  };

  const { data, isLoading, isFetching, error, refetch } = useQuery({
    queryKey: WORKSPACE_KEYS.availableMembers(organizationId, workspaceId ?? null, params),
    queryFn: async () => {
      if (!organizationId || !workspaceId) {
        return { data: [], total: 0, page, limit, has_more: false };
      }
      return await workspaceService.getAvailableMembers(organizationId, workspaceId, params);
    },
    enabled: enabled && !!organizationId && !!workspaceId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  useEffect(() => {
    if (!error) return;
    toast.error(
      getErrorMessage(error) || t('organization.workspaceManagement.detail.membersAddError')
    );
  }, [error, t]);

  return {
    members: data?.data ?? [],
    total: data?.total ?? 0,
    page: data?.page ?? page,
    limit: data?.limit ?? limit,
    hasMore: data?.has_more ?? false,
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch,
  };
}
