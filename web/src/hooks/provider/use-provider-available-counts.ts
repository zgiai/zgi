'use client';

import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { PROVIDER_KEYS } from '@/hooks/query-keys';
import { modelService } from '@/services/model.service';
import type { ProviderItem } from '@/services/types/provider';

export interface ProviderAvailableCountsResult {
  counts: Record<string, number>;
  isLoading: boolean;
}

export function useProviderAvailableCounts(
  providers: ProviderItem[]
): ProviderAvailableCountsResult {
  const trackedProviders = useMemo(
    () =>
      providers
        .filter(provider => provider.is_enabled && (provider.model_count ?? 0) > 0)
        .map(provider => provider.provider)
        .sort(),
    [providers]
  );

  const { data, isLoading } = useQuery({
    queryKey: PROVIDER_KEYS.availableCounts(trackedProviders),
    enabled: trackedProviders.length > 0,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
    queryFn: async () => {
      const settled = await Promise.allSettled(
        trackedProviders.map(async provider => {
          const response = await modelService.getAvailableModels({ provider });
          return [provider, response.data.total ?? response.data.items.length] as const;
        })
      );

      return settled.reduce<Record<string, number>>((acc, result, index) => {
        const provider = trackedProviders[index];
        if (!provider) return acc;

        if (result.status === 'fulfilled') {
          acc[provider] = result.value[1];
        } else {
          acc[provider] = 0;
        }

        return acc;
      }, {});
    },
  });

  return {
    counts: data ?? {},
    isLoading,
  };
}
