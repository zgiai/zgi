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
  type IngestFileToTableRequest,
  type IngestFileToTableData,
  type ParseFileForTableIngestRequest,
  type ParseFileForTableIngestData,
  type ExtractTextToTableRecordsRequest,
  type ExtractTextToTableRecordsData,
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
  ) => Promise<{
    columns: DbTableColumn[];
    results: BatchIngestFileToTableData['results'];
    totalCount: number;
    successCount: number;
    failedCount: number;
  }>;
  isPending: boolean;
}

export interface UseIngestFileToTableReturn {
  ingestFile: (
    payload: IngestFileToTableRequest
  ) => Promise<{
    columns: DbTableColumn[];
    result: IngestFileToTableData;
  }>;
  isPending: boolean;
}

export interface UseParseFileForTableIngestReturn {
  parseFile: (
    payload: ParseFileForTableIngestRequest
  ) => Promise<ParseFileForTableIngestData>;
  isPending: boolean;
}

export interface UseExtractTextToTableRecordsReturn {
  extractRecords: (
    payload: ExtractTextToTableRecordsRequest
  ) => Promise<{
    columns: DbTableColumn[];
    result: ExtractTextToTableRecordsData;
  }>;
  isPending: boolean;
}

export function useBatchIngestFileToTable(
  dbId: string,
  tableId: string
): UseBatchIngestFileToTableReturn {
  const queryClient = useQueryClient();
  const t = useT('dbs');

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

      const results = incoming?.results ?? {};
      const totalCount = incoming?.total_count ?? Object.keys(results).length;
      const failedCount =
        incoming?.failed_count ??
        Object.values(results).filter(item => item.error || (item.records ?? []).length === 0)
          .length;
      const successCount = incoming?.success_count ?? Math.max(0, totalCount - failedCount);

      if (totalCount > 0 && successCount === 0) {
        toast.error(t('tableIngest.stepTwo.batchAllFailed', { count: totalCount }));
      } else if (failedCount > 0) {
        toast.warning(
          t('tableIngest.stepTwo.batchPartialFailed', {
            success: successCount,
            total: totalCount,
            failed: failedCount,
          })
        );
      } else {
        toast.success(t('tableIngest.stepTwo.batchAllSuccess', { count: successCount }));
      }
    },
    onError: err => {
      toast.error((err as Error)?.message || t('tableIngest.stepTwo.batchRequestFailed'));
    },
  });

  const ingest = useCallback(
    async (
      payload: BatchIngestFileToTableRequest
    ): Promise<{
      columns: DbTableColumn[];
      results: BatchIngestFileToTableData['results'];
      totalCount: number;
      successCount: number;
      failedCount: number;
    }> => {
      const res = await mutateAsync(payload);
      const results = res?.data?.results ?? {};
      const totalCount = res?.data?.total_count ?? Object.keys(results).length;
      const failedCount =
        res?.data?.failed_count ??
        Object.values(results).filter(item => item.error || (item.records ?? []).length === 0)
          .length;
      const successCount = res?.data?.success_count ?? Math.max(0, totalCount - failedCount);
      const normalizedColumns: DbTableColumn[] = Array.isArray(res?.data?.columns)
        ? (res?.data?.columns ?? []).map(c => ({
            ...c,
            type: normalizeColumnType((c as { type?: unknown }).type),
            is_system_field: Boolean(c.is_system_field),
          }))
        : [];
      return { columns: normalizedColumns, results, totalCount, successCount, failedCount };
    },
    [mutateAsync]
  );

  return { ingest, isPending };
}

export function useParseFileForTableIngest(): UseParseFileForTableIngestReturn {
  const t = useT('dbs');
  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<ParseFileForTableIngestData>,
    unknown,
    ParseFileForTableIngestRequest
  >({
    mutationFn: variables => dbService.parseFileForTableIngest(variables),
    onError: err => {
      toast.error((err as Error)?.message || t('tableIngest.stepTwo.parseRequestFailed'));
    },
  });

  const parseFile = useCallback(
    async (payload: ParseFileForTableIngestRequest): Promise<ParseFileForTableIngestData> => {
      const res = await mutateAsync(payload);
      return {
        ...(res?.data ?? {}),
        message: res?.data?.message ?? '',
      };
    },
    [mutateAsync]
  );

  return { parseFile, isPending };
}

export function useExtractTextToTableRecords(
  dbId: string,
  tableId: string
): UseExtractTextToTableRecordsReturn {
  const queryClient = useQueryClient();
  const t = useT('dbs');

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<ExtractTextToTableRecordsData>,
    unknown,
    ExtractTextToTableRecordsRequest
  >({
    mutationFn: variables => dbService.extractTextToTableRecords(variables),
    onSuccess: response => {
      const incoming = response?.data;
      const normalizedColumns: DbTableColumn[] = Array.isArray(incoming?.columns)
        ? (incoming?.columns ?? []).map(c => ({
            ...c,
            type: normalizeColumnType((c as { type?: unknown }).type),
            is_system_field: Boolean(c.is_system_field),
          }))
        : [];

      void queryClient.invalidateQueries({ queryKey: getTableColumnsKeyWithSystem(dbId, tableId) });
      queryClient.setQueryData<ApiResponseData<DbTableColumnsPayload>>(
        getTableColumnsKeyNoSystem(dbId, tableId),
        {
          code: response.code,
          message: response.message,
          data: { columns: normalizedColumns.filter(c => !c.is_system_field) },
        }
      );
    },
    onError: err => {
      toast.error((err as Error)?.message || t('tableIngest.stepTwo.recognitionRequestFailed'));
    },
  });

  const extractRecords = useCallback(
    async (
      payload: ExtractTextToTableRecordsRequest
    ): Promise<{
      columns: DbTableColumn[];
      result: ExtractTextToTableRecordsData;
    }> => {
      const res = await mutateAsync(payload);
      const normalizedColumns: DbTableColumn[] = Array.isArray(res?.data?.columns)
        ? (res?.data?.columns ?? []).map(c => ({
            ...c,
            type: normalizeColumnType((c as { type?: unknown }).type),
            is_system_field: Boolean(c.is_system_field),
          }))
        : [];
      return {
        columns: normalizedColumns,
        result: {
          ...(res?.data ?? {
            message: '',
            records: [],
            columns: [],
          }),
          columns: normalizedColumns,
          records: res?.data?.records ?? [],
        },
      };
    },
    [mutateAsync]
  );

  return { extractRecords, isPending };
}

export function useIngestFileToTable(
  dbId: string,
  tableId: string
): UseIngestFileToTableReturn {
  const queryClient = useQueryClient();
  const t = useT('dbs');

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<IngestFileToTableData>,
    unknown,
    IngestFileToTableRequest
  >({
    mutationFn: variables => dbService.ingestFileToTable(variables),
    onSuccess: response => {
      const incoming = response?.data;
      const normalizedColumns: DbTableColumn[] = Array.isArray(incoming?.columns)
        ? (incoming?.columns ?? []).map(c => ({
            ...c,
            type: normalizeColumnType((c as { type?: unknown }).type),
            is_system_field: Boolean(c.is_system_field),
          }))
        : [];

      void queryClient.invalidateQueries({ queryKey: getTableColumnsKeyWithSystem(dbId, tableId) });
      queryClient.setQueryData<ApiResponseData<DbTableColumnsPayload>>(
        getTableColumnsKeyNoSystem(dbId, tableId),
        {
          code: response.code,
          message: response.message,
          data: { columns: normalizedColumns.filter(c => !c.is_system_field) },
        }
      );
    },
    onError: err => {
      toast.error((err as Error)?.message || t('tableIngest.stepTwo.batchRequestFailed'));
    },
  });

  const ingestFile = useCallback(
    async (
      payload: IngestFileToTableRequest
    ): Promise<{
      columns: DbTableColumn[];
      result: IngestFileToTableData;
    }> => {
      const res = await mutateAsync(payload);
      const normalizedColumns: DbTableColumn[] = Array.isArray(res?.data?.columns)
        ? (res?.data?.columns ?? []).map(c => ({
            ...c,
            type: normalizeColumnType((c as { type?: unknown }).type),
            is_system_field: Boolean(c.is_system_field),
          }))
        : [];
      return {
        columns: normalizedColumns,
        result: {
          ...(res?.data ?? {
            message: '',
            records: [],
            columns: [],
          }),
          columns: normalizedColumns,
          records: res?.data?.records ?? [],
        },
      };
    },
    [mutateAsync]
  );

  return { ingestFile, isPending };
}
