'use client';

import { useState, useEffect, useMemo } from 'react';
import { QrCode, XCircle } from 'lucide-react';
import { useT } from '@/i18n';
import QRCode from 'react-qr-code';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { RadioGroup, Radio } from '@/components/ui/radio';
import { cn } from '@/lib/utils';
import { usePaymentStatusPolling } from '@/hooks/pay/use-payment-status-polling';
import { toast } from 'sonner';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import type { AiCreditProduct, Wallet, OrderStatus, PaymentMethod } from '@/services/types/pay';

// Alipay icon component
function AlipayIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 1024 1024" fill="currentColor">
      <path d="M1024 629.76c0 0-177.664-73.728-284.16-116.736 34.816-68.608 60.928-146.432 74.752-227.84h-202.752v-67.072h247.808v-46.08h-247.808v-119.808h-108.032c-15.872 0-15.872 15.872-15.872 15.872v103.936h-237.568v46.08h237.568v67.072h-192.512v46.08h373.76c-11.776 56.32-30.208 109.568-53.76 157.696-112.128-39.936-238.592-76.8-362.496-76.8-187.392 0-253.44 100.352-253.44 185.344 0 171.008 197.12 205.312 355.328 183.296 114.176-15.872 225.792-67.072 316.928-140.8 102.912 53.248 298.496 147.456 298.496 147.456l53.76-157.696zM254.464 778.24c-122.88 0-154.112-55.808-154.112-99.84 0-68.096 62.976-106.496 159.232-106.496 96.256 0 195.072 26.624 295.936 68.096-77.312 86.016-183.808 138.24-301.056 138.24z" />
    </svg>
  );
}

interface BuyAiCreditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  product: AiCreditProduct | null;
  walletData: Wallet | undefined;
  onConfirm: (params: {
    useBalance: boolean;
    balanceAmount?: number;
    paymentMethod: PaymentMethod;
  }) => void;
  onPaymentSuccess?: () => void;
  orderNo?: string | null;
  qrcodeUrl?: string | null;
  paymentMethod?: PaymentMethod;
  isLoading?: boolean;
}

export function BuyAiCreditDialog({
  open,
  onOpenChange,
  product,
  walletData,
  onConfirm,
  onPaymentSuccess,
  orderNo,
  qrcodeUrl,
  paymentMethod: propPaymentMethod,
  isLoading = false,
}: BuyAiCreditDialogProps) {
  const t = useT('dashboard');
  const [useBalance, setUseBalance] = useState(false);
  const [balanceAmount, setBalanceAmount] = useState('');
  const [paymentMethod, setPaymentMethod] = useState<PaymentMethod>('alipay');
  const [orderStatus, setOrderStatus] = useState<OrderStatus | null>(null);
  const [showConfirmDialog, setShowConfirmDialog] = useState(false);

  // Determine if we should show QR code
  const showQrcode = !!(orderNo && qrcodeUrl);

  const productPrice = product?.price ?? 0;
  const availableBalance = walletData?.available_balance ?? 0;

  // Reset state when dialog opens/closes or product changes
  useEffect(() => {
    if (open && product && !showQrcode) {
      setUseBalance(false);
      setBalanceAmount('');
      setOrderStatus(null);
    } else if (open && showQrcode && qrcodeUrl) {
      setOrderStatus('pending');
    }
  }, [open, product, showQrcode, qrcodeUrl]);

  useEffect(() => {
    setPaymentMethod('alipay');
  }, [open]);
  // Use prop payment method when showing QR code
  useEffect(() => {
    if (propPaymentMethod) {
      setPaymentMethod(propPaymentMethod);
    }
  }, [propPaymentMethod]);

  const isFinalStatus = useMemo(
    () =>
      orderStatus === 'completed' ||
      orderStatus === 'failed' ||
      orderStatus === 'cancelled' ||
      orderStatus === 'expired',
    [orderStatus]
  );

  // Payment status polling
  usePaymentStatusPolling({
    orderNo: orderNo || null,
    enabled: !!orderNo && !isFinalStatus && open && showQrcode,
    interval: 3000,
    maxAttempts: 200,
    onSuccess: (status: OrderStatus) => {
      setOrderStatus(status);
      toast.success(t('costCenter.rechargeDialog.payment.success'), {
        description: t('costCenter.rechargeDialog.payment.successDesc'),
      });
      if (onPaymentSuccess) {
        onPaymentSuccess();
        onOpenChange(false);
      }
    },
    onFailure: (status: OrderStatus) => {
      setOrderStatus(status);
      toast.error(t('costCenter.rechargeDialog.payment.failed'), {
        description: t('costCenter.rechargeDialog.payment.failedDesc'),
      });
    },
    onExpired: () => {
      setOrderStatus('expired');
      toast.error(t('costCenter.rechargeDialog.payment.expired'), {
        description: t('costCenter.rechargeDialog.payment.expiredDesc'),
      });
    },
  });

  // Calculate deduction and payment amounts
  const deductionAmount = useMemo(() => {
    if (!useBalance || !balanceAmount) return 0;
    const amount = parseFloat(balanceAmount);
    if (isNaN(amount) || amount <= 0) return 0;
    // Cannot deduct more than available balance or product price
    return Math.min(amount, availableBalance, productPrice);
  }, [useBalance, balanceAmount, availableBalance, productPrice]);

  const finalPaymentAmount = useMemo(() => {
    return Math.max(0, productPrice - deductionAmount);
  }, [productPrice, deductionAmount]);

  const executeConfirm = () => {
    if (!product) return;

    // Direct pass-through of user selection to parent component
    // The parent component (and backend) will handle the hybrid payment logic
    onConfirm({
      useBalance,
      balanceAmount: useBalance ? deductionAmount : 0,
      paymentMethod,
    });
  };

  const handleConfirm = () => {
    if (!product) return;

    if (useBalance && deductionAmount > 0) {
      setShowConfirmDialog(true);
    } else {
      executeConfirm();
    }
  };

  if (!product) return null;

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle>
              {showQrcode
                ? paymentMethod === 'alipay'
                  ? t('costCenter.rechargeDialog.payment.alipay')
                  : t('costCenter.rechargeDialog.payment.wechat')
                : t('costCenter.packages.buyNow')}
            </DialogTitle>
          </DialogHeader>

          <DialogBody>
            {showQrcode ? (
              // QR Code Payment View
              <div className="space-y-6 py-4">
                <div className="flex flex-col items-center justify-center py-8">
                  <div className="w-48 h-48 border-2 rounded-lg flex items-center justify-center mb-3 bg-white relative">
                    {qrcodeUrl && orderStatus === 'pending' ? (
                      <div className="relative w-full h-full flex items-center justify-center p-4">
                        <QRCode
                          value={qrcodeUrl}
                          size={180}
                          style={{ height: 'auto', maxWidth: '100%', width: '100%' }}
                          viewBox="0 0 180 180"
                        />
                      </div>
                    ) : orderStatus === 'failed' || orderStatus === 'cancelled' ? (
                      <div className="text-center text-red-600">
                        <XCircle className="h-8 w-8 mx-auto mb-2" />
                        <span className="text-sm font-medium">
                          {t('costCenter.rechargeDialog.payment.failed')}
                        </span>
                      </div>
                    ) : (
                      <div className="text-center text-gray-400">
                        <QrCode className="h-8 w-8 mx-auto mb-2" />
                        <span className="text-sm">
                          {t('costCenter.rechargeDialog.qrcode.placeholder')}
                        </span>
                      </div>
                    )}
                  </div>
                  {orderStatus === 'pending' && (
                    <p className="text-sm font-medium animate-pulse">
                      {t('costCenter.rechargeDialog.payment.waitingForPayment')}
                    </p>
                  )}
                </div>
              </div>
            ) : (
              // Purchase Selection View
              <div className="space-y-6 py-4">
                {/* Product Info */}
                <div className="border-b pb-4">
                  <div className="text-sm text-muted-foreground mb-1">
                    {t('costCenter.packages.product')}
                  </div>
                  <div className="text-lg font-bold">{product.product_name}</div>
                  <div className="text-2xl font-bold text-blue-600 mt-2">
                    ¥ {productPrice.toFixed(2)}
                  </div>
                </div>

                {/* Use Balance Section */}
                <div className="space-y-3">
                  <div className="flex items-center space-x-2">
                    <Checkbox
                      id="use-balance"
                      checked={useBalance}
                      onCheckedChange={checked => {
                        const isChecked = checked === true;
                        setUseBalance(isChecked);
                        if (isChecked && !balanceAmount) {
                          setBalanceAmount(Math.min(availableBalance, productPrice).toString());
                        } else if (!isChecked) {
                          setBalanceAmount('');
                        }
                      }}
                    />
                    <Label htmlFor="use-balance" className="text-sm font-medium cursor-pointer">
                      {t('costCenter.packages.useBalance')}
                    </Label>
                    <span className="text-sm text-orange-600">
                      ({t('costCenter.packages.currentBalance')} ¥{availableBalance.toFixed(2)})
                    </span>
                  </div>

                  {useBalance && (
                    <div className="ml-6 space-y-2">
                      <div className="flex items-center gap-2">
                        <Input
                          type="number"
                          value={balanceAmount}
                          onChange={e => setBalanceAmount(e.target.value)}
                          placeholder="0"
                          min={0}
                          max={Math.min(availableBalance, productPrice)}
                          className="flex-1"
                        />
                        <span className="text-sm">{t('costCenter.packages.currencyUnit')}</span>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {t('costCenter.packages.balanceNotice')}
                      </p>
                      <div className="flex justify-end">
                        <span className="text-sm text-muted-foreground">
                          {t('costCenter.packages.deduction')}: ¥{deductionAmount.toFixed(2)}
                        </span>
                      </div>
                    </div>
                  )}
                </div>

                {/* Payment Method Selection */}
                {finalPaymentAmount > 0 && (
                  <div className="space-y-3 border-t pt-4">
                    <div className="flex items-center justify-between mb-2">
                      <div className="text-sm font-medium">
                        {t('costCenter.packages.selectPaymentMethod')}
                      </div>
                      <div className="text-sm font-bold text-orange-600">
                        {t('costCenter.packages.payment')}: ¥{finalPaymentAmount.toFixed(2)}
                      </div>
                    </div>

                    <RadioGroup
                      value={paymentMethod}
                      onValueChange={value => setPaymentMethod(value as PaymentMethod)}
                      orientation="horizontal"
                      className="gap-3"
                    >
                      <div className="flex-1">
                        <label
                          htmlFor="alipay"
                          className={cn(
                            'flex items-center justify-center gap-2 p-4 rounded-lg border-2 cursor-pointer transition-all',
                            paymentMethod === 'alipay'
                              ? 'border-blue-500 bg-blue-50'
                              : 'border-gray-200 hover:border-blue-300 hover:bg-gray-50'
                          )}
                        >
                          <Radio value="alipay" id="alipay" className="sr-only" />
                          <AlipayIcon
                            className={cn(
                              'h-6 w-6',
                              paymentMethod === 'alipay' ? 'text-blue-600' : 'text-gray-400'
                            )}
                          />
                          <span
                            className={cn(
                              'font-medium',
                              paymentMethod === 'alipay' ? 'text-blue-600' : 'text-gray-600'
                            )}
                          >
                            {t('costCenter.rechargeDialog.payment.alipay')}
                          </span>
                        </label>
                      </div>
                    </RadioGroup>
                  </div>
                )}

                {finalPaymentAmount === 0 && useBalance && (
                  <div className="border-t pt-4">
                    <div className="text-sm text-center text-green-600 font-medium">
                      {t('costCenter.packages.balanceSufficient')}
                    </div>
                  </div>
                )}
              </div>
            )}
          </DialogBody>

          {!showQrcode && (
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isLoading}>
                {t('costCenter.packages.cancel')}
              </Button>
              <Button onClick={handleConfirm} disabled={isLoading || finalPaymentAmount < 0}>
                {isLoading ? t('costCenter.packages.buying') : t('costCenter.packages.confirmBuy')}
              </Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={showConfirmDialog}
        onOpenChange={setShowConfirmDialog}
        title={t('costCenter.rechargeDialog.payment.confirmWalletPaymentTitle')}
        description={t('costCenter.rechargeDialog.payment.confirmWalletPaymentDesc')}
        onConfirm={executeConfirm}
        confirmText={t('costCenter.packages.confirmBuy')}
        cancelText={t('costCenter.packages.cancel')}
      />
    </>
  );
}
