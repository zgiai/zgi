'use client';

import { useMemo, useState } from 'react';
import { format, parseISO } from 'date-fns';
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';

import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import type { ModelUsageDailyItem } from '@/services/types/statistics';
import { formatAiCreditValue } from '@/utils/ai-credits';
import { formatNumber } from '@/utils/format';

interface TokenTrendChartProps {
  dailyData: ModelUsageDailyItem[];
  showSourceBreakdown: boolean;
}

interface StackedPointsShapeProps {
  fill?: string;
  height?: number;
  payload?: {
    officialPoints: number;
    privatePoints: number;
    totalPoints: number;
  };
  width?: number;
  x?: number;
  y?: number;
}

type ChartMode = 'points' | 'tokens';

const COLORS = {
  official: '#3B82F6',
  private: '#F59E0B',
  points: '#0F766E',
  tokens: '#2563EB',
};

function StackedPointsBarShape({ x = 0, y = 0, width = 0, height = 0, payload, fill = COLORS.points }: StackedPointsShapeProps) {
  const officialPoints = payload?.officialPoints ?? 0;
  const privatePoints = payload?.privatePoints ?? 0;
  const totalPoints = payload?.totalPoints ?? 0;

  if (width <= 0 || height <= 0 || totalPoints <= 0) {
    return null;
  }

  const splitTotal = officialPoints + privatePoints;

  if (splitTotal <= 0) {
    return <rect x={x} y={y} width={width} height={height} rx={4} ry={4} fill={fill} />;
  }

  const privateHeight = Math.max(0, (height * privatePoints) / splitTotal);
  const officialHeight = Math.max(0, height - privateHeight);

  return (
    <g>
      {privateHeight > 0 ? (
        <rect
          x={x}
          y={y + height - privateHeight}
          width={width}
          height={privateHeight}
          fill={COLORS.private}
        />
      ) : null}
      {officialHeight > 0 ? (
        <rect
          x={x}
          y={y}
          width={width}
          height={officialHeight}
          fill={COLORS.official}
        />
      ) : null}
    </g>
  );
}

export function TokenTrendChart({ dailyData, showSourceBreakdown }: TokenTrendChartProps) {
  const t = useT('dashboard');
  const [chartMode, setChartMode] = useState<ChartMode>('points');

  const chartData = useMemo(
    () =>
      dailyData.map(item => {
        const officialPoints = item.official_points;
        const privatePoints = item.private_points;
        const totalPoints = Math.max(item.total_points, officialPoints + privatePoints);

        return {
          date: format(parseISO(item.date), 'MM-dd'),
          fullDate: format(parseISO(item.date), 'yyyy-MM-dd'),
          officialPoints,
          privatePoints,
          totalPoints,
          totalTokens: item.total_tokens,
          attemptCount: item.attempt_count,
          successCount: item.success_count,
          failedCount: item.failed_count,
          partialCount: item.partial_count,
          hasPointBreakdown: officialPoints > 0 || privatePoints > 0,
        };
      }),
    [dailyData]
  );

  const hasPointBreakdownData = useMemo(
    () => showSourceBreakdown && chartData.some(item => item.hasPointBreakdown),
    [chartData, showSourceBreakdown]
  );

  const totalValue = useMemo(
    () => chartData.reduce((sum, item) => sum + (chartMode === 'points' ? item.totalPoints : item.totalTokens), 0),
    [chartData, chartMode]
  );

  const seriesLabel =
    chartMode === 'points'
      ? hasPointBreakdownData
        ? t('usage.chart.totalPointsSeries')
        : t('usage.chart.pointConsumptionSeries')
      : t('usage.chart.totalTokensSeries');

  const unitLabel = chartMode === 'points' ? t('usage.chart.points') : t('usage.chart.tokens');
  const showStackedPoints = chartMode === 'points' && hasPointBreakdownData;

  return (
    <Card>
      <CardContent className="p-6">
        <div className="mb-6 flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h2 className="text-lg font-semibold">{t('usage.chart.dailyTrend')}</h2>
            <p className="mt-1 text-sm text-muted-foreground">{t('usage.chart.dailyTrendSubtitle')}</p>
          </div>
          <div className="flex flex-wrap gap-2 rounded-lg border p-1">
            <Button
              variant="ghost"
              size="sm"
              className={cn(
                'px-3 text-xs',
                chartMode === 'points' &&
                  'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground'
              )}
              onClick={() => setChartMode('points')}
            >
              {hasPointBreakdownData ? t('usage.chart.totalPointsSeries') : t('usage.chart.pointConsumptionSeries')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className={cn(
                'px-3 text-xs',
                chartMode === 'tokens' &&
                  'bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground'
              )}
              onClick={() => setChartMode('tokens')}
            >
              {t('usage.chart.totalTokensSeries')}
            </Button>
          </div>
        </div>

        {chartData.length === 0 ? (
          <div className="flex h-[300px] items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">
            {t('usage.empty')}
          </div>
        ) : (
          <div className="h-[300px]">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={chartData} margin={{ top: 20, right: 30, left: 20, bottom: 20 }}>
                <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#E5E7EB" />
                <XAxis
                  dataKey="date"
                  axisLine={false}
                  tickLine={false}
                  tick={{ fontSize: 12, fill: '#6B7280' }}
                  dy={10}
                />
                <YAxis
                  axisLine={false}
                  tickLine={false}
                  tick={{ fontSize: 12, fill: '#6B7280' }}
                  tickFormatter={value =>
                    chartMode === 'points' ? formatAiCreditValue(value) : formatNumber(value, 2) || '0'
                  }
                  dx={-10}
                />
                <Tooltip
                  content={({ active, payload }) => {
                    if (!active || !payload?.length) return null;
                    const data = payload[0].payload as (typeof chartData)[number];

                    return (
                      <div className="rounded-lg border bg-popover p-3 text-sm shadow-md">
                        <div className="mb-1 font-medium">{data.fullDate}</div>
                        {showStackedPoints ? (
                          <div className="space-y-1 text-muted-foreground">
                            <div>
                              {t('usage.filters.sourceOfficial')}: {formatAiCreditValue(data.officialPoints)}{' '}
                              {t('usage.chart.points')}
                            </div>
                            <div>
                              {t('usage.filters.sourcePrivate')}: {formatAiCreditValue(data.privatePoints)}{' '}
                              {t('usage.chart.points')}
                            </div>
                            <div>
                              {t('usage.chart.totalPointsSeries')}: {formatAiCreditValue(data.totalPoints)}{' '}
                              {t('usage.chart.points')}
                            </div>
                            <div>
                              {t('usage.cards.attemptCount')}: {formatNumber(data.attemptCount)}
                            </div>
                            <div>
                              {t('usage.cards.successCount')}: {formatNumber(data.successCount)} ·{' '}
                              {t('usage.cards.failedCount')}: {formatNumber(data.failedCount)}
                            </div>
                            {data.partialCount > 0 ? (
                              <div>
                                {t('usage.cards.partialCount')}: {formatNumber(data.partialCount)}
                              </div>
                            ) : null}
                          </div>
                        ) : (
                          <div className="space-y-1 text-muted-foreground">
                            <div>
                              {seriesLabel}:{' '}
                              {chartMode === 'points'
                                ? formatAiCreditValue(data.totalPoints)
                                : formatNumber(data.totalTokens, 2)}{' '}
                              {unitLabel}
                            </div>
                            <div>
                              {t('usage.cards.attemptCount')}: {formatNumber(data.attemptCount)}
                            </div>
                            <div>
                              {t('usage.cards.successCount')}: {formatNumber(data.successCount)} ·{' '}
                              {t('usage.cards.failedCount')}: {formatNumber(data.failedCount)}
                            </div>
                            {data.partialCount > 0 ? (
                              <div>
                                {t('usage.cards.partialCount')}: {formatNumber(data.partialCount)}
                              </div>
                            ) : null}
                          </div>
                        )}
                      </div>
                    );
                  }}
                />
                {showStackedPoints ? (
                  <Bar
                    dataKey="totalPoints"
                    fill={COLORS.points}
                    radius={[4, 4, 0, 0]}
                    maxBarSize={60}
                    minPointSize={2}
                    shape={(props: StackedPointsShapeProps) => <StackedPointsBarShape {...props} />}
                  />
                ) : (
                  <Bar
                    dataKey={chartMode === 'points' ? 'totalPoints' : 'totalTokens'}
                    fill={chartMode === 'points' ? COLORS.points : COLORS.tokens}
                    radius={[4, 4, 0, 0]}
                    maxBarSize={60}
                    minPointSize={2}
                  />
                )}
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}

        <div className="mt-4 flex flex-col gap-3 border-t pt-4 text-sm lg:flex-row lg:items-center lg:justify-between">
          {showStackedPoints ? (
            <div className="flex flex-wrap items-center gap-4">
              <LegendItem color={COLORS.official} label={t('usage.filters.sourceOfficial')} />
              <LegendItem color={COLORS.private} label={t('usage.filters.sourcePrivate')} />
            </div>
          ) : (
            <LegendItem
              color={chartMode === 'points' ? COLORS.points : COLORS.tokens}
              label={seriesLabel}
            />
          )}
          <span className="text-muted-foreground">
            {t('usage.chart.totalLabel', {
              count: chartMode === 'points' ? formatAiCreditValue(totalValue) : formatNumber(totalValue, 2),
            })}{' '}
            {unitLabel}
          </span>
        </div>
      </CardContent>
    </Card>
  );
}

function LegendItem({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-2">
      <span className="h-3 w-3 rounded" style={{ backgroundColor: color }} />
      <span>{label}</span>
    </div>
  );
}
