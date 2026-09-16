'use client';

import { useState } from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import QrCodeRecharge from './qrcode-recharge';
import { payService } from '@/services/pay.service';

interface RechargeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onPaymentSuccess?: () => void;
}

export function RechargeDialog({ open, onOpenChange, onPaymentSuccess }: RechargeDialogProps) {
  const t = useT('dashboard');
  const [pendingOrderId, setPendingOrderId] = useState<string | null>(null);

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && pendingOrderId) {
      const orderId = pendingOrderId;
      setPendingOrderId(null);
      onOpenChange(false);
      void payService.cancelOrder(orderId).catch(() => {
        // Closing the dialog must not depend on provider cancellation.
      });
      return;
    }
    onOpenChange(nextOpen);
  };

  // Handle payment success - refresh balance and close dialog
  const handlePaymentSuccess = () => {
    onPaymentSuccess?.();
    setPendingOrderId(null);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>{t('costCenter.rechargeDialog.title')}</DialogTitle>
        </DialogHeader>

        <DialogBody>
          <QrCodeRecharge
            onClose={handlePaymentSuccess}
            onPendingOrderChange={setPendingOrderId}
          />
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

export default RechargeDialog;
