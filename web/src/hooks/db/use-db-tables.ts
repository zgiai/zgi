'use client';

// DB table hooks powered by React Query
// All comments are in English for clarity and maintainability

import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { dbService } from '@/services';
import type { ApiResponseData } from '@/services/types/common';
import type { DbTable, CreateDbTableRequest, UpdateDbTableRequest } from '@/services/types/db';
import { getErrorMessage } from '@/utils/error-notifications';
import { DB_KEYS } from '@/hooks/query-keys';
import { workspaceInvalidatePredicate } from '@/hooks/query-utils';
import { useCurrentWorkspace } from '@/store/workspace-store';

// Local query-key helpers are now centralized in DB_KEYS
const getDbTablesKey = (dbId: string) => DB_KEYS.tableList(dbId, {});
const getDbTableDetailKey = (dbId: string, id: string) => DB_KEYS.tableDetail(dbId, id);

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseDbTablesOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

export interface UseDbTablesReturn {
  tables: DbTable[];
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<unknown>;
}

/* -------------------------------------------------------------------------- */
/* Hook: useDbTables – list all tables in DB                                  */
/* -------------------------------------------------------------------------- */

export function useDbTables(
  dbId: string | undefined,
  {
    enabled = true,
    staleTime = 3 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
  }: UseDbTablesOptions = {}
): UseDbTablesReturn {
  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<DbTable[]>,
    unknown
  >({
    queryKey: dbId ? DB_KEYS.tableList(dbId, {}) : DB_KEYS.tables('undefined'),
    queryFn: () => {
      if (!dbId) throw new Error('dbId is required');
      return dbService.getDbTables(dbId);
    },
    enabled: Boolean(dbId) && enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
    retry: false,
  });

  if (error) {
    const message = getErrorMessage(error);
    toast.error(message || 'Failed to load tables');
  }

  return {
    tables: data?.data ?? [],
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch,
  };
}

/* -------------------------------------------------------------------------- */
/* Hook: useDbTableDetail – table detail                                      */
/* -------------------------------------------------------------------------- */

export function useDbTableDetail(
  dbId: string | undefined,
  id: string | undefined,
  options: Omit<UseQueryOptions<ApiResponseData<DbTable>, unknown>, 'queryKey' | 'queryFn'> = {}
) {
  return useQuery<ApiResponseData<DbTable>, unknown>({
    queryKey:
      dbId && id ? DB_KEYS.tableDetail(dbId, id) : DB_KEYS.tableDetail('undefined', 'undefined'),
    queryFn: () => {
      if (!dbId) throw new Error('dbId is required');
      if (!id) throw new Error('id is required');
      return dbService.getDbTableDetail(dbId, id);
    },
    enabled: Boolean(dbId) && typeof id === 'string' && (options.enabled ?? true),
    ...options,
  });
}

/* -------------------------------------------------------------------------- */
/* Mutations: create/update/delete with optimistic updates                    */
/* -------------------------------------------------------------------------- */

export function useCreateDbTable(dbId: string | undefined) {
  const queryClient = useQueryClient();
  const t = useT('dbs');
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  return useMutation({
    mutationFn: (payload: CreateDbTableRequest) => {
      if (!dbId) return Promise.reject(new Error('dbId is required'));
      return dbService.createDbTable(dbId, payload);
    },
    onMutate: async payload => {
      if (!dbId) return;
      await queryClient.cancelQueries({ queryKey: getDbTablesKey(dbId) });
      const previous = queryClient.getQueryData<ApiResponseData<DbTable[]>>(getDbTablesKey(dbId));

      const optimisticItem: DbTable = {
        id: `optimistic-${Date.now()}`,
        organization_id: '',
        data_source_id: '',
        name: payload.name,
        // Stop using table_id; keep field only for type compatibility if present
        table_id: 0,
        table_name: '',
        description: payload.description,
        created_by: '',
        updated_by: '',
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };

      queryClient.setQueryData<ApiResponseData<DbTable[]>>(getDbTablesKey(dbId), old => {
        const current = old?.data ?? [];
        return { code: old?.code, message: old?.message, data: [optimisticItem, ...current] };
      });

      return { previous };
    },
    onSuccess: response => {
      if (!dbId) return;
      // Replace optimistic item with server item if present
      queryClient.setQueryData<ApiResponseData<DbTable[]>>(getDbTablesKey(dbId), old => {
        const serverItem = response?.data;
        if (!old || !serverItem) return old;
        const updated = [
          serverItem,
          ...(old.data || []).filter(i => !String(i.id).startsWith('optimistic-')),
        ];
        return { code: old.code, message: old.message, data: updated };
      });
      toast.success(t('tableCreateSuccess', { defaultMessage: 'Table created' }));
      // Standardize invalidation
      queryClient.invalidateQueries({
        queryKey: DB_KEYS.all,
        predicate: workspaceInvalidatePredicate(DB_KEYS.all[0], currentWorkspaceId),
      });
    },
    onError: (error, _payload, context) => {
      const msg = getErrorMessage(error);
      toast.error(msg || t('failed', { defaultMessage: 'Operation failed' }));
      if (!dbId) return;
      // Rollback
      queryClient.setQueryData(getDbTablesKey(dbId), context?.previous);
    },
    onSettled: () => {
      if (!dbId) return;
      queryClient.invalidateQueries({ queryKey: getDbTablesKey(dbId) });
    },
  });
}

export function useUpdateDbTable(dbId: string | undefined, id: string | undefined) {
  const queryClient = useQueryClient();
  const t = useT('dbs');
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  return useMutation({
    mutationFn: (payload: UpdateDbTableRequest) => {
      if (!dbId) return Promise.reject(new Error('dbId is required'));
      if (!id) return Promise.reject(new Error('id is required'));
      return dbService.updateDbTable(dbId, id, payload);
    },
    onMutate: async payload => {
      if (!dbId || !id) return;
      await queryClient.cancelQueries({ queryKey: getDbTablesKey(dbId) });
      await queryClient.cancelQueries({ queryKey: getDbTableDetailKey(dbId, id) });
      const previousList = queryClient.getQueryData<ApiResponseData<DbTable[]>>(
        getDbTablesKey(dbId)
      );
      const previousDetail = queryClient.getQueryData<ApiResponseData<DbTable>>(
        getDbTableDetailKey(dbId, id)
      );

      // Optimistically update list item
      queryClient.setQueryData<ApiResponseData<DbTable[]>>(getDbTablesKey(dbId), old => {
        if (!old) return old;
        const updated = (old.data || []).map(item =>
          item.id === id
            ? {
                ...item,
                name: payload.name ?? item.name,
                description: payload.description ?? item.description,
              }
            : item
        );
        return { code: old.code, message: old.message, data: updated };
      });

      // Optimistically update detail
      queryClient.setQueryData<ApiResponseData<DbTable>>(getDbTableDetailKey(dbId, id), old => {
        if (!old) return old;
        return {
          code: old.code,
          message: old.message,
          data: {
            ...old.data,
            name: payload.name ?? old.data.name,
            description: payload.description ?? old.data.description,
          },
        };
      });

      return { previousList, previousDetail };
    },
    onSuccess: () => {
      if (!dbId || !id) return;
      toast.success(t('tableUpdateSuccess', { defaultMessage: 'Table updated' }));
      // Invalidate both paginated and basic lists that are either unfiltered or belong to current workspace
      queryClient.invalidateQueries({
        queryKey: DB_KEYS.all,
        predicate: workspaceInvalidatePredicate(DB_KEYS.all[0], currentWorkspaceId),
      });
    },
    onError: (error, _payload, context) => {
      const msg = getErrorMessage(error);
      toast.error(msg || t('failed', { defaultMessage: 'Operation failed' }));
      if (!dbId || !id) return;
      // Rollback
      if (context?.previousList) {
        queryClient.setQueryData(getDbTablesKey(dbId), context.previousList);
      }
      if (context?.previousDetail) {
        queryClient.setQueryData(getDbTableDetailKey(dbId, id), context.previousDetail);
      }
    },
    onSettled: () => {
      if (!dbId || !id) return;
      queryClient.invalidateQueries({ queryKey: getDbTablesKey(dbId) });
      queryClient.invalidateQueries({ queryKey: getDbTableDetailKey(dbId, id) });
    },
  });
}

export function useDeleteDbTable(dbId: string | undefined) {
  const queryClient = useQueryClient();
  const t = useT('dbs');
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  return useMutation<ApiResponseData<{ result: 'success' | 'fail' }>, unknown, string>({
    mutationFn: (id: string) => {
      if (!dbId) return Promise.reject(new Error('dbId is required'));
      return dbService.deleteDbTable(dbId, id);
    },
    onMutate: async id => {
      if (!dbId) return;
      await queryClient.cancelQueries({ queryKey: getDbTablesKey(dbId) });
      const previous = queryClient.getQueryData<ApiResponseData<DbTable[]>>(getDbTablesKey(dbId));
      queryClient.setQueryData<ApiResponseData<DbTable[]>>(getDbTablesKey(dbId), old => {
        if (!old) return old;
        const updated = (old.data || []).filter(item => item.id !== id);
        return { code: old.code, message: old.message, data: updated };
      });
      return { previous };
    },
    onSuccess: (_res, id) => {
      if (!dbId) return;
      // Remove detail cache too
      queryClient.removeQueries({ queryKey: DB_KEYS.tableDetail(dbId, id) });
      // Standardize invalidation
      queryClient.invalidateQueries({
        queryKey: DB_KEYS.all,
        predicate: workspaceInvalidatePredicate(DB_KEYS.all[0], currentWorkspaceId),
      });
      toast.success(t('tableDeleteSuccess', { defaultMessage: 'Table deleted' }));
    },
    onError: (error, _tableId, context) => {
      const msg = getErrorMessage(error);
      toast.error(msg || t('failed', { defaultMessage: 'Operation failed' }));
      if (!dbId) return;
      // Rollback
      queryClient.setQueryData(
        getDbTablesKey(dbId),
        (context as { previous?: ApiResponseData<DbTable[]> })?.previous
      );
    },
    onSettled: () => {
      if (!dbId) return;
      queryClient.invalidateQueries({ queryKey: getDbTablesKey(dbId) });
    },
  });
}
