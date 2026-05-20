'use client';

// Analyze file to generate table structure via AI
// Powered by React Query mutation with success/error toasts

import { useMutation } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { dbService } from '@/services';
import type { ApiResponseData } from '@/services/types/common';
import {
  type DbTableColumn,
  type DbTableColumnsPayload,
  Type,
  type AnalyzeFileForTableRequest,
} from '@/services/types/db';
import { DB_KEYS } from '@/hooks/query-keys';

export interface UseAnalyzeFileForTableReturn {
  analyze: (payload: AnalyzeFileForTableRequest) => Promise<DbTableColumn[]>;
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
      const msg = (error as { message?: string }).message ?? 'Failed to analyze file';
      toast.error(msg);
    },
  });

  const analyze = async (payload: AnalyzeFileForTableRequest): Promise<DbTableColumn[]> => {
    const res = await mutateAsync(payload);
    const raw = Array.isArray(res?.data?.columns) ? res.data.columns : [];
    // Ensure strict types and defaults
    return raw.map(col => ({
      id: String(col.id ?? ''),
      name: String(col.name ?? ''),
      description: String(col.description ?? ''),
      type: normalizeType(String(col.type ?? 'text')),
      is_required: Boolean(col.is_required),
      is_system_field: Boolean((col as { is_system_field?: boolean }).is_system_field),
    }));
  };

  return { analyze, isPending };
}
