'use client';

import { useQuery } from '@tanstack/react-query';
import { toolService } from '@/services/tool.service';

/* -------------------------------------------------------------------------- */
/* Query-key helpers                                                          */
/* -------------------------------------------------------------------------- */

const TOOLS_QUERY_KEY = 'tools';
const getBuiltinToolsKey = () => [TOOLS_QUERY_KEY, 'builtin', 'workflow'] as const;

/* -------------------------------------------------------------------------- */
/* Hook: useBuiltinTools                                                      */
/* -------------------------------------------------------------------------- */

export interface UseBuiltinToolsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

/**
 * Fetch tool providers available in the current organization.
 * Uses React Query for caching and background refetching
 */
export function useBuiltinTools(options: UseBuiltinToolsOptions = {}) {
  const query = useQuery({
    queryKey: getBuiltinToolsKey(),
    queryFn: async () => toolService.getBuiltinTools('workflow'),
    select: resp => resp.data,
    enabled: options.enabled ?? true,
    staleTime: options.staleTime ?? 10 * 60 * 1000, // 10 minutes
    gcTime: options.gcTime ?? 30 * 60 * 1000, // 30 minutes
    refetchOnWindowFocus: options.refetchOnWindowFocus ?? false,
    refetchInterval: options.refetchInterval ?? false,
    retry: false,
  });

  return {
    tools: query.data,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    isError: query.isError,
    error: query.error,
    refetch: query.refetch,
  };
}
