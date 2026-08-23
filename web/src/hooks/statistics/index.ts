'use client';

import { useEffect, useMemo } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { isAxiosError } from 'axios';
import { toast } from 'sonner';

import { useT } from '@/i18n';
import { STATS_KEYS } from '@/hooks/query-keys';
import { statisticsService } from '@/services/statistics.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  GetModelUsageParams,
  ModelUsageByAppTypeItem,
  ModelUsageByModelItem,
  ModelUsageDailyItem,
  ModelUsageData,
  ModelUsageSummary,
  GetInvocationLogParams,
  InvocationLogData,
  UpdateInvocationContentSettingsInput,
} from '@/services/types/statistics';
import { getErrorMessage } from '@/utils/error-notifications';
import { normalizeAiCreditValue, normalizeModelUsageData } from '@/utils/ai-credits';

export interface UseModelUsageOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

export interface UseModelUsageReturn {
  data: ModelUsageData | null;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  refetch: () => Promise<unknown>;
}

function toNumber(value: number | string | null | undefined): number {
  return Number(value ?? 0);
}

function sanitizeSummary(summary: ModelUsageSummary): ModelUsageSummary {
  return {
    attempt_count: toNumber(summary.attempt_count),
    success_count: toNumber(summary.success_count),
    failed_count: toNumber(summary.failed_count),
    partial_count: toNumber(summary.partial_count),
    prompt_tokens: toNumber(summary.prompt_tokens),
    cache_read_tokens: toNumber(summary.cache_read_tokens),
    cache_write_tokens: toNumber(summary.cache_write_tokens),
    completion_tokens: toNumber(summary.completion_tokens),
    total_tokens: toNumber(summary.total_tokens),
    official_points: toNumber(summary.official_points),
    private_points: toNumber(summary.private_points),
    total_points: toNumber(summary.total_points),
  };
}

function sanitizeModelItem(item: ModelUsageByModelItem): ModelUsageByModelItem {
  return {
    ...item,
    attempt_count: toNumber(item.attempt_count),
    success_count: toNumber(item.success_count),
    failed_count: toNumber(item.failed_count),
    partial_count: toNumber(item.partial_count),
    prompt_tokens: toNumber(item.prompt_tokens),
    cache_read_tokens: toNumber(item.cache_read_tokens),
    cache_write_tokens: toNumber(item.cache_write_tokens),
    completion_tokens: toNumber(item.completion_tokens),
    total_tokens: toNumber(item.total_tokens),
    official_points: toNumber(item.official_points),
    private_points: toNumber(item.private_points),
    total_points: toNumber(item.total_points),
    points_share: toNumber(item.points_share),
  };
}

function sanitizeAppTypeItem(item: ModelUsageByAppTypeItem): ModelUsageByAppTypeItem {
  return {
    ...item,
    attempt_count: toNumber(item.attempt_count),
    success_count: toNumber(item.success_count),
    failed_count: toNumber(item.failed_count),
    partial_count: toNumber(item.partial_count),
    prompt_tokens: toNumber(item.prompt_tokens),
    cache_read_tokens: toNumber(item.cache_read_tokens),
    cache_write_tokens: toNumber(item.cache_write_tokens),
    completion_tokens: toNumber(item.completion_tokens),
    total_tokens: toNumber(item.total_tokens),
    official_points: toNumber(item.official_points),
    private_points: toNumber(item.private_points),
    total_points: toNumber(item.total_points),
    points_share: toNumber(item.points_share),
  };
}

function sanitizeDailyItem(item: ModelUsageDailyItem): ModelUsageDailyItem {
  return {
    ...item,
    attempt_count: toNumber(item.attempt_count),
    success_count: toNumber(item.success_count),
    failed_count: toNumber(item.failed_count),
    partial_count: toNumber(item.partial_count),
    prompt_tokens: toNumber(item.prompt_tokens),
    cache_read_tokens: toNumber(item.cache_read_tokens),
    cache_write_tokens: toNumber(item.cache_write_tokens),
    completion_tokens: toNumber(item.completion_tokens),
    total_tokens: toNumber(item.total_tokens),
    official_tokens: toNumber(item.official_tokens),
    private_tokens: toNumber(item.private_tokens),
    official_points: toNumber(item.official_points),
    private_points: toNumber(item.private_points),
    total_points: toNumber(item.total_points),
  };
}

export function useModelUsage(
  params: GetModelUsageParams,
  {
    enabled = true,
    staleTime = 5 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
  }: UseModelUsageOptions = {}
): UseModelUsageReturn {
  const t = useT('dashboard');

  const { data, isLoading, isFetching, error, refetch } = useQuery<
    ApiResponseData<ModelUsageData>,
    Error
  >({
    queryKey: STATS_KEYS.usage(params),
    queryFn: () => statisticsService.getModelUsage(params),
    enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
    retry: false,
  });

  useEffect(() => {
    if (!error) return;
    const message = getErrorMessage(error);
    toast.error(message || t('usage.loadFailed'));
  }, [error, t]);

  const stats = useMemo<ModelUsageData | null>(() => {
    if (!data?.data) return null;

    return normalizeModelUsageData({
      ...data.data,
      period: {
        start_time: toNumber(data.data.period.start_time),
        end_time: toNumber(data.data.period.end_time),
      },
      summary: sanitizeSummary(data.data.summary),
      by_model: (data.data.by_model || []).map(sanitizeModelItem),
      by_app_type: (data.data.by_app_type || []).map(sanitizeAppTypeItem),
      daily_trend: (data.data.daily_trend || []).map(sanitizeDailyItem),
    });
  }, [data]);

  return {
    data: stats,
    isLoading,
    isFetching,
    error: error ? error.message : null,
    refetch,
  };
}

export function useInvocationLog(params: GetInvocationLogParams, enabled = true) {
  const t = useT('dashboard');
  const query = useQuery<ApiResponseData<InvocationLogData>, Error>({
    queryKey: STATS_KEYS.invocations(params),
    queryFn: () => statisticsService.getInvocationLog(params),
    enabled,
    staleTime: 60_000,
    retry: false,
  });

  useEffect(() => {
    if (!query.error) return;
    const message = getErrorMessage(query.error);
    toast.error(message || t('usage.invocations.loadFailed'));
  }, [query.error, t]);

  const data = useMemo(() => {
    const response = query.data?.data;
    if (!response) return null;
    return {
      summary: {
        ...response.summary,
        invocation_count: toNumber(response.summary.invocation_count),
        api_count: toNumber(response.summary.api_count),
        product_count: toNumber(response.summary.product_count),
        unknown_count: toNumber(response.summary.unknown_count),
        total_tokens: toNumber(response.summary.total_tokens),
        total_points:
          normalizeAiCreditValue(toNumber(response.summary.total_points), { precision: 3 }) ?? 0,
      },
      items: (response.items ?? []).map(item => ({
        ...item,
        attempt_count: toNumber(item.attempt_count),
        prompt_tokens: toNumber(item.prompt_tokens),
        completion_tokens: toNumber(item.completion_tokens),
        total_tokens: toNumber(item.total_tokens),
        total_points: normalizeAiCreditValue(toNumber(item.total_points), { precision: 3 }) ?? 0,
        duration_ms: toNumber(item.duration_ms),
        started_at: toNumber(item.started_at),
        settled_at: toNumber(item.settled_at),
        content_expires_at:
          item.content_expires_at === undefined ? undefined : toNumber(item.content_expires_at),
      })),
      next_cursor: response.next_cursor,
    };
  }, [query.data]);

  return { ...query, data };
}

export function useInvocationContentSettings(organizationId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: STATS_KEYS.invocationContentSettings(organizationId ?? ''),
    queryFn: () => statisticsService.getInvocationContentSettings(),
    enabled: enabled && Boolean(organizationId),
    staleTime: 60_000,
    retry: false,
  });
}

export function useUpdateInvocationContentSettings(organizationId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateInvocationContentSettingsInput) =>
      statisticsService.updateInvocationContentSettings(input),
    onSuccess: data => {
      queryClient.setQueryData(STATS_KEYS.invocationContentSettings(organizationId ?? ''), data);
    },
  });
}

export function usePurgeInvocationContent(organizationId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => statisticsService.purgeInvocationContent(),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: STATS_KEYS.invocationContentSettings(organizationId ?? ''),
      });
      void queryClient.invalidateQueries({ queryKey: STATS_KEYS.all });
    },
  });
}

export function useInvocationContent(invocationId: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: STATS_KEYS.invocationContent(invocationId ?? ''),
    queryFn: () => statisticsService.getInvocationContent(invocationId ?? ''),
    enabled: enabled && Boolean(invocationId),
    staleTime: 0,
    gcTime: 0,
    // Content is written asynchronously in ~200ms batches. A short retry
    // window prevents a just-completed invocation from looking permanently
    // unavailable without retrying authorization or other failures forever.
    retry: (failureCount, error) =>
      isAxiosError(error) && error.response?.status === 404 && failureCount < 2,
    retryDelay: attempt => 250 * (attempt + 1),
  });
}
