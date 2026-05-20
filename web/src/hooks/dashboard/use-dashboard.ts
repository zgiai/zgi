'use client';

import { useQuery } from '@tanstack/react-query';
import { dashboardService } from '@/services/dashboard.service';
import { DASHBOARD_KEYS } from '@/hooks/query-keys';

const getDashboardStatsKey = () => DASHBOARD_KEYS.stats();

export const useDashboardStats = () => {
  return useQuery({
    queryKey: getDashboardStatsKey(),
    queryFn: () => dashboardService.getDashboardStats(),
    // 5 minutes cache as suggested in docs
    staleTime: 5 * 60 * 1000,
  });
};
