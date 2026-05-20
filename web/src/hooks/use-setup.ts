'use client';

// Setup hooks powered by React Query
// English comments only. Strict TypeScript (no any).

import { useEffect, useMemo } from 'react';
import { useT } from '@/i18n';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { IS_CLOUD } from '@/lib/config';
import { getErrorMessage } from '@/utils/error-notifications';
import { setupService } from '@/services/setup.service';
import type { ApiResponseData } from '@/services/types/common';
import type { CreateSetupAdminRequest, SystemSetupStatus } from '@/services/types/setup';
import {
  isSetupFinishedLocally,
  saveSetupFinished,
  saveSetupNotStarted,
} from '@/utils/system/setup';
import { useLocale } from '@/hooks/use-locale';

/* -------------------------------------------------------------------------- */
/* Query-key helpers                                                          */
/* -------------------------------------------------------------------------- */

const SETUP_QUERY_KEY = 'setup' as const;
const getSetupStatusKey = () => [SETUP_QUERY_KEY, 'status'] as const;

function normalizeSetupErrorMessage(
  message: string | null | undefined,
  fallback: string,
  passwordRuleMessage: string
) {
  const normalized = message?.trim().toLowerCase();
  if (!normalized) return fallback;
  if (normalized.includes('password validation failed')) {
    return passwordRuleMessage;
  }
  return message as string;
}

/* -------------------------------------------------------------------------- */
/* Types                                                                      */
/* -------------------------------------------------------------------------- */

export interface UseSetupStatusOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

export interface UseSetupStatusReturn {
  status: SystemSetupStatus | null;
  isInitialized: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<unknown>;
}

/* -------------------------------------------------------------------------- */
/* Hook: useSetupStatus – fetch current system setup status                    */
/* -------------------------------------------------------------------------- */

export function useSetupStatus({
  enabled = true,
  staleTime = 5 * 60 * 1000,
  gcTime = 30 * 60 * 1000,
  refetchOnWindowFocus = false,
  refetchInterval = false,
}: UseSetupStatusOptions = {}): UseSetupStatusReturn {
  const t = useT('auth');
  const { locale } = useLocale();

  // Cloud deployments do not expose self-hosted setup flow.
  const isCloudInitialized = IS_CLOUD;

  // Do not request again if finished is cached locally
  const isFinishedCached = isCloudInitialized || isSetupFinishedLocally();

  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<SystemSetupStatus>,
    unknown
  >({
    queryKey: getSetupStatusKey(),
    queryFn: () => setupService.getStatus(locale),
    enabled: enabled && !isFinishedCached,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
    retry: false,
    select: resp => resp,
  });

  // Persist local cache if we discover finished
  useEffect(() => {
    if (isCloudInitialized) {
      return;
    }

    const status = data?.data;
    if (status?.step === 'finished') {
      saveSetupFinished(status.setup_at);
    } else if (status?.step === 'not_started') {
      // Store not_started to allow UI decisions while still allowing future queries
      saveSetupNotStarted();
    }
  }, [data, isCloudInitialized]);

  // Side-effect error toast
  useEffect(() => {
    if (isCloudInitialized) return;
    if (!error) return;
    const message = getErrorMessage(error);
    toast.error(message || t('setupStatusLoadFailed'));
  }, [error, isCloudInitialized, t]);

  const status: SystemSetupStatus | null = useMemo(() => {
    if (isCloudInitialized) {
      return { step: 'finished' };
    }

    return data?.data ?? null;
  }, [data, isCloudInitialized]);

  const isInitialized = isFinishedCached || status?.step === 'finished';

  return {
    status,
    isInitialized,
    isLoading: isCloudInitialized ? false : isLoading,
    isFetching: isCloudInitialized ? false : isFetching,
    error: isCloudInitialized
      ? null
      : error
        ? ((error as { message?: string }).message ?? 'error')
        : null,
    refetch,
  };
}

/* -------------------------------------------------------------------------- */
/* Hook: useCreateSetupAdmin – create admin account                            */
/* -------------------------------------------------------------------------- */

export function useCreateSetupAdmin() {
  const queryClient = useQueryClient();
  const t = useT('auth');
  const { locale } = useLocale();

  return useMutation<
    ApiResponseData<{ result: 'success' | string }>,
    unknown,
    CreateSetupAdminRequest
  >({
    mutationFn: (payload: CreateSetupAdminRequest) =>
      setupService.createAdmin({ ...payload, language: locale }),
    onSuccess: result => {
      if (result?.data?.result === 'success') {
        // Mark setup finished locally to stop future GETs
        const now = new Date().toISOString();
        saveSetupFinished(now);
        toast.success(t('setupInitializedSuccess'));
        // Update and cancel the setup status query to avoid network re-fetch
        queryClient.setQueryData(getSetupStatusKey(), {
          data: { step: 'finished', setup_at: now },
        } as ApiResponseData<SystemSetupStatus>);
        queryClient.cancelQueries({ queryKey: getSetupStatusKey() });
      } else {
        toast.error(t('setupInitializedFailed'));
      }
    },
    onError: (err: unknown) => {
      const msg = getErrorMessage(err);
      toast.error(
        normalizeSetupErrorMessage(msg, t('setupInitializeFailed'), t('initPasswordRule'))
      );
    },
  });
}
