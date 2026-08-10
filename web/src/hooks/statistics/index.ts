'use client';

import { useEffect, useMemo } from 'react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
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
  const query = useInfiniteQuery<ApiResponseData<InvocationLogData>, Error>({
    queryKey: STATS_KEYS.invocations(params),
    queryFn: ({ pageParam }) => {
      const cursor = pageParam as { time: string; id: string } | undefined;
      return statisticsService.getInvocationLog({
        ...params,
        cursor_time: cursor?.time,
        cursor_id: cursor?.id,
      });
    },
    initialPageParam: undefined,
    getNextPageParam: lastPage => lastPage.data?.next_cursor,
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
    const pages = query.data?.pages ?? [];
    const first = pages[0]?.data;
    if (!first) return null;
    return {
      summary: {
        ...first.summary,
        invocation_count: toNumber(first.summary.invocation_count),
        api_count: toNumber(first.summary.api_count),
        product_count: toNumber(first.summary.product_count),
        unknown_count: toNumber(first.summary.unknown_count),
        total_tokens: toNumber(first.summary.total_tokens),
        total_points:
          normalizeAiCreditValue(toNumber(first.summary.total_points), { precision: 3 }) ?? 0,
      },
      items: pages.flatMap(page =>
        (page.data?.items ?? []).map(item => ({
          ...item,
          attempt_count: toNumber(item.attempt_count),
          prompt_tokens: toNumber(item.prompt_tokens),
          completion_tokens: toNumber(item.completion_tokens),
          total_tokens: toNumber(item.total_tokens),
          total_points: normalizeAiCreditValue(toNumber(item.total_points), { precision: 3 }) ?? 0,
          duration_ms: toNumber(item.duration_ms),
          started_at: toNumber(item.started_at),
          settled_at: toNumber(item.settled_at),
        }))
      ),
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
    retry: false,
  });
}
