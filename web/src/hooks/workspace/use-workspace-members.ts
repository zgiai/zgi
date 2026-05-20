'use client';

import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import type { WorkspaceMemberAccount } from '@/services/types/workspace';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useCurrentWorkspace } from '@/store/workspace-store';

import { WORKSPACE_KEYS } from '@/hooks/query-keys';

/**
 * Get query key for workspace members
 */
export const getWorkspaceMembersQueryKey = (
  organizationId: string | null,
  workspaceId: string | null,
  params?: { keyword?: string; page?: number; limit?: number }
) => WORKSPACE_KEYS.members(organizationId, workspaceId, params);

interface UseWorkspaceMembersOptions {
  enabled?: boolean;
  page?: number;
  limit?: number;
  keepPreviousData?: boolean;
}

/**
 * Hook for fetching workspace members
 */
export function useWorkspaceMembers(
  orgId?: string | null,
  wsId?: string | null,
  options: UseWorkspaceMembersOptions & { keyword?: string } = {}
) {
  const { enabled = true, keyword, page = 1, limit = 20, keepPreviousData: keepData } = options;

  const { currentOrganization } = useOrganizations();
  const currentWorkspace = useCurrentWorkspace();

  const organizationId = orgId ?? currentOrganization?.id ?? null;
  const workspaceId = wsId ?? currentWorkspace?.id ?? null;

  const {
    data: responseData,
    isLoading,
    isFetching,
    isPlaceholderData,
    error,
    refetch,
  } = useQuery({
    queryKey: WORKSPACE_KEYS.members(organizationId, workspaceId, { keyword, page, limit }),
    queryFn: async () => {
      if (!organizationId || !workspaceId) {
        return { data: [], total: 0, has_more: false, page, limit };
      }
      return await workspaceService.getWorkspaceMembers(organizationId, workspaceId, {
        keyword,
        page,
        limit,
      });
    },
    enabled: enabled && !!organizationId && !!workspaceId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
    placeholderData: keepData ? keepPreviousData : undefined,
  });

  return {
    members: (responseData?.data ?? []) as WorkspaceMemberAccount[],
    total: responseData?.total ?? 0,
    page: responseData?.page ?? page,
    limit: responseData?.limit ?? limit,
    hasMore: responseData?.has_more ?? false,
    isLoading,
    isFetching,
    isPlaceholderData,
    error,
    refetch,
  };
}

/**
 * Hook for fetching workspace members with infinite pagination.
 */
export function useWorkspaceMembersInfinite(
  orgId?: string | null,
  wsId?: string | null,
  options: UseWorkspaceMembersOptions & { keyword?: string } = {}
) {
  const { enabled = true, keyword, limit = 20 } = options;

  const { currentOrganization } = useOrganizations();
  const currentWorkspace = useCurrentWorkspace();

  const organizationId = orgId ?? currentOrganization?.id ?? null;
  const workspaceId = wsId ?? currentWorkspace?.id ?? null;
  const normalizedKeyword = keyword?.trim() || undefined;

  const query = useInfiniteQuery({
    queryKey: WORKSPACE_KEYS.membersInfinite(organizationId, workspaceId, {
      keyword: normalizedKeyword,
      limit,
    }),
    queryFn: async ({ pageParam }) => {
      const page = typeof pageParam === 'number' ? pageParam : 1;
      if (!organizationId || !workspaceId) {
        return { data: [], total: 0, has_more: false, page, limit };
      }
      return await workspaceService.getWorkspaceMembers(organizationId, workspaceId, {
        keyword: normalizedKeyword,
        page,
        limit,
      });
    },
    initialPageParam: 1,
    getNextPageParam: lastPage => (lastPage.has_more ? lastPage.page + 1 : undefined),
    enabled: enabled && !!organizationId && !!workspaceId,
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  const members = query.data?.pages.flatMap(page => page.data) ?? [];
  const lastPage = query.data?.pages.at(-1);

  return {
    members: members as WorkspaceMemberAccount[],
    total: lastPage?.total ?? 0,
    hasMore: Boolean(query.hasNextPage),
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    isFetchingNextPage: query.isFetchingNextPage,
    error: query.error,
    fetchNextPage: query.fetchNextPage,
    refetch: query.refetch,
  };
}
