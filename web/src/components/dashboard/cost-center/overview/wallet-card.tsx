'use client';

import { useState } from 'react';
import { useT } from '@/i18n/translations';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { RechargeDialog } from '@/components/dashboard/recharge/recharge-dialog';
import { useWallet } from '@/hooks/pay/use-wallet';

export function WalletCard() {
  const t = useT('dashboard');
  const [isRechargeDialogOpen, setIsRechargeDialogOpen] = useState(false);
  const { data: walletData, isLoading: isWalletLoading, refetch: refetchWallet } = useWallet();

  const balance = walletData?.balance ?? 0;

  return (
    <>
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center justify-between mb-2">
            <span className="text-sm text-muted-foreground">{t('costCenter.walletBalance')}</span>
          </div>
          {isWalletLoading ? (
            <>
              <Skeleton className="h-8 w-32 mb-1" />
              <div className="flex items-center gap-1 justify-between mt-2">
                <span className="text-xs text-muted-foreground">
                  {t('costCenter.availableBalance')}:
                </span>
              </div>
            </>
          ) : (
            <>
              <div className="text-2xl font-bold mb-1">¥ {balance.toFixed(2)}</div>
            </>
          )}
          <Button
            className="w-full mt-16"
            variant="default"
            onClick={() => setIsRechargeDialogOpen(true)}
          >
            {t('costCenter.recharge')}
          </Button>
        </CardContent>
      </Card>

      <RechargeDialog
        open={isRechargeDialogOpen}
        onOpenChange={setIsRechargeDialogOpen}
        onPaymentSuccess={refetchWallet}
      />
    </>
  );
}
