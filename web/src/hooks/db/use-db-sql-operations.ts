'use client';

// SQL operations history hooks powered by React Query
// English comments for clarity and maintainability

import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { dbService } from '@/services';
import type { ApiResponseData } from '@/services/types/common';
import type {
  DbSqlOperation,
  DbSqlOperationList,
  OperationType,
  SqlOperationStatus,
} from '@/services/types/db';
import { getErrorMessage } from '@/utils/error-notifications';
import { DB_KEYS } from '@/hooks/query-keys';

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseDbSqlOperationsFilters {
  created_by?: string;
  end_time?: string;
  operation_type?: OperationType;
  start_time?: string;
  status?: SqlOperationStatus;
}

/* -------------------------------------------------------------------------- */
/* Hook: useDbSqlOperationsPaged – classic pagination for system paginator     */
/* -------------------------------------------------------------------------- */

export interface UseDbSqlOperationsPagedReturn {
  items: DbSqlOperation[];
  total: number;
  page: number;
  pageSize: number;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<unknown>;
}

export function useDbSqlOperationsPaged(
  dbId: string,
  filters: UseDbSqlOperationsFilters = {},
  page: number,
  pageSize: number = 20,
  {
    enabled = true,
    staleTime = 30 * 1000,
    gcTime = 10 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
  }: UseDbSqlOperationsOptions = {}
): UseDbSqlOperationsPagedReturn {
  const t = useT('dbs');

  const normalizedParams = useMemo(
    () => ({
      created_by: filters.created_by,
      end_time: filters.end_time,
      operation_type: filters.operation_type,
      start_time: filters.start_time,
      status: filters.status,
      limit: String(pageSize),
      page: String(page),
    }),
    [
      filters.created_by,
      filters.end_time,
      filters.operation_type,
      filters.start_time,
      filters.status,
      pageSize,
      page,
    ]
  );

  const query = useQuery<ApiResponseData<DbSqlOperationList>, unknown>({
    queryKey: DB_KEYS.sqlOperations(dbId, normalizedParams),
    queryFn: () => dbService.getDbSqlOperations(dbId, normalizedParams),
    enabled: Boolean(dbId) && enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
    retry: false,
  });

  if (query.error) {
    const message = getErrorMessage(query.error);
    toast.error(message || t('failed', { defaultMessage: 'Operation failed' }));
  }

  const payload = query.data?.data;
  return {
    items: payload?.data ?? [],
    total: payload?.total ?? 0,
    page: payload?.page ?? page,
    pageSize,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: (query.error as Error | null)?.message ?? null,
    refetch: query.refetch,
  };
}

export interface UseDbSqlOperationsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}
