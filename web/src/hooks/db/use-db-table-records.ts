'use client';

// Table records hooks powered by React Query
// English comments for maintainability and clarity

import { useCallback } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { dbService } from '@/services';
import type { ApiResponseData } from '@/services/types/common';
import type {
  DbTableRecord,
  DbTableRecordsList,
  GetDbTableRecordsParams,
  CreateDbTableRecordsRequest,
  UpdateDbTableRecordsRequest,
} from '@/services/types/db';
import { DB_KEYS } from '@/hooks/query-keys';

// Local query-key helpers are now centralized in DB_KEYS
const getDbTableRecordsKey = (dbId: string, tableId: string, params: GetDbTableRecordsParams) =>
  DB_KEYS.tableRecords(dbId, tableId, params);
const getTableColumnsKeyWithSystem = (dbId: string, tableId: string) =>
  DB_KEYS.tableColumns(dbId, tableId, true);

// Stable empty array to prevent re-render loops on initial load
const EMPTY_RECORDS: DbTableRecord[] = [];

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseDbTableRecordsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

export interface UseDbTableRecordsReturn {
  records: DbTableRecord[];
  total: number;
  hasMore: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<unknown>;
}

/* -------------------------------------------------------------------------- */
/* Hook: useDbTableRecords – fetch data rows                                  */
/* -------------------------------------------------------------------------- */

export function useDbTableRecords(
  dbId: string,
  tableId: string,
  params: GetDbTableRecordsParams,
  options?: UseDbTableRecordsOptions
): UseDbTableRecordsReturn {
  const query = useQuery<ApiResponseData<DbTableRecordsList>, unknown>({
    queryKey: getDbTableRecordsKey(dbId, tableId, params),
    queryFn: () => dbService.getDbTableRecords(dbId, tableId, params),
    enabled: !!dbId && !!tableId && (options?.enabled ?? true),
    staleTime: options?.staleTime ?? 30 * 1000, // 30s
    gcTime: options?.gcTime ?? 10 * 60 * 1000, // 10m
    refetchOnWindowFocus: options?.refetchOnWindowFocus ?? false,
    refetchInterval: options?.refetchInterval ?? false,
    retry: false,
  });

  const payload = query.data?.data;
  return {
    records: payload?.data ?? EMPTY_RECORDS,
    total: payload?.total_num ?? 0,
    hasMore: payload?.has_more ?? false,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: (query.error as Error | null)?.message ?? null,
    refetch: query.refetch,
  };
}

/* -------------------------------------------------------------------------- */
/* Mutations: create/update/delete with optimistic updates                    */
/* -------------------------------------------------------------------------- */

export interface UseCreateDbTableRecordsReturn {
  createRecords: (records: CreateDbTableRecordsRequest['records']) => Promise<void>;
  isPending: boolean;
}

export function useCreateDbTableRecords(
  dbId: string,
  tableId: string,
  paramsForList: GetDbTableRecordsParams
): UseCreateDbTableRecordsReturn {
  const queryClient = useQueryClient();

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<{ result: 'success' | 'fail' }>,
    unknown,
    CreateDbTableRecordsRequest,
    { previous?: DbTableRecordsList }
  >({
    mutationFn: data => dbService.createDbTableRecords(dbId, tableId, data),
    onMutate: async variables => {
      await queryClient.cancelQueries({
        queryKey: getDbTableRecordsKey(dbId, tableId, paramsForList),
      });
      const previousResp = queryClient.getQueryData<ApiResponseData<DbTableRecordsList>>(
        getDbTableRecordsKey(dbId, tableId, paramsForList)
      );
      const previous = previousResp?.data;

      const incoming = variables.records.map(r => ({ ...r, id: Number(-Date.now()) }));
      const optimistic: DbTableRecordsList = {
        has_more: previous?.has_more ?? false,
        total_num: (previous?.total_num ?? 0) + incoming.length,
        data: [...(previous?.data ?? []), ...incoming],
      };

      queryClient.setQueryData<ApiResponseData<DbTableRecordsList>>(
        getDbTableRecordsKey(dbId, tableId, paramsForList),
        { code: '0', message: 'optimistic', data: optimistic }
      );

      return { previous };
    },
    onError: (err, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData<ApiResponseData<DbTableRecordsList>>(
          getDbTableRecordsKey(dbId, tableId, paramsForList),
          { code: '0', message: 'rollback', data: context.previous }
        );
      }
      // No toast here; centralized error handling in table-data component
    },
    onSuccess: async _resp => {
      // Fetch authoritative list to replace optimistic items
      const fetched = await dbService.getDbTableRecords(dbId, tableId, paramsForList);
      queryClient.setQueryData<ApiResponseData<DbTableRecordsList>>(
        getDbTableRecordsKey(dbId, tableId, paramsForList),
        fetched
      );
      queryClient.invalidateQueries({
        queryKey: getTableColumnsKeyWithSystem(dbId, tableId),
      });
      // No success toast here; centralized success handling in table-data component
    },
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: getDbTableRecordsKey(dbId, tableId, paramsForList),
      });
    },
  });

  const createRecords = useCallback(
    async (records: CreateDbTableRecordsRequest['records']) => {
      await mutateAsync({ records });
    },
    [mutateAsync]
  );

  return { createRecords, isPending };
}

export interface UseUpdateDbTableRecordsReturn {
  updateRecords: (records: UpdateDbTableRecordsRequest['records']) => Promise<void>;
  isPending: boolean;
}

export function useUpdateDbTableRecords(
  dbId: string,
  tableId: string,
  paramsForList: GetDbTableRecordsParams
): UseUpdateDbTableRecordsReturn {
  const queryClient = useQueryClient();

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<{ result: 'success' | 'fail' }>,
    unknown,
    UpdateDbTableRecordsRequest,
    { previous?: DbTableRecordsList }
  >({
    mutationFn: data => dbService.updateDbTableRecords(dbId, tableId, data),
    onMutate: async variables => {
      await queryClient.cancelQueries({
        queryKey: getDbTableRecordsKey(dbId, tableId, paramsForList),
      });
      const previousResp = queryClient.getQueryData<ApiResponseData<DbTableRecordsList>>(
        getDbTableRecordsKey(dbId, tableId, paramsForList)
      );
      const previous = previousResp?.data;

      const incomingById = new Map<number, Partial<DbTableRecord>>();
      variables.records.forEach(r => {
        incomingById.set(r.id, r);
      });

      const optimisticData = (previous?.data ?? []).map(row => {
        const next = incomingById.get(row.id as number);
        return next ? { ...row, ...next } : row;
      });

      const optimistic: DbTableRecordsList = {
        has_more: previous?.has_more ?? false,
        total_num: previous?.total_num ?? optimisticData.length,
        data: optimisticData as DbTableRecord[],
      };

      queryClient.setQueryData<ApiResponseData<DbTableRecordsList>>(
        getDbTableRecordsKey(dbId, tableId, paramsForList),
        { code: '0', message: 'optimistic', data: optimistic }
      );

      return { previous };
    },
    onError: (err, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData<ApiResponseData<DbTableRecordsList>>(
          getDbTableRecordsKey(dbId, tableId, paramsForList),
          { code: '0', message: 'rollback', data: context.previous }
        );
      }
      // No toast here; centralized error handling in table-data component
    },
    onSuccess: async _resp => {
      const fetched = await dbService.getDbTableRecords(dbId, tableId, paramsForList);
      queryClient.setQueryData<ApiResponseData<DbTableRecordsList>>(
        getDbTableRecordsKey(dbId, tableId, paramsForList),
        fetched
      );
      // No success toast here; centralized success handling in table-data component
    },
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: getDbTableRecordsKey(dbId, tableId, paramsForList),
      });
    },
  });

  const updateRecords = useCallback(
    async (records: UpdateDbTableRecordsRequest['records']) => {
      await mutateAsync({ records });
    },
    [mutateAsync]
  );

  return { updateRecords, isPending };
}

export interface UseDeleteDbTableRecordReturn {
  deleteRecords: (ids: number[]) => Promise<void>;
  isPending: boolean;
}

export function useDeleteDbTableRecord(
  dbId: string,
  tableId: string,
  paramsForList: GetDbTableRecordsParams
): UseDeleteDbTableRecordReturn {
  const queryClient = useQueryClient();

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<{ result: 'success' | 'fail' }>,
    unknown,
    { ids: number[] },
    { previous?: DbTableRecordsList }
  >({
    mutationFn: ({ ids }) => dbService.deleteDbTableRecords(dbId, tableId, ids),
    onMutate: async variables => {
      await queryClient.cancelQueries({
        queryKey: getDbTableRecordsKey(dbId, tableId, paramsForList),
      });
      const previousResp = queryClient.getQueryData<ApiResponseData<DbTableRecordsList>>(
        getDbTableRecordsKey(dbId, tableId, paramsForList)
      );
      const previous = previousResp?.data;

      const idsSet = new Set(variables.ids.map(n => Number(n)));
      const optimisticData = (previous?.data ?? []).filter(row => !idsSet.has(Number(row.id)));
      const optimistic: DbTableRecordsList = {
        has_more: previous?.has_more ?? false,
        total_num: Math.max((previous?.total_num ?? 0) - variables.ids.length, 0),
        data: optimisticData,
      };

      queryClient.setQueryData<ApiResponseData<DbTableRecordsList>>(
        getDbTableRecordsKey(dbId, tableId, paramsForList),
        { code: '0', message: 'optimistic', data: optimistic }
      );

      return { previous };
    },
    onError: (err, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData<ApiResponseData<DbTableRecordsList>>(
          getDbTableRecordsKey(dbId, tableId, paramsForList),
          { code: '0', message: 'rollback', data: context.previous }
        );
      }
      // No toast here; centralized error handling in table-data component
    },
    onSuccess: async _resp => {
      const fetched = await dbService.getDbTableRecords(dbId, tableId, paramsForList);
      queryClient.setQueryData<ApiResponseData<DbTableRecordsList>>(
        getDbTableRecordsKey(dbId, tableId, paramsForList),
        fetched
      );
      // No success toast here; centralized success handling in table-data component
    },
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: getDbTableRecordsKey(dbId, tableId, paramsForList),
      });
    },
  });

  const deleteRecords = useCallback(
    async (ids: number[]) => {
      await mutateAsync({ ids });
    },
    [mutateAsync]
  );

  return { deleteRecords, isPending };
}
