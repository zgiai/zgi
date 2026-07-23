'use client';

// Analyze file to generate table structure via AI
// Powered by React Query mutation with success/error toasts

import { useMutation } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { dbService } from '@/services';
import type { ApiResponseData } from '@/services/types/common';
import {
  type DbTableColumnsPayload,
  Type,
  type AnalyzeFileForTableRequest,
} from '@/services/types/db';

export interface UseAnalyzeFileForTableReturn {
  analyze: (payload: AnalyzeFileForTableRequest) => Promise<DbTableColumnsPayload>;
  isPending: boolean;
}

// Runtime safe mapping for `type` string -> enum `Type`
const normalizeType = (value: string): Type => {
  switch (value) {
    case Type.Boolean:
    case 'boolean':
      return Type.Boolean;
    case Type.Integer:
    case 'integer':
      return Type.Integer;
    case Type.Numeric:
    case 'numeric':
      return Type.Numeric;
    case Type.Timestamp:
    case 'timestamp':
      return Type.Timestamp;
    case Type.Text:
    case 'text':
    default:
      return Type.Text;
  }
};

const getAnalyzeErrorMessage = (
  error: unknown,
  networkMessage: string,
  fallback: string
): string => {
  const requestError = error as
    | {
        code?: string;
        message?: string;
        response?: { data?: { message?: string; errorMessage?: string } };
        businessError?: { message?: string };
      }
    | undefined;
  const backendMessage =
    requestError?.response?.data?.message ||
    requestError?.response?.data?.errorMessage ||
    requestError?.businessError?.message;
  if (backendMessage?.trim()) return backendMessage.trim();
  if (
    requestError?.code === 'ERR_NETWORK' ||
    requestError?.code === 'NETWORK_ERROR' ||
    requestError?.message === 'Network Error'
  ) {
    return networkMessage;
  }
  return requestError?.message?.trim() || fallback;
};

export function useAnalyzeFileForTable(): UseAnalyzeFileForTableReturn {
  const t = useT();

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<DbTableColumnsPayload>,
    unknown,
    AnalyzeFileForTableRequest
  >({
    mutationFn: payload => {
      // Forward all required fields including model
      const data: AnalyzeFileForTableRequest = {
        prompt: payload.prompt,
        model: payload.model,
      };
      if (payload.file_id && payload.file_id.trim().length > 0) {
        data.file_id = payload.file_id.trim();
      }
      if (payload.data_source_id && payload.data_source_id.trim().length > 0) {
        data.data_source_id = payload.data_source_id.trim();
      }
      return dbService.analyzeFileForTable(data);
    },
    onSuccess: () => {
      toast.success(t('dbs.analyze.success'));
    },
    onError: error => {
      toast.error(
        getAnalyzeErrorMessage(error, t('dbs.analyze.networkFailed'), t('dbs.analyze.failed'))
      );
    },
  });

  const analyze = async (payload: AnalyzeFileForTableRequest): Promise<DbTableColumnsPayload> => {
    const res = await mutateAsync(payload);
    const raw = Array.isArray(res?.data?.columns) ? res.data.columns : [];
    // Ensure strict types and defaults
    const columns = raw.map(col => ({
      id: String(col.id ?? ''),
      name: String(col.name ?? ''),
      description: String(col.description ?? ''),
      type: normalizeType(String(col.type ?? 'text')),
      is_required: Boolean(col.is_required),
      is_system_field: Boolean((col as { is_system_field?: boolean }).is_system_field),
    }));
    return {
      columns,
      content: typeof res?.data?.content === 'string' ? res.data.content : '',
    };
  };

  return { analyze, isPending };
}
