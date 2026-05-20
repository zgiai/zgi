'use client';

import { useQuery } from '@tanstack/react-query';
import { payService } from '@/services/pay.service';
import type { AiCreditProduct } from '@/services/types/pay';
import { PAY_KEYS } from '@/hooks/query-keys';
import { normalizeAiCreditProduct } from '@/utils/ai-credits';

// Query key for AI credit products data
export const AI_CREDIT_PRODUCTS_QUERY_KEY = PAY_KEYS.products();

// Options for customizing AI credit products query behavior
interface UseAiCreditProductsOptions {
  staleTime?: number;
  refetchOnWindowFocus?: boolean | 'always';
  enabled?: boolean;
}

// Fetch AI credit products with caching
export function useAiCreditProducts(options?: UseAiCreditProductsOptions) {
  return useQuery<AiCreditProduct[]>({
    queryKey: AI_CREDIT_PRODUCTS_QUERY_KEY,
    queryFn: async () => {
      const res = await payService.getAiCreditProducts();
      return (res.data as AiCreditProduct[]).map(normalizeAiCreditProduct);
    },
    staleTime: options?.staleTime ?? 5 * 60 * 1000, // Cache for 5 minutes by default
    refetchOnWindowFocus: options?.refetchOnWindowFocus ?? false,
    enabled: options?.enabled ?? true,
  });
}
