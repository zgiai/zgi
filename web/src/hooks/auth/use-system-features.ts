'use client';

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { authenticationService } from '@/services/auth.service';
import type { SystemFeatures } from '@/services/types/auth';
import { useAuthStore } from '@/store/auth-store';
import { SYSTEM_KEYS } from '@/hooks/query-keys';

export function useSystemFeatures(options?: {
  staleTime?: number;
  refetchOnWindowFocus?: boolean | 'always';
  refetchOnReconnect?: boolean | 'always';
  enabled?: boolean;
}) {
  const result = useQuery<SystemFeatures | null>({
    queryKey: SYSTEM_KEYS.features(),
    queryFn: async () => {
      const resp = await authenticationService.getSystemFeatures();
      const features = resp?.data?.features ?? null;
      return features;
    },
    staleTime: options?.staleTime ?? 30 * 60 * 1000,
    refetchOnWindowFocus: options?.refetchOnWindowFocus ?? false,
    refetchOnReconnect: options?.refetchOnReconnect ?? false,
    refetchOnMount: false,
    enabled: options?.enabled ?? true,
    initialData: () => {
      const current = useAuthStore.getState().systemFeatures;
      return current ?? undefined;
    },
  });

  useEffect(() => {
    if (result.data) {
      useAuthStore.getState().setSystemFeatures(result.data);
    }
  }, [result.data]);

  return result;
}
