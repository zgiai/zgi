'use client';

// Batch ingest files into a table via AI
// Powered by React Query mutation with success/error toasts

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useCallback } from 'react';
import { toast } from 'sonner';
import { dbService } from '@/services';
import type { ApiResponseData } from '@/services/types/common';
import {
  type DbTableColumn,
  type DbTableColumnsPayload,
  Type,
  type BatchIngestFileToTableRequest,
  type BatchIngestFileToTableData,
} from '@/services/types/db';
import { useT } from '@/i18n';
import { DB_KEYS } from '@/hooks/query-keys';

// Local query-key helpers are now centralized in DB_KEYS
const getTableColumnsKeyWithSystem = (dbId: string, tableId: string) =>
  DB_KEYS.tableColumns(dbId, tableId, true);
const getTableColumnsKeyNoSystem = (dbId: string, tableId: string) =>
  DB_KEYS.tableColumns(dbId, tableId, false);

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
/* Hook: useBatchIngestFileToTable                                            */
/* -------------------------------------------------------------------------- */

export interface UseBatchIngestFileToTableReturn {
  ingest: (
    payload: BatchIngestFileToTableRequest
  ) => Promise<{ columns: DbTableColumn[]; results: BatchIngestFileToTableData['results'] }>;
  isPending: boolean;
}

export function useBatchIngestFileToTable(
  dbId: string,
  tableId: string
): UseBatchIngestFileToTableReturn {
  const queryClient = useQueryClient();
  const t = useT();

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<BatchIngestFileToTableData>,
    unknown,
    BatchIngestFileToTableRequest
  >({
    mutationFn: variables => dbService.batchIngestFileToTable(variables),
    onSuccess: response => {
      const incoming = response?.data;
      const normalizedColumns: DbTableColumn[] = Array.isArray(incoming?.columns)
        ? (incoming?.columns ?? []).map(c => ({
            ...c,
            type: normalizeColumnType((c as { type?: unknown }).type),
            is_system_field: Boolean(c.is_system_field),
          }))
        : [];

      // Update table columns cache for both with-system and no-system keys
      void queryClient.invalidateQueries({ queryKey: getTableColumnsKeyWithSystem(dbId, tableId) });
      queryClient.setQueryData<ApiResponseData<DbTableColumnsPayload>>(
        getTableColumnsKeyNoSystem(dbId, tableId),
        {
          code: response.code,
          message: response.message,
          data: { columns: normalizedColumns.filter(c => !c.is_system_field) },
        }
      );

      toast.success(t('common.success'));
    },
    onError: err => {
      toast.error((err as Error)?.message || 'Failed to ingest files into table');
    },
  });

  const ingest = useCallback(
    async (
      payload: BatchIngestFileToTableRequest
    ): Promise<{ columns: DbTableColumn[]; results: BatchIngestFileToTableData['results'] }> => {
      const res = await mutateAsync(payload);
      const results = res?.data?.results ?? {};
      const normalizedColumns: DbTableColumn[] = Array.isArray(res?.data?.columns)
        ? (res?.data?.columns ?? []).map(c => ({
            ...c,
            type: normalizeColumnType((c as { type?: unknown }).type),
            is_system_field: Boolean(c.is_system_field),
          }))
        : [];
      return { columns: normalizedColumns, results };
    },
    [mutateAsync]
  );

  return { ingest, isPending };
}
