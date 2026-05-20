'use client';

import { useEffect, useState, useMemo, useCallback } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { organizationService } from '@/services/organization.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useOrganizations } from './use-organizations';
import type { RoleMemberList } from '@/services/types/organization';

import { ORGANIZATION_KEYS } from '@/hooks/query-keys';

/**
 * Hook for fetching role members with infinite scroll
 */
export function useRoleMembers(roleId: string | null, enabled: boolean = true) {
  const t = useT('dashboard');
  const { currentOrganization } = useOrganizations();
  const [searchKeyword, setSearchKeyword] = useState('');

  const {
    data,
    isLoading,
    isFetching,
    error,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    refetch,
  } = useInfiniteQuery<RoleMemberList, unknown>({
    queryKey: ORGANIZATION_KEYS.roleMembers(currentOrganization?.id || '', roleId || ''),
    initialPageParam: 1,
    queryFn: async ({ pageParam }) => {
      if (!currentOrganization?.id || !roleId) {
        throw new Error('No organization or role selected');
      }
      const page = (pageParam as number) ?? 1;
      const response = await organizationService.getRoleMembers(currentOrganization.id, roleId, {
        page,
        limit: 20,
      });
      return response;
    },
    getNextPageParam: lastPage => {
      if (!lastPage) return undefined;
      return lastPage.has_more ? lastPage.page + 1 : undefined;
    },
    enabled: enabled && !!currentOrganization?.id && !!roleId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  // Show error toast if query fails
  useEffect(() => {
    if (!error) return;
    toast.error(getErrorMessage(error) || t('organization.permissions.loadError'));
  }, [error, t]);

  useEffect(() => {
    if (!searchKeyword.trim() || !hasNextPage || isFetchingNextPage) return;
    fetchNextPage();
  }, [fetchNextPage, hasNextPage, isFetchingNextPage, searchKeyword]);

  // Flatten all pages into single array
  const allMembers = useMemo(() => {
    const pages = data?.pages ?? [];
    return pages.flatMap(page => page?.items ?? []);
  }, [data]);

  // Get total count from first page
  const total = useMemo(() => {
    return data?.pages?.[0]?.total ?? 0;
  }, [data]);

  // Filter members by search keyword
  const filteredMembers = useMemo(() => {
    if (!searchKeyword.trim()) return allMembers;
    const keyword = searchKeyword.toLowerCase();
    return allMembers.filter(
      member =>
        member.name.toLowerCase().includes(keyword) || member.email.toLowerCase().includes(keyword)
    );
  }, [allMembers, searchKeyword]);

  const stableFetchNextPage = useCallback(async () => {
    await fetchNextPage();
  }, [fetchNextPage]);

  return {
    members: filteredMembers,
    total,
    isLoading,
    isFetching,
    hasNextPage: !!hasNextPage,
    fetchNextPage: stableFetchNextPage,
    isFetchingNextPage,
    error: error ? getErrorMessage(error) : null,
    refetch,
    searchKeyword,
    setSearchKeyword,
  };
}
