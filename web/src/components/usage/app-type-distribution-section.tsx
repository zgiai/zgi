'use client';

import { useMemo } from 'react';
import { PieChart, Pie, Cell, ResponsiveContainer, Tooltip } from 'recharts';
import { Inbox } from 'lucide-react';

import { useT } from '@/i18n';
import { Card, CardContent } from '@/components/ui/card';
import type { ModelUsageAppType, ModelUsageByAppTypeItem } from '@/services/types/statistics';
import { formatAiCreditValue } from '@/utils/ai-credits';
import { formatNumber } from '@/utils/format';

const COLORS = ['#0F766E', '#2563EB', '#CA8A04', '#9333EA'];

interface AppTypeDistributionSectionProps {
  items: ModelUsageByAppTypeItem[];
}

function formatShare(share: number): string {
  return `${formatNumber(share * 100, 2)}%`;
}

export function AppTypeDistributionSection({ items }: AppTypeDistributionSectionProps) {
  const t = useT('dashboard');

  const chartData = useMemo(() => {
    return items.map((item, index) => ({
      attempts: item.attempt_count,
      color: COLORS[index % COLORS.length],
      label: t(`usage.appTypes.${item.app_type}` as `usage.appTypes.${ModelUsageAppType}`),
      percentage: formatShare(item.points_share),
      value: item.total_points,
    }));
  }, [items, t]);

  return (
    <Card>
      <CardContent className="p-6">
        <div className="mb-6 flex flex-col gap-1">
          <h2 className="text-lg font-semibold">{t('usage.appTypeDistribution.title')}</h2>
          <p className="text-sm text-muted-foreground">{t('usage.appTypeDistribution.subtitle')}</p>
        </div>

        {chartData.length === 0 ? (
          <div className="flex h-[260px] flex-col items-center justify-center rounded-lg border border-dashed text-muted-foreground">
            <Inbox className="mb-2 size-10" />
            <div className="text-sm">{t('usage.appTypeDistribution.empty')}</div>
          </div>
        ) : (
          <div className="grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
            <div className="relative h-[260px]">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie data={chartData} cx="50%" cy="50%" innerRadius={70} outerRadius={105} dataKey="value">
                    {chartData.map((entry, index) => (
                      <Cell key={`${entry.label}-${index}`} fill={entry.color} />
                    ))}
                  </Pie>
                  <Tooltip
                    wrapperStyle={{ zIndex: 30 }}
                    content={({ active, payload }) => {
                      if (!active || !payload?.length) return null;
                      const datum = payload[0].payload as (typeof chartData)[number];

                      return (
                        <div className="rounded-lg border bg-popover p-3 text-sm shadow-md">
                          <div className="font-medium">{datum.label}</div>
                          <div className="text-muted-foreground">
                            {t('usage.chart.totalPointsSeries')}: {formatAiCreditValue(datum.value)}{' '}
                            {t('usage.chart.points')} ({datum.percentage})
                          </div>
                        </div>
                      );
                    }}
                  />
                </PieChart>
              </ResponsiveContainer>
              <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
                <div className="text-2xl font-bold text-primary">
                  {formatAiCreditValue(items.reduce((sum, item) => sum + item.total_points, 0))}
                </div>
                <div className="text-sm text-muted-foreground">{t('usage.chart.totalPoints')}</div>
              </div>
            </div>

            <div className="space-y-3">
              {chartData.map(item => (
                <div key={item.label} className="rounded-lg border bg-card p-4">
                  <div className="mb-2 flex items-center gap-2 text-sm font-medium">
                    <span className="h-3 w-3 rounded-full" style={{ backgroundColor: item.color }} />
                    <span>{item.label}</span>
                    <span className="ml-auto text-muted-foreground">{item.percentage}</span>
                  </div>
                  <div className="flex flex-wrap gap-4 text-sm text-muted-foreground">
                    <span>
                      {t('usage.appTypeDistribution.totalPoints')}: {formatAiCreditValue(item.value)}
                    </span>
                    <span>
                      {t('usage.appTypeDistribution.attempts')}: {formatNumber(item.attempts)}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
