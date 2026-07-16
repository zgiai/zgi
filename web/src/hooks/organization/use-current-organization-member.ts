'use client';

import { useQuery } from '@tanstack/react-query';
import { ORGANIZATION_KEYS } from '@/hooks/query-keys';
import { organizationService } from '@/services/organization.service';

interface UseCurrentOrganizationMemberOptions {
  enabled?: boolean;
}

export function useCurrentOrganizationMember(
  memberId: string | null | undefined,
  options: UseCurrentOrganizationMemberOptions = {}
) {
  const normalizedMemberId = memberId?.trim() || null;
  const { enabled = true } = options;

  const query = useQuery({
    queryKey: ORGANIZATION_KEYS.currentMember(normalizedMemberId),
    queryFn: () => organizationService.getCurrentOrganizationMember(normalizedMemberId ?? ''),
    enabled: enabled && Boolean(normalizedMemberId),
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    retry: false,
  });

  return {
    member: query.data ?? null,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: query.error,
    refetch: query.refetch,
  };
}
