'use client';

import { useQuery } from '@tanstack/react-query';
import { organizationService } from '@/services/organization.service';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { ORGANIZATION_KEYS } from '@/hooks/query-keys';

/**
 * Hook for fetching join requests
 */
export function useJoinRequests(page: number = 1, limit: number = 10, enabled: boolean = true) {
  const { currentOrganization } = useOrganizations();

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ORGANIZATION_KEYS.joinRequests(currentOrganization?.id || '', { page, limit }),
    queryFn: async () => {
      if (!currentOrganization?.id) {
        throw new Error('No organization selected');
      }
      return await organizationService.getJoinRequests(currentOrganization.id, {
        page: page.toString(),
        limit: limit.toString(),
      });
    },
    enabled: !!currentOrganization?.id && enabled,
  });

  return {
    data: data?.data || [],
    total: data?.total || 0,
    page: data?.page || page,
    limit: data?.limit || limit,
    hasMore: data?.has_more || false,
    isLoading,
    error,
    refetch,
  };
}
