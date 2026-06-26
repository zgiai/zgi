'use client';

import { useMutation } from '@tanstack/react-query';
import { fileManageService } from '@/services/file-manage.service';
import type { ApiResponseData } from '@/services/types/common';
import type { AskFileQuestionRequest, AskFileQuestionResponse } from '@/services/types/file';

export function useAskFileQuestion(fileId: string) {
  return useMutation<ApiResponseData<AskFileQuestionResponse>, unknown, AskFileQuestionRequest>({
    mutationFn: data => fileManageService.askFileQuestion(fileId, data),
  });
}
