'use client';

// Table columns hooks powered by React Query
// English comments for maintainability and clarity

import { useCallback, useMemo } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { dbService } from '@/services';
import type { ApiResponseData } from '@/services/types/common';
import { Type } from '@/services/types/db';
import { getErrorMessage } from '@/utils/error-notifications';
import { DB_KEYS } from '@/hooks/query-keys';
import type {
  DbTableColumn,
  DbTableColumnsPayload,
  UpdateDbTableColumnsRequest,
  DbTableColumnUpdateInput,
  UpdateDbTableColumnsResponse,
} from '@/services/types/db';

// Local query-key helpers are now centralized in DB_KEYS
const getTableColumnsKey = (dbId: string, tableId: string, includeSystemFields: boolean = true) =>
  DB_KEYS.tableColumns(dbId, tableId, includeSystemFields);

// Stable empty array to prevent re-render loops on initial load
const EMPTY_COLUMNS: DbTableColumn[] = [];

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseDbTableColumnsOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
  includeSystemFields?: boolean;
}

export interface UseDbTableColumnsReturn {
  columns: DbTableColumn[];
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<unknown>;
}

/* -------------------------------------------------------------------------- */
/* Helpers                                                                    */
/* -------------------------------------------------------------------------- */

// Normalize backend type strings to strict enum `Type`
const normalizeColumnType = (value: unknown): Type => {
  const v = String(value ?? '')
    .toLowerCase()
    .trim();
  switch (v) {
    case 'int':
    case 'integer':
    case Type.Integer:
      return Type.Integer;
    case 'numeric':
    case Type.Numeric:
      return Type.Numeric;
    case 'boolean':
    case Type.Boolean:
      return Type.Boolean;
    case 'timestamp':
    case Type.Timestamp:
      return Type.Timestamp;
    case 'text':
    case Type.Text:
    default:
      return Type.Text;
  }
};

/* -------------------------------------------------------------------------- */
/* Hook: useDbTableColumns – fetch table structure                            */
/* -------------------------------------------------------------------------- */

export function useDbTableColumns(
  dbId: string,
  tableId: string,
  options?: UseDbTableColumnsOptions
): UseDbTableColumnsReturn {
  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<DbTableColumnsPayload>
  >({
    queryKey: getTableColumnsKey(dbId, tableId, options?.includeSystemFields ?? true),
    queryFn: () => dbService.getDbTableColumns(dbId, tableId, options?.includeSystemFields ?? true),
    enabled: !!dbId && !!tableId && (options?.enabled ?? true),
    staleTime: options?.staleTime ?? 60 * 1000,
    gcTime: options?.gcTime ?? 10 * 60 * 1000,
    refetchOnWindowFocus: options?.refetchOnWindowFocus ?? false,
    refetchInterval: options?.refetchInterval ?? false,
  });
  const columns = useMemo(
    () =>
      (data?.data?.columns ?? EMPTY_COLUMNS).map(c => ({
        ...c,
        // Ensure type compatibility: backend may return 'int' while UI uses 'integer'
        type: normalizeColumnType((c as { type?: unknown }).type),
        is_system_field: Boolean(c.is_system_field),
      })),
    [data?.data?.columns]
  );

  return {
    columns,
    isLoading,
    isFetching,
    error: (error as Error | null)?.message ?? null,
    refetch,
  };
}

/* -------------------------------------------------------------------------- */
/* Hook: useUpdateDbTableColumns – optimistic update with toasts              */
/* -------------------------------------------------------------------------- */

export interface UseUpdateDbTableColumnsReturn {
  updateColumns: (next: DbTableColumn[]) => Promise<DbTableColumn[]>;
  isPending: boolean;
}

export function useUpdateDbTableColumns(
  dbId: string,
  tableId: string
): UseUpdateDbTableColumnsReturn {
  const queryClient = useQueryClient();
  const t = useT('dbs');
  interface ColumnUpdateVariables {
    clientColumns: DbTableColumn[];
    request: UpdateDbTableColumnsRequest;
  }

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<UpdateDbTableColumnsResponse>,
    unknown,
    ColumnUpdateVariables,
    { previous?: DbTableColumn[] }
  >({
    mutationFn: variables => dbService.updateDbTableColumns(dbId, tableId, variables.request),
    // Optimistic update
    onMutate: async variables => {
      await queryClient.cancelQueries({ queryKey: getTableColumnsKey(dbId, tableId, true) });
      const previousData = queryClient.getQueryData<ApiResponseData<DbTableColumnsPayload>>(
        getTableColumnsKey(dbId, tableId, true)
      );
      const previous = previousData?.data?.columns ?? [];

      // Merge non-system updates into previous, keep system fields untouched
      const incoming = Array.isArray(variables.clientColumns)
        ? variables.clientColumns.filter(c => !c.is_system_field)
        : [];
      const merged: DbTableColumn[] = previous.map(prevCol => {
        if (prevCol.is_system_field) return prevCol;
        const updated = incoming.find(c => c.id === prevCol.id);
        return updated ? updated : prevCol;
      });
      // Append newly added non-system columns not existing previously
      const appended = incoming.filter(c => !previous.some(p => p.id === c.id));
      const optimistic = [...merged, ...appended];

      queryClient.setQueryData<ApiResponseData<DbTableColumnsPayload>>(
        getTableColumnsKey(dbId, tableId, true),
        {
          code: '0',
          message: 'optimistic',
          data: { columns: optimistic },
        }
      );

      return { previous };
    },
    // On error, rollback
    onError: (err, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData<ApiResponseData<DbTableColumnsPayload>>(
          getTableColumnsKey(dbId, tableId, true),
          {
            code: '0',
            message: 'rollback',
            data: { columns: context.previous },
          }
        );
      }
      const msg = getErrorMessage(err);
      toast.error(msg || t('failed', { defaultMessage: 'Operation failed' }));
    },
    // On success, show toast; columns sync handled in updateColumns via a single GET
    onSuccess: _response => {
      toast.success(t('columnsUpdateSuccess', { defaultMessage: 'Columns updated' }));
    },
  });

  const updateColumns = useCallback(
    async (next: DbTableColumn[]) => {
      // Build request payload: omit `id` for newly added rows (not in previous cache)
      const existingData = queryClient.getQueryData<ApiResponseData<DbTableColumnsPayload>>(
        getTableColumnsKey(dbId, tableId, true)
      );
      const previous = existingData?.data?.columns ?? [];
      const previousIds = new Set(previous.map(c => c.id));

      const requestColumns: DbTableColumnUpdateInput[] = next
        .filter(c => !c.is_system_field)
        .map(c => ({
          // Only include id if it's from server (exists in previous cache)
          id: previousIds.has(c.id) ? c.id : undefined,
          name: c.name,
          description: c.description,
          type: c.type,
          is_required: c.is_required,
        }));

      await mutateAsync({ clientColumns: next, request: { columns: requestColumns } });
      // Single authoritative GET after successful PUT; also sync cache for subscribers
      const fetched = await dbService.getDbTableColumns(dbId, tableId, true);
      const sync = Array.isArray(fetched.data?.columns) ? fetched.data.columns : [];
      const normalized = sync.map(c => ({
        ...c,
        type: normalizeColumnType((c as { type?: unknown }).type),
        is_system_field: Boolean(c.is_system_field),
      }));
      queryClient.setQueryData<ApiResponseData<DbTableColumnsPayload>>(
        getTableColumnsKey(dbId, tableId, true),
        {
          code: fetched.code,
          message: fetched.message,
          data: { columns: normalized },
        }
      );
      return normalized;
    },
    [mutateAsync, queryClient]
  );

  return { updateColumns, isPending };
}
