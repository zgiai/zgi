'use client';

import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import QrCodeRecharge from './qrcode-recharge';

interface RechargeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onPaymentSuccess?: () => void;
}

export function RechargeDialog({ open, onOpenChange, onPaymentSuccess }: RechargeDialogProps) {
  const t = useT('dashboard');

  // Handle payment success - refresh balance and close dialog
  const handlePaymentSuccess = () => {
    onPaymentSuccess?.();
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>{t('costCenter.rechargeDialog.title')}</DialogTitle>
        </DialogHeader>

        <DialogBody>
          <QrCodeRecharge onClose={handlePaymentSuccess} />
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

export default RechargeDialog;
