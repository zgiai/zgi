'use client';

import { useMemo } from 'react';

import { useT } from '@/i18n';
import { Card, CardContent } from '@/components/ui/card';
import type { ModelUsageByModelItem, ModelUsageSummary } from '@/services/types/statistics';
import { formatAiCreditValue } from '@/utils/ai-credits';
import { formatNumber } from '@/utils/format';

const CHART_COLORS = ['#3B82F6', '#F59E0B', '#22C55E'];

interface StatsCardsProps {
  summary: ModelUsageSummary;
  models: ModelUsageByModelItem[];
  showSourceBreakdown: boolean;
}

function formatShare(share: number): string {
  return `${formatNumber(share * 100, 2)}%`;
}

function TopModelsList({ models }: { models: Array<{ color: string; name: string; share: string }> }) {
  const t = useT('dashboard');

  return (
    <div className="space-y-1.5">
      {models.length > 0 ? (
        models.map(model => (
          <div key={model.name} className="flex items-center gap-2 text-xs">
            <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: model.color }} />
            <span className="truncate">{model.name}</span>
            <span className="ml-auto text-muted-foreground">{model.share}</span>
          </div>
        ))
      ) : (
        <div className="text-xs text-muted-foreground">{t('usage.cards.noModels')}</div>
      )}
    </div>
  );
}

export function StatsCards({ summary, models, showSourceBreakdown }: StatsCardsProps) {
  const t = useT('dashboard');

  const topModels = useMemo(() => {
    return models.slice(0, 3).map((model, index) => ({
      color: CHART_COLORS[index % CHART_COLORS.length],
      name: model.model_name,
      share: formatShare(model.points_share),
    }));
  }, [models]);

  return (
    <div className={`grid gap-4 md:grid-cols-2 ${showSourceBreakdown ? 'xl:grid-cols-5' : 'xl:grid-cols-4'}`}>
      <Card>
        <CardContent className="space-y-1.5 p-5">
          <div className="text-base">{t('usage.cards.attemptCount')}</div>
          <div className="text-3xl font-bold">{formatNumber(summary.attempt_count)}</div>
          <div className="space-y-0.5 text-sm text-muted-foreground">
            <div>
              {t('usage.cards.successCount')}: {formatNumber(summary.success_count)}
            </div>
            <div>
              {t('usage.cards.failedCount')}: {formatNumber(summary.failed_count)}
            </div>
            <div>
              {t('usage.cards.partialCount')}: {formatNumber(summary.partial_count)}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="space-y-1.5 p-5">
          <div className="text-base">{t('usage.cards.totalTokens')}</div>
          <div className="text-3xl font-bold">{formatNumber(summary.total_tokens, 2)}</div>
          <div className="space-y-0.5 text-sm text-muted-foreground">
            <div>
              {t('usage.cards.inputTokens')}: {formatNumber(summary.prompt_tokens, 2)}
            </div>
            <div>
              {t('usage.cards.outputTokens')}: {formatNumber(summary.completion_tokens, 2)}
            </div>
          </div>
        </CardContent>
      </Card>

      {showSourceBreakdown ? (
        <>
          <Card>
            <CardContent className="space-y-1.5 p-5">
              <div className="text-base">{t('usage.cards.officialPoints')}</div>
              <div className="text-3xl font-bold">{formatAiCreditValue(summary.official_points)}</div>
              <div className="text-sm text-muted-foreground">{t('usage.cards.officialPointsHint')}</div>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-1.5 p-5">
              <div className="text-base">{t('usage.cards.privatePoints')}</div>
              <div className="text-3xl font-bold">{formatAiCreditValue(summary.private_points)}</div>
              <div className="text-sm text-muted-foreground">{t('usage.cards.privatePointsHint')}</div>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-1.5 p-5">
              <div className="text-base">{t('usage.cards.totalPoints')}</div>
              <div className="text-3xl font-bold">{formatAiCreditValue(summary.total_points)}</div>
              <div className="mb-2 text-xs text-muted-foreground">
                {t('usage.cards.modelCount', { count: models.length })}
              </div>
              <TopModelsList models={topModels} />
            </CardContent>
          </Card>
        </>
      ) : (
        <>
          <Card>
            <CardContent className="space-y-1.5 p-5">
              <div className="text-base">{t('usage.cards.pointsConsumption')}</div>
              <div className="text-3xl font-bold">{formatAiCreditValue(summary.total_points)}</div>
              <div className="text-sm text-muted-foreground">{t('usage.cards.pointsConsumptionHint')}</div>
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-1.5 p-5">
              <div className="text-base">{t('usage.cards.topModels')}</div>
              <div className="text-3xl font-bold">{formatNumber(models.length)}</div>
              <div className="mb-2 text-xs text-muted-foreground">
                {t('usage.cards.modelCount', { count: models.length })}
              </div>
              <TopModelsList models={topModels} />
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
