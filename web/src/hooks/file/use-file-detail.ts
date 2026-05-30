'use client';

import { useQuery } from '@tanstack/react-query';
import { fileManageService } from '@/services/file-manage.service';
import type { ApiResponseData } from '@/services/types/common';
import type { FileDetailResponse } from '@/services/types/file';

export const FILE_DETAIL_QUERY_KEY = 'file-detail';

export const getFileDetailKey = (fileId: string) => [FILE_DETAIL_QUERY_KEY, fileId];

export function useFileDetail(
  fileId: string,
  options: {
    enabled?: boolean;
    refetchInterval?: number | false;
    refetchOnWindowFocus?: boolean;
  } = {}
) {
  const {
    enabled = true,
    refetchInterval = false,
    refetchOnWindowFocus = false,
  } = options;

  return useQuery<ApiResponseData<FileDetailResponse>>({
    queryKey: getFileDetailKey(fileId),
    queryFn: () => fileManageService.getFileDetail(fileId),
    enabled: enabled && Boolean(fileId),
    refetchInterval,
    refetchOnWindowFocus,
    retry: false,
  });
}
