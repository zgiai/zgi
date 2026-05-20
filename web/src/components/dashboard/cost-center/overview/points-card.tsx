'use client';

import { useT } from '@/i18n/translations';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import {
  useAiCredits,
  getOfficialBalance,
  getPrivateChannelTotal,
} from '@/hooks/pay/use-ai-credits';

export function PointsCard() {
  const t = useT('dashboard');
  const { data: aiCreditsData, isLoading: isAiCreditsLoading } = useAiCredits();

  const officialBalance = getOfficialBalance(aiCreditsData);
  const privateChannelTotal = getPrivateChannelTotal(aiCreditsData);

  return (
    <Card>
      <CardContent className="pt-6">
        <div className="flex items-center justify-between mb-2">
          <span className="text-sm text-muted-foreground">{t('costCenter.aiPoints')}</span>
        </div>
        {isAiCreditsLoading ? (
          <>
            <Skeleton className="h-8 w-40 mb-2" />
            <Skeleton className="h-5 w-full mb-1" />
            <Skeleton className="h-5 w-full" />
          </>
        ) : (
          <div className="flex flex-col gap-4 mt-4">
            <div className="flex items-center justify-between">
              <div>
                <div className="text-xs text-muted-foreground mb-1">
                  {t('costCenter.officialCredits')}
                </div>
                <div className="text-2xl font-bold text-blue-600">
                  {officialBalance.toLocaleString()}
                </div>
              </div>
            </div>
            <div className="h-px w-full bg-border" />
            <div className="flex items-center justify-between">
              <div>
                <div className="text-xs text-muted-foreground mb-1">
                  {t('costCenter.privateChannelCredits')}
                </div>
                <div className="text-2xl font-bold text-green-600">
                  {privateChannelTotal.toLocaleString()}
                </div>
              </div>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
