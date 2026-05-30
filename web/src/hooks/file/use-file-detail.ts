'use client';

import { useQuery } from '@tanstack/react-query';
import { fileManageService } from '@/services/file-manage.service';
import type { ApiResponseData } from '@/services/types/common';
import type { FileDetailResponse } from '@/services/types/file';

export const FILE_DETAIL_QUERY_KEY = 'file-detail';

export const getFileDetailKey = (fileId: string) => [FILE_DETAIL_QUERY_KEY, fileId];

const POLLING_STATUSES = new Set(['parsing', 'generating']);

function getDetailProcessingStatus(detail?: FileDetailResponse): string {
  return (
    detail?.processing?.summary.product_status || detail?.file.processing_status || 'stored_only'
  );
}

export function useFileDetail(
  fileId: string,
  options: {
    enabled?: boolean;
    pollProcessingStatus?: boolean;
    refetchOnWindowFocus?: boolean;
  } = {}
) {
  const {
    enabled = true,
    pollProcessingStatus = false,
    refetchOnWindowFocus = false,
  } = options;

  return useQuery<ApiResponseData<FileDetailResponse>>({
    queryKey: getFileDetailKey(fileId),
    queryFn: () => fileManageService.getFileDetail(fileId),
    enabled: enabled && Boolean(fileId),
    refetchInterval: query => {
      if (!pollProcessingStatus) return false;

      const status = getDetailProcessingStatus(query.state.data?.data);
      return POLLING_STATUSES.has(status) ? 2000 : false;
    },
    refetchOnWindowFocus,
    retry: false,
  });
}
