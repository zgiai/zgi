'use client';

import { useCallback } from 'react';

import { useQuery, useQueryClient } from '@tanstack/react-query';

import { payService } from '@/services/pay.service';

import type { AiCredits } from '@/services/types/pay';

import { PAY_KEYS } from '@/hooks/query-keys';
import { normalizeAiCredits } from '@/utils/ai-credits';

// Query key for AI credits data

export const AI_CREDITS_QUERY_KEY = PAY_KEYS.aiCredits();

// Options for customizing AI credits query behavior

interface UseAiCreditsOptions {
  staleTime?: number;

  refetchOnWindowFocus?: boolean | 'always';

  enabled?: boolean;
}

// Fetch current user AI credits with caching

export function useAiCredits(options?: UseAiCreditsOptions) {
  const queryClient = useQueryClient();

  const query = useQuery<AiCredits>({
    queryKey: AI_CREDITS_QUERY_KEY,

    queryFn: async () => {
      const res = await payService.getMyAiCredits();

      return normalizeAiCredits(res.data as AiCredits);
    },

    staleTime: options?.staleTime ?? 60_000, // Cache for 1 minute by default

    refetchOnWindowFocus: options?.refetchOnWindowFocus ?? false,

    enabled: options?.enabled ?? true,
  });

  const refetch = useCallback(() => {
    queryClient.refetchQueries({ queryKey: AI_CREDITS_QUERY_KEY, exact: true });
  }, [queryClient]);

  return {
    ...query,

    refetch,
  };
}

// Helper to calculate total AI credits (official + private channel)
export function getTotalAiCredits(credits: AiCredits | undefined): number {
  if (!credits) return 0;
  return (credits.official_ai_credits?.balance ?? 0) + (credits.private_channel_funds?.total ?? 0);
}

// Helper to get official AI credits balance
export function getOfficialBalance(credits: AiCredits | undefined): number {
  if (!credits) return 0;
  return credits.official_ai_credits?.balance ?? 0;
}

// Helper to get private channel funds total
export function getPrivateChannelTotal(credits: AiCredits | undefined): number {
  if (!credits) return 0;
  return credits.private_channel_funds?.total ?? 0;
}
