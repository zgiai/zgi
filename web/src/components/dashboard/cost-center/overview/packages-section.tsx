'use client';

import { useState } from 'react';
import { useT } from '@/i18n/translations';
import { PackageCard } from '@/components/dashboard/recharge/package-card';
import { BuyAiCreditDialog } from '@/components/dashboard/recharge/buy-ai-credit-dialog';
import { useAiCreditProducts } from '@/hooks/pay/use-ai-credit-products';
import { useBuyAiCredits } from '@/hooks/pay/use-buy-ai-credits';
import { useWallet } from '@/hooks/pay/use-wallet';
import { useAiCredits } from '@/hooks/pay/use-ai-credits';
import type { AiCreditProduct, PaymentMethod } from '@/services/types/pay';

export function PackagesSection() {
  const t = useT('dashboard');
  const { data: aiCreditProducts, isLoading: isAiCreditProductsLoading } = useAiCreditProducts();
  const { mutate: buyAiCredits, isPending: isBuying } = useBuyAiCredits();
  const { data: walletData, refetch: refetchWallet } = useWallet();
  const { refetch: refetchAiCredits } = useAiCredits();

  const [buyDialog, setBuyDialog] = useState<{
    open: boolean;
    product: AiCreditProduct | null;
    orderNo: string | null;
    qrcodeUrl: string | null;
    paymentMethod: PaymentMethod | undefined;
  }>({
    open: false,
    product: null,
    orderNo: null,
    qrcodeUrl: null,
    paymentMethod: undefined,
  });

  const handlePaymentSuccess = () => {
    refetchWallet();
    refetchAiCredits();
  };

  const handleBuyButtonClick = (product: AiCreditProduct) => {
    setBuyDialog({
      open: true,
      product,
      orderNo: null,
      qrcodeUrl: null,
      paymentMethod: undefined,
    });
  };

  const handleConfirmBuy = (params: {
    useBalance: boolean;
    balanceAmount?: number;
    paymentMethod: PaymentMethod;
  }) => {
    if (!buyDialog.product) return;

    const { useBalance, balanceAmount, paymentMethod } = params;

    buyAiCredits(
      {
        product_id: buyDialog.product.id,
        payment_method: paymentMethod,
        use_wallet_balance: useBalance,
        wallet_deduction_amount: balanceAmount || 0,
        quantity: 1,
      },
      {
        onSuccess: data => {
          const payment = data.payment;

          if (payment?.qr_code_content) {
            setBuyDialog(prev => ({
              ...prev,
              orderNo: data.order.id,
              qrcodeUrl: payment.qr_code_content || null,
              paymentMethod: paymentMethod,
            }));
          } else {
            handlePaymentSuccess();
            setBuyDialog({
              open: false,
              product: null,
              orderNo: null,
              qrcodeUrl: null,
              paymentMethod: undefined,
            });
          }
        },
      }
    );
  };

  // Format credit amount for display
  const formatCreditAmount = (amount: number): string => {
    if (amount >= 100_000_000) {
      return `${(amount / 100_000_000).toFixed(0)} ${t('costCenter.format.yiPoints')}`;
    } else if (amount >= 10_000) {
      return `${(amount / 10_000).toFixed(0)} ${t('costCenter.format.wanPoints')}`;
    }
    return `${amount.toLocaleString()} ${t('costCenter.format.points')}`;
  };

  return (
    <>
      <div>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-xl font-bold">{t('costCenter.packages.title')}</h2>
            <p className="text-sm text-muted-foreground">{t('costCenter.packages.subtitle')}</p>
          </div>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {isAiCreditProductsLoading
            ? Array.from({ length: 3 }).map((_, index) => (
                <PackageCard
                  key={`loading-${index}`}
                  id={`loading-${index}`}
                  name=""
                  points=""
                  price=""
                  productCode=""
                  isLoading
                />
              ))
            : aiCreditProducts?.map((product, index) => (
                <PackageCard
                  key={product.id || `product-${index}`}
                  id={product.id || `product-${index}`}
                  name={product.product_name}
                  points={formatCreditAmount(product.credit_amount)}
                  price={`¥ ${product.price.toFixed(2)}`}
                  productCode={product.product_code}
                  isBuying={isBuying}
                  onBuy={() => handleBuyButtonClick(product)}
                />
              ))}
        </div>
      </div>

      <BuyAiCreditDialog
        open={buyDialog.open}
        onOpenChange={open =>
          setBuyDialog({
            open,
            product: open ? buyDialog.product : null,
            orderNo: open ? buyDialog.orderNo : null,
            qrcodeUrl: open ? buyDialog.qrcodeUrl : null,
            paymentMethod: open ? buyDialog.paymentMethod : undefined,
          })
        }
        product={buyDialog.product}
        walletData={walletData}
        onConfirm={handleConfirmBuy}
        onPaymentSuccess={handlePaymentSuccess}
        orderNo={buyDialog.orderNo}
        qrcodeUrl={buyDialog.qrcodeUrl}
        paymentMethod={buyDialog.paymentMethod}
        isLoading={isBuying}
      />
    </>
  );
}
