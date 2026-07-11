'use client';

import { useQuery } from '@tanstack/react-query';
import { ORGANIZATION_KEYS } from '@/hooks/query-keys';
import { organizationService } from '@/services/organization.service';
import { useOrganizations } from '@/hooks/organization/use-organizations';

interface UseCurrentOrganizationMembersOptions {
  keyword?: string;
  page?: number;
  limit?: number;
  enabled?: boolean;
}

export function useCurrentOrganizationMembers(options: UseCurrentOrganizationMembersOptions = {}) {
  const { keyword, page = 1, limit = 100, enabled = true } = options;
  const { currentOrganization } = useOrganizations();
  const params = {
    page,
    limit,
    keyword: keyword?.trim() || undefined,
  };

  const query = useQuery({
    queryKey: ORGANIZATION_KEYS.currentMembers({
      organizationId: currentOrganization?.id ?? null,
      ...params,
    }),
    queryFn: () => organizationService.getCurrentOrganizationMembers(params),
    enabled: enabled && !!currentOrganization?.id,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    retry: false,
  });

  return {
    members: query.data?.data ?? [],
    total: query.data?.total ?? 0,
    page: query.data?.page ?? page,
    limit: query.data?.limit ?? limit,
    hasMore: query.data?.has_more ?? false,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: query.error,
    refetch: query.refetch,
  };
}
