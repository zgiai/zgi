'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import {
  Info,
  XCircle,
  Loader2,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  ChevronLeft,
} from 'lucide-react';
import { useT, type DashboardKey } from '@/i18n';
import QRCode from 'react-qr-code';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { useRechargePayment } from '@/hooks/pay/use-recharge-payment';
import { usePaymentStatusPolling } from '@/hooks/pay/use-payment-status-polling';
import { toast } from 'sonner';
import type { OrderStatus } from '@/services/types/pay';

type PaymentMethod = 'wechat' | 'alipay';

const RECHARGE_AMOUNTS = [
  { cny: 100, usd: 14.29 },
  { cny: 500, usd: 71.43 },
  { cny: 1000, usd: 142.86 },
  { cny: 2000, usd: 285.71 },
  { cny: 5000, usd: 714.29 },
  { cny: 10000, usd: 1428.57 },
];

const EXCHANGE_RATE = 7.0;
const MIN_CUSTOM_AMOUNT = 10;
const ORDER_TIMEOUT_SECONDS = 600; // 10 minutes

const PAYMENT_METHODS: Array<{
  value: PaymentMethod;
  label: string;
  colors: {
    active: { border: string; bg: string; text: string };
    inactive: { border: string; text: string };
  };
}> = [
  // {
  //   value: 'wechat',
  //   label: 'payment.wechat',
  //   colors: {
  //     active: { border: 'border-green-500', bg: 'bg-green-50', text: 'text-green-600' },
  //     inactive: { border: 'border-gray-200', text: 'text-gray-400' },
  //   },
  // },
  {
    value: 'alipay',
    label: 'payment.alipay',
    colors: {
      active: { border: 'border-blue-500', bg: 'bg-blue-50', text: 'text-blue-600' },
      inactive: { border: 'border-gray-200', text: 'text-gray-400' },
    },
  },
];

// WeChat icon component
function WeChatIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor">
      <path d="M8.691 2.188C3.891 2.188 0 5.476 0 9.53c0 2.212 1.17 4.203 3.002 5.55a.59.59 0 0 1 .213.665l-.39 1.48c-.019.07-.048.141-.048.213 0 .163.13.295.29.295a.326.326 0 0 0 .167-.054l1.903-1.114a.864.864 0 0 1 .717-.098 10.16 10.16 0 0 0 2.837.403c.276 0 .543-.027.811-.05-.857-2.578.157-4.972 1.932-6.446 1.703-1.415 3.882-1.98 5.853-1.838-.576-3.583-4.196-6.348-8.596-6.348zM5.785 5.991c.642 0 1.162.529 1.162 1.18a1.17 1.17 0 0 1-1.162 1.178A1.17 1.17 0 0 1 4.623 7.17c0-.651.52-1.18 1.162-1.18zm5.813 0c.642 0 1.162.529 1.162 1.18a1.17 1.17 0 0 1-1.162 1.178 1.17 1.17 0 0 1-1.162-1.178c0-.651.52-1.18 1.162-1.18zm5.34 2.867c-1.797-.052-3.746.512-5.28 1.786-1.72 1.428-2.687 3.72-1.78 6.22.942 2.453 3.666 4.229 6.884 4.229.826 0 1.622-.12 2.361-.336a.722.722 0 0 1 .598.082l1.584.926a.272.272 0 0 0 .14.047c.134 0 .24-.111.24-.247 0-.06-.023-.12-.038-.177l-.327-1.233a.582.582 0 0 1-.023-.156.49.49 0 0 1 .201-.398C23.024 18.48 24 16.82 24 14.98c0-3.21-2.931-5.837-6.656-6.088V8.89c-.135-.01-.27-.027-.407-.03zm-2.53 3.274c.535 0 .969.44.969.982a.976.976 0 0 1-.969.983.976.976 0 0 1-.969-.983c0-.542.434-.982.97-.982zm4.844 0c.535 0 .969.44.969.982a.976.976 0 0 1-.969.983.976.976 0 0 1-.969-.983c0-.542.434-.982.969-.982z" />
    </svg>
  );
}

// Alipay icon component
function AlipayIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 1024 1024" fill="currentColor">
      <path d="M1024 629.76c0 0-177.664-73.728-284.16-116.736 34.816-68.608 60.928-146.432 74.752-227.84h-202.752v-67.072h247.808v-46.08h-247.808v-119.808h-108.032c-15.872 0-15.872 15.872-15.872 15.872v103.936h-237.568v46.08h237.568v67.072h-192.512v46.08h373.76c-11.776 56.32-30.208 109.568-53.76 157.696-112.128-39.936-238.592-76.8-362.496-76.8-187.392 0-253.44 100.352-253.44 185.344 0 171.008 197.12 205.312 355.328 183.296 114.176-15.872 225.792-67.072 316.928-140.8 102.912 53.248 298.496 147.456 298.496 147.456l53.76-157.696zM254.464 778.24c-122.88 0-154.112-55.808-154.112-99.84 0-68.096 62.976-106.496 159.232-106.496 96.256 0 195.072 26.624 295.936 68.096-77.312 86.016-183.808 138.24-301.056 138.24z" />
    </svg>
  );
}

interface QrCodeRechargeProps {
  onClose?: () => void;
}

interface OrderState {
  orderNo: string | null;
  qrcodeUrl: string | null;
  status: OrderStatus | null;
}

export function QrCodeRecharge({ onClose }: QrCodeRechargeProps) {
  const t = useT();
  const [step, setStep] = useState<'selection' | 'payment'>('selection');

  // Selection state
  const [selectedAmount, setSelectedAmount] = useState<number | null>(null);
  const [customAmount, setCustomAmount] = useState('');
  const [paymentMethod, setPaymentMethod] = useState<PaymentMethod>('alipay');

  // Payment state
  const [order, setOrder] = useState<OrderState>({ orderNo: null, qrcodeUrl: null, status: null });
  const [timeLeft, setTimeLeft] = useState(ORDER_TIMEOUT_SECONDS);

  const { mutate: createPayment, isPending: isCreatingPayment } = useRechargePayment();

  const finalAmount = useMemo(() => {
    if (customAmount) {
      const amount = parseFloat(customAmount);
      return isNaN(amount) ? 0 : amount;
    }
    return selectedAmount || 0;
  }, [selectedAmount, customAmount]);

  const isFinalStatus = useMemo(
    () =>
      order.status === 'completed' ||
      order.status === 'failed' ||
      order.status === 'cancelled' ||
      order.status === 'expired',
    [order.status]
  );

  // Polling hook
  usePaymentStatusPolling({
    orderNo: order.orderNo,
    enabled: step === 'payment' && !!order.orderNo && !isFinalStatus,
    interval: 3000,
    maxAttempts: 200,
    onSuccess: (status: OrderStatus) => {
      setOrder(prev => ({ ...prev, status }));
      toast.success(t('dashboard.costCenter.rechargeDialog.payment.success'), {
        description: t('dashboard.costCenter.rechargeDialog.payment.successDesc'),
      });
    },
    onFailure: (status: OrderStatus) => {
      setOrder(prev => ({ ...prev, status }));
      toast.error(t('dashboard.costCenter.rechargeDialog.payment.failed'), {
        description: t('dashboard.costCenter.rechargeDialog.payment.failedDesc'),
      });
    },
    onExpired: () => {
      setOrder(prev => ({ ...prev, status: 'expired' }));
      toast.error(t('dashboard.costCenter.rechargeDialog.payment.expired'), {
        description: t('dashboard.costCenter.rechargeDialog.payment.expiredDesc'),
      });
    },
  });

  // Countdown timer
  useEffect(() => {
    if (step === 'payment' && order.status === 'pending' && timeLeft > 0) {
      const timer = setInterval(() => {
        setTimeLeft(prev => {
          if (prev <= 1) {
            clearInterval(timer);
            setOrder(o => ({ ...o, status: 'expired' }));
            return 0;
          }
          return prev - 1;
        });
      }, 1000);
      return () => clearInterval(timer);
    }
  }, [step, order.status, timeLeft]);

  const handleAmountSelect = useCallback((amount: number) => {
    setSelectedAmount(amount);
    setCustomAmount('');
  }, []);

  const handleCustomAmountChange = useCallback((value: string) => {
    setCustomAmount(value);
    setSelectedAmount(null);
  }, []);

  const initiatePayment = useCallback(() => {
    if (finalAmount <= 0) return;

    setStep('payment');
    setOrder({ orderNo: null, qrcodeUrl: null, status: null });

    createPayment(
      { amount: finalAmount, payment_method: paymentMethod, currency: 'CNY' },
      {
        onSuccess: data => {
          if (!data.payment.qr_code_content) {
            toast.error(t('dashboard.costCenter.rechargeDialog.payment.fetchQrCodeFailed'), {
              description: t('dashboard.costCenter.rechargeDialog.payment.fetchQrCodeFailedDesc'),
            });
            setOrder(prev => ({ ...prev, status: 'failed' }));
            return;
          }

          setOrder({
            orderNo: data.order.id,
            qrcodeUrl: data.payment.qr_code_content,
            status: 'pending',
          });
          setTimeLeft(ORDER_TIMEOUT_SECONDS);
        },
        onError: () => {
          setOrder(prev => ({ ...prev, status: 'failed' }));
        },
      }
    );
  }, [finalAmount, paymentMethod, createPayment, t]);

  const handleRefresh = useCallback(() => {
    initiatePayment();
  }, [initiatePayment]);

  const handleBack = useCallback(() => {
    setStep('selection');
    setOrder({ orderNo: null, qrcodeUrl: null, status: null });
  }, []);

  const formatTime = (seconds: number) => {
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
  };

  if (step === 'selection') {
    return (
      <div className="space-y-4">
        {/* Recharge Instructions */}
        <div className="bg-blue-50 border border-blue-200 rounded-lg p-3">
          <div className="flex items-start gap-2">
            <Info className="h-4 w-4 text-blue-600 mt-0.5 shrink-0" />
            <div className="text-sm text-blue-800">
              <div className="font-medium mb-1">
                {t('dashboard.costCenter.rechargeDialog.instructions.title')}
              </div>
              <ul className="space-y-1 text-xs">
                <li>• {t('dashboard.costCenter.rechargeDialog.instructions.scan')}</li>
                <li>
                  •{' '}
                  {t('dashboard.costCenter.rechargeDialog.instructions.rate', {
                    rate: EXCHANGE_RATE.toFixed(1),
                  })}
                </li>
                <li>• {t('dashboard.costCenter.rechargeDialog.instructions.arrival')}</li>
              </ul>
            </div>
          </div>
        </div>

        {/* Amount Selection */}
        <div className="space-y-3">
          <div className="font-medium">
            {t('dashboard.costCenter.rechargeDialog.amountSelection')}
          </div>
          <div className="grid grid-cols-3 gap-3">
            {RECHARGE_AMOUNTS.map(({ cny, usd }) => (
              <button
                key={cny}
                type="button"
                onClick={() => handleAmountSelect(cny)}
                className={cn(
                  'flex flex-col items-center justify-center p-3 rounded-lg border-2 transition-all',
                  selectedAmount === cny && !customAmount
                    ? 'border-blue-500 bg-blue-50'
                    : 'border-gray-200 hover:border-blue-300 hover:bg-gray-50'
                )}
              >
                <span className="text-lg font-bold">¥ {cny}</span>
                <span className="text-xs text-muted-foreground">≈ ${usd.toFixed(2)}</span>
              </button>
            ))}
          </div>

          {/* Custom Amount Input */}
          <div className="space-y-2">
            <label className="text-sm font-medium">
              {t('dashboard.costCenter.rechargeDialog.customAmount')}
            </label>
            <Input
              type="number"
              placeholder={t('dashboard.costCenter.rechargeDialog.customAmountPlaceholder')}
              value={customAmount}
              onChange={e => handleCustomAmountChange(e.target.value)}
              min={MIN_CUSTOM_AMOUNT}
            />
            <p className="text-xs text-muted-foreground">
              {t('dashboard.costCenter.rechargeDialog.customAmountMinHint', {
                min: MIN_CUSTOM_AMOUNT,
              })}
            </p>
          </div>
        </div>

        {/* Payment Method Selection */}
        <div className="flex gap-3 pt-6">
          {PAYMENT_METHODS.map(({ value, label, colors }) => {
            const Icon = value === 'wechat' ? WeChatIcon : AlipayIcon;
            const isActive = paymentMethod === value;
            return (
              <button
                key={value}
                type="button"
                onClick={() => setPaymentMethod(value)}
                className={cn(
                  'flex-1 flex items-center justify-center gap-2 py-3 px-4 rounded-lg border-2 transition-all',
                  isActive
                    ? `${colors.active.border} ${colors.active.bg}`
                    : `${colors.inactive.border} hover:bg-gray-50`
                )}
              >
                <Icon
                  className={cn('h-6 w-6', isActive ? colors.active.text : colors.inactive.text)}
                />
                <span
                  className={cn('font-medium', isActive ? colors.active.text : 'text-gray-600')}
                >
                  {t(`dashboard.costCenter.rechargeDialog.${label}` as DashboardKey)}
                </span>
              </button>
            );
          })}
        </div>

        <Button
          className="w-full mt-4"
          size="lg"
          onClick={initiatePayment}
          disabled={finalAmount < MIN_CUSTOM_AMOUNT}
        >
          {t('dashboard.costCenter.rechargeDialog.payment.pay')}
        </Button>
      </div>
    );
  }

  // Payment Step
  return (
    <div className="flex flex-col items-center justify-center py-4 space-y-6">
      {/* Back Button */}
      {order.status !== 'completed' && (
        <div className="w-full flex justify-start">
          <Button variant="ghost" size="sm" onClick={handleBack} className="gap-1 pl-0">
            <ChevronLeft className="h-4 w-4" />
            {t('dashboard.costCenter.rechargeDialog.payment.recharge')}
          </Button>
        </div>
      )}

      {isCreatingPayment ? (
        <div className="flex flex-col items-center justify-center py-12">
          <Loader2 className="h-10 w-10 animate-spin text-primary mb-4" />
          <p className="text-muted-foreground">
            {t('dashboard.costCenter.rechargeDialog.payment.creating')}
          </p>
        </div>
      ) : order.status === 'completed' ? (
        <div className="flex flex-col items-center justify-center py-8 text-center animate-in fade-in zoom-in duration-300">
          <CheckCircle2 className="h-16 w-16 text-green-500 mb-4" />
          <h3 className="text-xl font-bold text-green-600 mb-2">
            {t('dashboard.costCenter.rechargeDialog.payment.success')}
          </h3>
          <p className="text-muted-foreground mb-8">
            {t('dashboard.costCenter.rechargeDialog.payment.successDesc')}
          </p>
          <Button onClick={() => onClose?.()} className="w-full max-w-xs">
            {t('dashboard.costCenter.rechargeDialog.payment.close')}
          </Button>
        </div>
      ) : order.status === 'failed' ||
        order.status === 'expired' ||
        (!order.qrcodeUrl && !isCreatingPayment) ? (
        <div className="flex flex-col items-center justify-center py-8 text-center">
          {order.status === 'expired' ? (
            <AlertCircle className="h-16 w-16 text-yellow-500 mb-4" />
          ) : (
            <XCircle className="h-16 w-16 text-red-500 mb-4" />
          )}
          <h3 className="text-xl font-bold mb-2">
            {order.status === 'expired'
              ? t('dashboard.costCenter.rechargeDialog.payment.orderTimeout')
              : !order.qrcodeUrl
                ? t('dashboard.costCenter.rechargeDialog.payment.fetchQrCodeFailed')
                : t('dashboard.costCenter.rechargeDialog.payment.failed')}
          </h3>
          <p className="text-muted-foreground mb-8">
            {order.status === 'expired'
              ? t('dashboard.costCenter.rechargeDialog.payment.expiredDesc')
              : !order.qrcodeUrl
                ? t('dashboard.costCenter.rechargeDialog.payment.fetchQrCodeFailedDesc')
                : t('dashboard.costCenter.rechargeDialog.payment.failedDesc')}
          </p>
          <Button onClick={handleRefresh} variant="outline" className="gap-2">
            <RefreshCw className="h-4 w-4" />
            {t('dashboard.costCenter.rechargeDialog.payment.refresh')}
          </Button>
        </div>
      ) : (
        <div className="flex flex-col items-center w-full">
          {/* Countdown Timer */}
          <div className="mb-6 text-center">
            <div className="text-3xl font-mono font-bold text-primary mb-2">
              {formatTime(timeLeft)}
            </div>
            <p className="text-sm text-muted-foreground">
              {t('dashboard.costCenter.rechargeDialog.payment.timeoutHint')}
            </p>
          </div>

          {/* QR Code */}
          <div className="w-64 h-64 border-2 rounded-xl flex items-center justify-center p-4 bg-white shadow-sm mb-4">
            {order.qrcodeUrl && (
              <QRCode
                value={order.qrcodeUrl}
                size={220}
                style={{ height: 'auto', maxWidth: '100%', width: '100%' }}
                viewBox="0 0 220 220"
              />
            )}
          </div>

          <div className="flex items-center gap-2 text-sm text-muted-foreground animate-pulse">
            <Loader2 className="h-3 w-3 animate-spin" />
            {t('dashboard.costCenter.rechargeDialog.payment.waitingForPayment')}
          </div>

          <div className="mt-2 text-xs text-muted-foreground">
            {t(`dashboard.costCenter.rechargeDialog.qrcode.${paymentMethod}Hint`)}
          </div>
        </div>
      )}
    </div>
  );
}

export default QrCodeRecharge;
