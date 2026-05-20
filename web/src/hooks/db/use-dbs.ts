'use client';

// DB hooks powered by React Query
// All comments are in English for clarity and maintainability

import { useMemo } from 'react';
import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { dbService } from '@/services';
import { useCurrentWorkspace } from '@/store/workspace-store';
import type { ApiResponseData } from '@/services/types/common';
import type { Db, CreateDbRequest, UpdateDbRequest } from '@/services/types/db';
import { getErrorMessage } from '@/utils/error-notifications';

import { DB_KEYS } from '@/hooks/query-keys';
import { workspaceInvalidatePredicate } from '@/hooks/query-utils';

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseDbsParams {
  limit?: number;
  keyword?: string;
  workspace_id?: string;
}

export interface UseDbsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

export function useDb(
  dbId: string | undefined,
  options: Omit<UseQueryOptions<ApiResponseData<Db>, unknown>, 'queryKey' | 'queryFn'> = {}
) {
  return useQuery<ApiResponseData<Db>, unknown>({
    queryKey: dbId ? DB_KEYS.detail(dbId) : DB_KEYS.detail('undefined'),
    queryFn: () => {
      if (!dbId) throw new Error('dbId is required');
      return dbService.getDb(dbId);
    },
    enabled: Boolean(dbId) && (options.enabled ?? true),
    ...options,
  });
}

/* -------------------------------------------------------------------------- */
/* Mutations: create/update/delete                                            */
/* -------------------------------------------------------------------------- */

export function useCreateDb() {
  const queryClient = useQueryClient();
  const t = useT('dbs');
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  return useMutation({
    mutationFn: (payload: CreateDbRequest) => dbService.createDb(payload),
    onSuccess: () => {
      toast.success(t('createSuccess'));
      // Invalidate both paginated and basic lists that are either unfiltered or belong to current workspace
      queryClient.invalidateQueries({
        queryKey: DB_KEYS.all,
        predicate: workspaceInvalidatePredicate(DB_KEYS.all[0], currentWorkspaceId),
      });
    },
    onError: (error: unknown) => {
      const msg = getErrorMessage(error);
      toast.error(msg || t('failed'));
    },
  });
}

export function useUpdateDb(dbId: string | undefined) {
  const queryClient = useQueryClient();
  const t = useT('dbs');
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  return useMutation({
    mutationFn: (data: UpdateDbRequest) => {
      if (!dbId) return Promise.reject(new Error('dbId is required'));
      return dbService.updateDb(dbId, data);
    },
    onSuccess: () => {
      toast.success(t('updateSuccess'));
      if (dbId) {
        queryClient.invalidateQueries({ queryKey: DB_KEYS.detail(dbId) });
        // Invalidate both paginated and basic lists that are either unfiltered or belong to current workspace
        queryClient.invalidateQueries({
          queryKey: DB_KEYS.all,
          predicate: workspaceInvalidatePredicate(DB_KEYS.all[0], currentWorkspaceId),
        });
      }
    },
    onError: (error: unknown) => {
      const msg = getErrorMessage(error);
      toast.error(msg || t('failed', { defaultMessage: 'Operation failed' }));
    },
  });
}

export function useDeleteDb() {
  const queryClient = useQueryClient();
  const t = useT('dbs');
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  return useMutation<ApiResponseData<{ result: 'success' | 'fail' }>, unknown, string>({
    mutationFn: (dbId: string) => dbService.deleteDb(dbId),
    onSuccess: (_result, dbId) => {
      toast.success(t('deleteSuccess'));
      if (dbId) {
        queryClient.removeQueries({ queryKey: DB_KEYS.detail(dbId) });
        // Invalidate list queries that are either unfiltered or belong to current workspace
        queryClient.invalidateQueries({
          queryKey: DB_KEYS.all,
          predicate: workspaceInvalidatePredicate(DB_KEYS.all[0], currentWorkspaceId),
        });
      }
    },
    onError: (error: unknown) => {
      const message = getErrorMessage(error);
      toast.error(message || t('failed'));
    },
  });
}

/* -------------------------------------------------------------------------- */
/* Hook: useDbsBasic – non-paginated list                                    */
/* -------------------------------------------------------------------------- */

export interface UseDbsBasicReturn {
  dbs: Db[];
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<unknown>;
}

export function useDbsBasic(
  params: Omit<UseDbsParams, 'limit'> = {},
  {
    enabled = true,
    staleTime = 5 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
  }: UseDbsOptions = {}
): UseDbsBasicReturn {
  const normalizedParams = useMemo(
    () => ({ keyword: params.keyword, workspace_id: params.workspace_id }),
    [params.keyword, params.workspace_id]
  );

  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<Db[]>, unknown>({
    queryKey: DB_KEYS.list(normalizedParams),
    queryFn: () => dbService.getDbsBasic(normalizedParams),
    enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
    retry: false,
  });

  if (error) {
    const message = getErrorMessage(error);
    toast.error(message || 'Failed to load databases');
  }

  return {
    dbs: data?.data ?? [],
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch,
  };
}
