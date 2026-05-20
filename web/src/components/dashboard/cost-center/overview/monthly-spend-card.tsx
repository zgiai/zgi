'use client';

import Link from 'next/link';
import { useT } from '@/i18n/translations';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useMonthlyStats } from '@/hooks/pay/use-monthly-stats';

export function MonthlySpendCard() {
  const t = useT('dashboard');
  const { data: monthlyStats, isLoading: isMonthlyStatsLoading } = useMonthlyStats();

  const stats = {
    monthlyCashConsumed: monthlyStats?.cash.total_consumed ?? 0,
    monthlyCreditsConsumed: monthlyStats?.credits.total_credits_consumed ?? 0,
    subscriptionCreditsConsumed: monthlyStats?.credits.subscription_credits_consumed ?? 0,
    purchasedCreditsConsumed: monthlyStats?.credits.purchased_credits_consumed ?? 0,
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
    <Card>
      <CardContent className="pt-6">
        <div className="flex items-center justify-between mb-2">
          <span className="text-sm text-muted-foreground">{t('costCenter.monthlySpend')}</span>
        </div>
        {isMonthlyStatsLoading ? (
          <>
            <Skeleton className="h-8 w-32 mb-1" />
            <Skeleton className="h-8 w-32 mb-2" />
            <Skeleton className="h-4 w-40 mb-1" />
            <Skeleton className="h-4 w-40" />
          </>
        ) : (
          <>
            <div className="text-2xl font-bold mb-1">
              ¥{stats.monthlyCashConsumed.toFixed(2)}
            </div>
            <div className="text-2xl font-bold mb-2">
              {formatCreditAmount(stats.monthlyCreditsConsumed)}
            </div>
            <div className="text-xs text-muted-foreground mb-1">
              {t('costCenter.stats.subscriptionConsumed')}
              {formatCreditAmount(stats.subscriptionCreditsConsumed)}
            </div>
            <div className="text-xs text-muted-foreground">
              {t('costCenter.stats.purchasedConsumed')}
              {formatCreditAmount(stats.purchasedCreditsConsumed)}
            </div>
          </>
        )}
        <Link href="/dashboard/cost-center/bills" className="block w-full mt-4">
          <Button variant="link" className="w-full text-blue-600 p-0 h-auto">
            {t('costCenter.viewDetails')} →
          </Button>
        </Link>
      </CardContent>
    </Card>
  );
}
