'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { dbService } from '@/services';
import { DB_KEYS } from '@/hooks/query-keys';
import { getErrorMessage } from '@/utils/error-notifications';
import type {
  AnalyzeExcelImportRequest,
  ConfirmExcelImportRequest,
  ExcelImportErrorList,
  ExcelImportJob,
} from '@/services/types/db';
import type { ApiResponseData } from '@/services/types/common';
import { useT } from '@/i18n';

export function useAnalyzeExcelImport(dbId: string | undefined) {
  const t = useT('dbs');
  return useMutation({
    mutationFn: (payload: AnalyzeExcelImportRequest) => {
      if (!dbId) return Promise.reject(new Error('dbId is required'));
      return dbService.analyzeExcelImport(dbId, payload);
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('excelImport.errors.analyzeFailed'));
    },
  });
}

export function useConfirmExcelImport(dbId: string | undefined, jobId: string | undefined) {
  const queryClient = useQueryClient();
  const t = useT('dbs');
  return useMutation({
    mutationFn: (payload: ConfirmExcelImportRequest) => {
      if (!dbId) return Promise.reject(new Error('dbId is required'));
      if (!jobId) return Promise.reject(new Error('jobId is required'));
      return dbService.confirmExcelImport(dbId, jobId, payload);
    },
    onSuccess: response => {
      if (dbId) {
        queryClient.invalidateQueries({ queryKey: DB_KEYS.tables(dbId), exact: false });
      }
      toast.success(
        response.data.failed_rows > 0
          ? t('excelImport.result.partial')
          : t('excelImport.result.success')
      );
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('excelImport.errors.importFailed'));
    },
  });
}

export function useExcelImportJob(
  dbId: string | undefined,
  jobId: string | undefined,
  enabled = true
) {
  return useQuery<ApiResponseData<ExcelImportJob>, unknown>({
    queryKey:
      dbId && jobId
        ? DB_KEYS.excelImportJob(dbId, jobId)
        : DB_KEYS.excelImportJob('undefined', 'undefined'),
    queryFn: () => {
      if (!dbId) throw new Error('dbId is required');
      if (!jobId) throw new Error('jobId is required');
      return dbService.getExcelImportJob(dbId, jobId);
    },
    enabled: Boolean(dbId && jobId && enabled),
    refetchInterval: query => {
      const status = query.state.data?.data.status;
      return status === 'analyzing' || status === 'importing' ? 2000 : false;
    },
  });
}

export function useExcelImportErrors(
  dbId: string | undefined,
  jobId: string | undefined,
  params: { limit: number; offset: number },
  enabled = true
) {
  return useQuery<ApiResponseData<ExcelImportErrorList>, unknown>({
    queryKey:
      dbId && jobId
        ? DB_KEYS.excelImportErrors(dbId, jobId, params)
        : DB_KEYS.excelImportErrors('undefined', 'undefined', params),
    queryFn: () => {
      if (!dbId) throw new Error('dbId is required');
      if (!jobId) throw new Error('jobId is required');
      return dbService.getExcelImportErrors(dbId, jobId, params);
    },
    enabled: Boolean(dbId && jobId && enabled),
  });
}
