'use client';

import { useQuery } from '@tanstack/react-query';
import { payService } from '@/services/pay.service';
import type { MonthlyStats } from '@/services/types/pay';
import { PAY_KEYS } from '@/hooks/query-keys';
import { normalizeMonthlyStats } from '@/utils/ai-credits';

// Query key for monthly stats
export const MONTHLY_STATS_QUERY_KEY = PAY_KEYS.monthlyStats();

// Options for customizing monthly stats query behavior
interface UseMonthlyStatsOptions {
  staleTime?: number;
  refetchOnWindowFocus?: boolean | 'always';
  enabled?: boolean;
}

// Fetch monthly statistics with caching
export function useMonthlyStats(options?: UseMonthlyStatsOptions) {
  return useQuery<MonthlyStats>({
    queryKey: MONTHLY_STATS_QUERY_KEY,
    queryFn: async () => {
      const res = await payService.getMonthlyStats();
      return normalizeMonthlyStats(res.data as MonthlyStats);
    },
    staleTime: options?.staleTime ?? 60_000, // Cache for 1 minute by default
    refetchOnWindowFocus: options?.refetchOnWindowFocus ?? false,
    enabled: options?.enabled ?? true,
  });
}
