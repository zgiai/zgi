import { Query, QueryClient, QueryKey } from '@tanstack/react-query';

/**
 * Common predicate for invalidating queries that belong to a specific workspace.
 * Matches if the query key starts with the domain and contains the workspaceId in its params object.
 *
 * @param domainKey - The root string of the query key (e.g., 'agents', 'datasets')
 * @param currentWorkspaceId - The ID of the currently active workspace
 * @returns Invalidation predicate function
 */
export const workspaceInvalidatePredicate = (
  domainKey: string,
  currentWorkspaceId: string | undefined
) => {
  return (query: Query<any, any, any, QueryKey>) => {
    const key = query.queryKey;
    if (key[0] !== domainKey) return false;

    // Detail queries usually shouldn't be invalidated by list changes
    // But we check if the second element is 'detail' to safely exclude them if desired
    if (key[1] === 'detail') return false;

    // Find params object in the query key array
    const params = key.find(k => typeof k === 'object' && k !== null) as any;
    const wId = params?.workspace_id || params?.workspaceId;

    // Invalidate if it's the current workspace or if no workspace filter is applied (global list)
    return !wId || wId === currentWorkspaceId;
  };
};

/**
 * Standard reload for infinite queries.
 * Removes queries and refetches to reset to page 1.
 */
export const reloadInfiniteQuery = async (queryClient: QueryClient, queryKey: QueryKey) => {
  queryClient.removeQueries({ queryKey });
  await queryClient.refetchQueries({ queryKey });
};

/**
 * Helpers for complex infinite query manipulations
 */
export const infiniteQueryUtils = {
  /**
   * Refetch from a specific page onwards by trimming the cache.
   * Useful when an item is deleted from a middle page.
   */
  refetchFromPage: async (
    queryClient: QueryClient,
    queryKey: QueryKey,
    startIndex: number,
    fetchNextPage: () => Promise<any>
  ) => {
    const cached = queryClient.getQueryData<{
      pages: any[];
      pageParams: any[];
    }>(queryKey);

    if (cached && Array.isArray(cached.pages)) {
      // Keep pages BEFORE the startIndex
      const keepCount = Math.max(0, startIndex);
      const nextPages = cached.pages.slice(0, keepCount);
      const nextParams = cached.pageParams.slice(0, keepCount);
      queryClient.setQueryData(queryKey, { pages: nextPages, pageParams: nextParams });
    }

    // Trigger fetch for the next page (which is now the one at startIndex)
    await fetchNextPage();
  },
};
