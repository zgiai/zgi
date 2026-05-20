'use client';

import { useEffect, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
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
} from '@/services/types/statistics';
import { getErrorMessage } from '@/utils/error-notifications';
import { normalizeModelUsageData } from '@/utils/ai-credits';

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
