'use client';

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { organizationService } from '@/services/organization.service';
import type { AllDepartmentMemberList } from '@/services/types/organization';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { ORGANIZATION_KEYS } from '@/hooks/query-keys';

interface UseDepartmentMembersOptions {
  deptId?: string | null;
  keyword?: string;
  page?: number | string;
  limit?: number | string;
  includeSubDepts?: boolean | string;
  enabled?: boolean;
}

/**
 * Hook for fetching department members with pagination and flexible parameters.
 * Consolidates useDepartmentMembers and useDepartmentMembersWithParams.
 */
export function useDepartmentMembers(options: UseDepartmentMembersOptions = {}) {
  const {
    deptId = null,
    keyword,
    page = '1',
    limit = '20',
    includeSubDepts = true,
    enabled = true,
  } = options;

  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();

  // Normalize parameters for the query key and service call
  const params = {
    keyword: keyword || undefined,
    page: page.toString(),
    limit: limit.toString(),
    include_sub_depts: includeSubDepts?.toString(),
    department_id: deptId || undefined,
  };

  const {
    data: responseData,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useQuery<AllDepartmentMemberList>({
    queryKey: ORGANIZATION_KEYS.departmentMembers(currentOrganization?.id || '', deptId || '', {
      ...params,
    }),
    queryFn: async () => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }

      return await organizationService.getDepartmentMembersWithParams(currentOrganization.id, {
        ...params,
        department_id: deptId || undefined,
      });
    },
    enabled: enabled && !!currentOrganization?.id,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  // Show error toast if query fails
  useEffect(() => {
    if (!error) return;
    toast.error(getErrorMessage(error) || t('organization.contacts.loadMembersError'));
  }, [error, t]);

  return {
    members: responseData?.data ?? [],
    total: responseData?.total ?? 0,
    page: Number(responseData?.page || page),
    limit: Number(responseData?.limit || limit),
    hasMore: responseData?.has_more ?? false,
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch,
  };
}
