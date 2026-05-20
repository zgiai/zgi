'use client';

// Table prompt hooks powered by React Query
// English comments for maintainability and clarity

import { useCallback } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { dbService } from '@/services';
import type { ApiResponseData } from '@/services/types/common';
import type { DbTablePrompt, UpdateDbTablePromptRequest } from '@/services/types/db';
import { getErrorMessage } from '@/utils/error-notifications';
import { DB_KEYS } from '@/hooks/query-keys';

// Local query-key helpers are now centralized in DB_KEYS
const getTablePromptKey = (dbId: string, tableId: string) => DB_KEYS.tablePrompt(dbId, tableId);

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseDbTablePromptOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

export interface UseDbTablePromptReturn {
  record: DbTablePrompt | null;
  prompt: string;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<unknown>;
}

/* -------------------------------------------------------------------------- */
/* Hook: useDbTablePrompt – fetch current table prompt                         */
/* -------------------------------------------------------------------------- */

export function useDbTablePrompt(
  dbId: string,
  tableId: string,
  options?: UseDbTablePromptOptions
): UseDbTablePromptReturn {
  const { data, isLoading, isFetching, error, refetch } = useQuery<ApiResponseData<DbTablePrompt>>({
    queryKey: getTablePromptKey(dbId, tableId),
    queryFn: () => dbService.getDbTablePrompt(dbId, tableId),
    enabled: !!dbId && !!tableId && (options?.enabled ?? true),
    staleTime: options?.staleTime ?? 60 * 1000,
    gcTime: options?.gcTime ?? 10 * 60 * 1000,
    refetchOnWindowFocus: options?.refetchOnWindowFocus ?? false,
    refetchInterval: options?.refetchInterval ?? false,
  });

  const record = data?.data ?? null;
  return {
    record,
    prompt: record?.prompt ?? '',
    isLoading,
    isFetching,
    error: (error as Error | null)?.message ?? null,
    refetch,
  };
}

/* -------------------------------------------------------------------------- */
/* Hook: useUpdateDbTablePrompt – optimistic update with toasts               */
/* -------------------------------------------------------------------------- */

export interface UseUpdateDbTablePromptReturn {
  updatePrompt: (nextPrompt: string) => Promise<DbTablePrompt>;
  isPending: boolean;
}

export function useUpdateDbTablePrompt(
  dbId: string,
  tableId: string
): UseUpdateDbTablePromptReturn {
  const queryClient = useQueryClient();
  const t = useT('dbs');

  const { mutateAsync, isPending } = useMutation<
    ApiResponseData<DbTablePrompt>,
    unknown,
    UpdateDbTablePromptRequest,
    { previous?: DbTablePrompt | null }
  >({
    mutationFn: variables => dbService.updateDbTablePrompt(dbId, tableId, variables),
    // Optimistic update: set prompt immediately
    onMutate: async variables => {
      await queryClient.cancelQueries({ queryKey: getTablePromptKey(dbId, tableId) });
      const previousData = queryClient.getQueryData<ApiResponseData<DbTablePrompt>>(
        getTablePromptKey(dbId, tableId)
      );
      const previous = previousData?.data ?? null;
      const optimistic: DbTablePrompt | null = previous
        ? { ...previous, prompt: variables.prompt, updated_at: previous.updated_at }
        : null;
      if (optimistic) {
        queryClient.setQueryData<ApiResponseData<DbTablePrompt>>(getTablePromptKey(dbId, tableId), {
          code: '0',
          message: 'optimistic',
          data: optimistic,
        });
      }
      return { previous };
    },
    // Rollback on error
    onError: (err, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData<ApiResponseData<DbTablePrompt>>(getTablePromptKey(dbId, tableId), {
          code: '0',
          message: 'rollback',
          data: context.previous,
        });
      }
      const msg = getErrorMessage(err);
      toast.error(msg || t('failed', { defaultMessage: 'Operation failed' }));
    },
    // Success toast; authoritative sync handled in updatePrompt
    onSuccess: _response => {
      toast.success(t('promptUpdateSuccess', { defaultMessage: 'Prompt updated' }));
    },
  });

  const updatePrompt = useCallback(
    async (nextPrompt: string) => {
      await mutateAsync({ prompt: nextPrompt });
      // Authoritative GET to ensure latest timestamps and fields
      const fetched = await dbService.getDbTablePrompt(dbId, tableId);
      queryClient.setQueryData<ApiResponseData<DbTablePrompt>>(
        getTablePromptKey(dbId, tableId),
        fetched
      );
      return fetched.data as DbTablePrompt;
    },
    [mutateAsync, queryClient]
  );

  return { updatePrompt, isPending };
}
