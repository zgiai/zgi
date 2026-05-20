'use client';

// Hooks for DB table batch import operations (template download and file import)
// English comments for maintainability and clarity

import { useCallback, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { dbService } from '@/services';
import { toast } from 'sonner';
import { getErrorMessage } from '@/utils/error-notifications';
import type { ApiResponseData } from '@/services/types/common';
import type { ImportDbTableRecordsData, GetDbTableRecordsParams } from '@/services/types/db';
import type { FileItem } from '@/services/types/file';
import { DB_KEYS } from '@/hooks/query-keys';

// Local query-key helpers are now centralized in DB_KEYS
const getDbTableRecordsKey = (dbId: string, tableId: string, params?: GetDbTableRecordsParams) =>
  DB_KEYS.tableRecords(dbId, tableId, params || {});

/* -------------------------------------------------------------------------- */
/* Hook: useDownloadDbTableTemplate – download template file                  */
/* -------------------------------------------------------------------------- */

export interface UseDownloadDbTableTemplateReturn {
  downloadTemplate: () => Promise<void>;
  isDownloading: boolean;
  error: string | null;
}

export function useDownloadDbTableTemplate(
  dbId: string,
  tableId: string
): UseDownloadDbTableTemplateReturn {
  const [isDownloading, setIsDownloading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const downloadTemplate = useCallback(async () => {
    setIsDownloading(true);
    setError(null);
    try {
      const blob = await dbService.downloadDbTableTemplate(dbId, tableId);
      // Create download link and trigger download
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      // Default filename, server may provide Content-Disposition header
      link.download = `table_template_${tableId}.xlsx`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Download failed';
      setError(message);
      throw err;
    } finally {
      setIsDownloading(false);
    }
  }, [dbId, tableId]);

  return { downloadTemplate, isDownloading, error };
}

/* -------------------------------------------------------------------------- */
/* Hook: useImportDbTableRecords – import records from file                   */
/* -------------------------------------------------------------------------- */

export interface UseImportDbTableRecordsReturn {
  importRecords: (file: FileItem) => Promise<ImportDbTableRecordsData>;
  isPending: boolean;
  error: string | null;
  data: ImportDbTableRecordsData | null;
  reset: () => void;
}

export function useImportDbTableRecords(
  dbId: string,
  tableId: string
): UseImportDbTableRecordsReturn {
  const queryClient = useQueryClient();
  const t = useT('dbs');

  const { mutateAsync, isPending, error, data, reset } = useMutation<
    ApiResponseData<ImportDbTableRecordsData>,
    Error,
    FileItem
  >({
    mutationFn: (file: FileItem) =>
      dbService.importDbTableRecords(dbId, tableId, { upload_file_id: file.id }),
    onSuccess: response => {
      // Invalidate records queries to refresh data after import
      queryClient.invalidateQueries({
        queryKey: getDbTableRecordsKey(dbId, tableId),
      });
      // Show success toast with import statistics
      const result = response.data;
      toast.success(
        t('batchImport.importSuccess') +
          '\n' +
          t('batchImport.importResult', {
            total: result.total_count,
            success: result.affected_rows,
            failed: result.failed_count,
          })
      );
    },
    onError: err => {
      // Show error toast
      toast.error(getErrorMessage(err) || t('batchImport.importFailed'));
    },
  });

  const importRecords = useCallback(
    async (file: FileItem): Promise<ImportDbTableRecordsData> => {
      const response = await mutateAsync(file);
      return response.data;
    },
    [mutateAsync]
  );

  return {
    importRecords,
    isPending,
    error: error?.message ?? null,
    data: data?.data ?? null,
    reset,
  };
}
