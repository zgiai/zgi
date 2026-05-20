'use client';

import { useMutation } from '@tanstack/react-query';
import { payService } from '@/services/pay.service';
import type { BuyAiCreditRequest, BuyAiCreditResponse } from '@/services/types/pay';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { normalizeToastDescription } from '@/utils/error-notifications';

export function useBuyAiCredits() {
  const t = useT('dashboard');

  return useMutation<BuyAiCreditResponse, Error, BuyAiCreditRequest>({
    mutationFn: async (data: BuyAiCreditRequest) => {
      const response = await payService.buyAiCredits(data);
      return response.data;
    },
    onSuccess: (data, variables) => {
      // Only show success toast if payment is completed (wallet payment)
      if (variables.payment_method === 'wallet' || !data.payment) {
        toast.success(t('costCenter.packages.purchaseSuccess'), {
          description: t('costCenter.packages.purchaseSuccessDesc'),
        });
      }
    },
    onError: (error: Error) => {
      const title = t('costCenter.packages.purchaseFailed');
      const description = error.message || t('costCenter.packages.purchaseFailedDesc');
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    },
  });
}
