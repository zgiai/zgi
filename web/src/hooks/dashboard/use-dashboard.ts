'use client';

import { useQuery } from '@tanstack/react-query';
import { dashboardService } from '@/services/dashboard.service';
import { DASHBOARD_KEYS } from '@/hooks/query-keys';
import type { DashboardRecentWorkParams, DashboardStatsParams } from '@/services/types/dashboard';

export const useDashboardStats = (
  params: DashboardStatsParams = {},
  options?: { enabled?: boolean; refetchOnWindowFocus?: boolean; staleTime?: number }
) => {
  const scope = params.scope ?? 'overview';
  return useQuery({
    queryKey: DASHBOARD_KEYS.stats(scope, params.workspace_id),
    queryFn: () => dashboardService.getDashboardStats(params),
    enabled: options?.enabled ?? true,
    refetchOnWindowFocus: options?.refetchOnWindowFocus,
    // 5 minutes cache as suggested in docs
    staleTime: options?.staleTime ?? 5 * 60 * 1000,
  });
};

export const useDashboardRecentWork = (
  params: DashboardRecentWorkParams,
  options?: { enabled?: boolean; refetchOnWindowFocus?: boolean; staleTime?: number }
) => {
  const scope = params.scope ?? 'overview';

  return useQuery({
    queryKey: DASHBOARD_KEYS.recentWork(scope, params.workspace_id, params.limit),
    queryFn: () => dashboardService.getRecentWork(params),
    enabled: options?.enabled ?? true,
    refetchOnWindowFocus: options?.refetchOnWindowFocus,
    staleTime: options?.staleTime ?? 60 * 1000,
  });
};
