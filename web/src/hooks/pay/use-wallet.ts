'use client';

import { useCallback } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { payService } from '@/services/pay.service';
import type { Wallet } from '@/services/types/pay';
import { PAY_KEYS } from '@/hooks/query-keys';

// Query key for wallet data
export const WALLET_QUERY_KEY = PAY_KEYS.wallet();

// Options for customizing wallet query behavior
interface UseWalletOptions {
  staleTime?: number;
  refetchOnWindowFocus?: boolean | 'always';
  enabled?: boolean;
}

// Fetch current user wallet with caching
export function useWallet(options?: UseWalletOptions) {
  const queryClient = useQueryClient();
  const query = useQuery<Wallet>({
    queryKey: WALLET_QUERY_KEY,
    queryFn: async () => {
      const res = await payService.getMyWallet();
      return res.data as Wallet;
    },
    staleTime: options?.staleTime ?? 60_000, // Cache for 1 minute by default
    refetchOnWindowFocus: options?.refetchOnWindowFocus ?? false,
    enabled: options?.enabled ?? true,
  });

  const refetch = useCallback(() => {
    queryClient.refetchQueries({ queryKey: WALLET_QUERY_KEY, exact: true });
  }, [queryClient]);

  return {
    ...query,
    refetch,
  };
}
