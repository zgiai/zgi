'use client';

import { useMutation } from '@tanstack/react-query';
import { payService } from '@/services/pay.service';
import type { RechargePaymentRequest, RechargePaymentResponse } from '@/services/types/pay';
import { toast } from 'sonner';
import { useT } from '@/i18n';

export function useRechargePayment() {
  const t = useT('dashboard');

  return useMutation<RechargePaymentResponse, Error, RechargePaymentRequest>({
    mutationFn: async data => {
      const response = await payService.createRechargePayment(data);
      return response.data;
    },
    onError: () => {
      toast.error(t('costCenter.rechargeDialog.payment.createOrderFailed'));
    },
  });
}
