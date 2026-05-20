'use client';

import { useQuery, useMutation } from '@tanstack/react-query';
import { payService } from '@/services/pay.service';
import type { TransactionsResponse, BillFilters } from '@/services/types/pay';
import { PAY_KEYS } from '@/hooks/query-keys';
import { normalizeTransactionsResponse } from '@/utils/ai-credits';

// Query key for bills data
export const BILLS_QUERY_KEY = ['bills', 'transactions'] as const;

// Options for customizing bills query behavior
interface UseBillsOptions {
  staleTime?: number;
  refetchOnWindowFocus?: boolean | 'always';
  enabled?: boolean;
}

/**
 * Fetch bill transactions with filters and pagination
 */
export function useBills(filters: BillFilters, options?: UseBillsOptions) {
  return useQuery<TransactionsResponse>({
    queryKey: PAY_KEYS.bills(filters),
    queryFn: async () => {
      const res = await payService.getBillTransactions(filters);
      return normalizeTransactionsResponse(res.data as TransactionsResponse);
    },
    staleTime: options?.staleTime ?? 60_000, // Cache for 1 minute by default
    refetchOnWindowFocus: options?.refetchOnWindowFocus ?? false,
    enabled: options?.enabled ?? true,
  });
}

/**
 * Hook for exporting bills as Excel
 */
export function useExportBills() {
  return useMutation<Blob, Error, BillFilters>({
    mutationFn: async (filters: BillFilters) => {
      return await payService.exportBillTransactions(filters);
    },
  });
}
