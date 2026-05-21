'use client';

import { useMemo, useState } from 'react';
import { ModelIcon } from 'modelicons';
import { Inbox } from 'lucide-react';
import { useLocale } from 'next-intl';

import { useT } from '@/i18n';
import { Card, CardContent } from '@/components/ui/card';
import { Progress } from '@/components/ui/progress';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { useCustomProviders, useProviders } from '@/hooks/provider/use-provider';
import { useProviderI18n } from '@/hooks/provider/use-provider-i18n';
import type { ProviderItem } from '@/services/types/provider';
import type { ModelUsageByModelItem } from '@/services/types/statistics';
import { formatAiCreditValue } from '@/utils/ai-credits';
import { formatNumber } from '@/utils/format';
import { resolveProviderCanonicalKey, resolveProviderDisplayInfo } from '@/utils/provider/meta';

const CHART_COLORS = ['#3B82F6', '#F59E0B', '#22C55E', '#EF4444', '#8B5CF6', '#06B6D4', '#EC4899', '#84CC16', '#FF4500', '#7E57C2'];

interface ModelDetailsSectionProps {
  models: ModelUsageByModelItem[];
  totalPoints: number;
  totalAttempts: number;
  totalTokens: number;
  promptTokens: number;
  completionTokens: number;
  showSourceBreakdown: boolean;
}

interface PieDatum {
  color: string;
  name: string;
  percentage: string;
  value: number;
}

interface ChartViewProps {
  attemptsData: PieDatum[];
  pointsData: PieDatum[];
  tokensData: PieDatum[];
  totalAttempts: number;
  totalPoints: number;
  totalTokens: number;
}

type DistributionMetric = 'points' | 'attempts' | 'tokens';

function getColorByIndex(index: number): string {
  return CHART_COLORS[index % CHART_COLORS.length];
}

function formatShare(share: number): string {
  return `${formatNumber(share * 100, 2)}%`;
}

function getProviderLookupKey(value?: string | null): string {
  return value?.trim().toLowerCase() || '';
}

function getProviderIdentifierLookupKey(value?: string | null): string {
  const rawValue = value?.trim();
  if (!rawValue) return '';

  const canonicalKey = resolveProviderCanonicalKey(rawValue);
  return canonicalKey === 'unknown' ? getProviderLookupKey(rawValue) : canonicalKey;
}

function resolveMatchedProvider(
  model: Pick<ModelUsageByModelItem, 'provider_id' | 'provider_name'>,
  providerIdentifierLookup: Map<string, ProviderItem>,
  providerNameLookup: Map<string, ProviderItem>
): ProviderItem | null {
  const identifierKeyFromId = getProviderIdentifierLookupKey(model.provider_id);
  const identifierKeyFromName = getProviderIdentifierLookupKey(model.provider_name);
  const displayNameKey = getProviderLookupKey(model.provider_name);

  return (
    providerIdentifierLookup.get(identifierKeyFromId) ||
    providerIdentifierLookup.get(identifierKeyFromName) ||
    providerNameLookup.get(displayNameKey) ||
    null
  );
}

export function ModelDetailsSection({
  models,
  totalPoints,
  totalAttempts,
  totalTokens,
  promptTokens,
  completionTokens,
  showSourceBreakdown,
}: ModelDetailsSectionProps) {
  const t = useT('dashboard');

  const pointsData = useMemo(
    () =>
      models.map((model, index) => ({
        color: getColorByIndex(index),
        name: model.model_name,
        percentage: formatShare(model.points_share),
        value: model.total_points,
      })),
    [models]
  );

  const attemptsData = useMemo(() => {
    const total = models.reduce((sum, model) => sum + model.attempt_count, 0) || 1;

    return models.map((model, index) => ({
      color: getColorByIndex(index),
      name: model.model_name,
      percentage: `${formatNumber((model.attempt_count / total) * 100, 2)}%`,
      value: model.attempt_count,
    }));
  }, [models]);

  const tokensData = useMemo(() => {
    const total = models.reduce((sum, model) => sum + model.total_tokens, 0) || 1;

    return models.map((model, index) => ({
      color: getColorByIndex(index),
      name: model.model_name,
      percentage: `${formatNumber((model.total_tokens / total) * 100, 2)}%`,
      value: model.total_tokens,
    }));
  }, [models]);

  return (
    <Card>
      <CardContent className="p-0">
        <div className="border-b px-6 py-6">
          <h2 className="text-lg font-semibold">{t('usage.modelDetails.title')}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{t('usage.modelDetails.subtitle')}</p>
        </div>

      <TableView models={models} showSourceBreakdown={showSourceBreakdown} />

        {models.length > 1 ? (
          <ChartView
            attemptsData={attemptsData}
            pointsData={pointsData}
            tokensData={tokensData}
            totalAttempts={totalAttempts}
            totalPoints={totalPoints}
            totalTokens={totalTokens}
          />
        ) : (
          <div className="border-t px-6 py-5 text-sm text-muted-foreground">
            {t('usage.cards.modelCount', { count: models.length })}
          </div>
        )}

        <div className="flex flex-col gap-3 border-t px-6 py-4 text-sm text-muted-foreground lg:flex-row lg:items-center lg:justify-between">
          <span>
            {t('usage.modelDetails.totalModels', {
              count: models.length,
              total: formatAiCreditValue(totalPoints),
            })}
          </span>
          <div className="flex flex-wrap gap-4">
            <span>{t('usage.modelDetails.inputTotal', { tokens: formatNumber(promptTokens, 2) })}</span>
            <span>{t('usage.modelDetails.outputTotal', { tokens: formatNumber(completionTokens, 2) })}</span>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function TableView({
  models,
  showSourceBreakdown,
}: {
  models: ModelUsageByModelItem[];
  showSourceBreakdown: boolean;
}) {
  const t = useT('dashboard');
  const locale = useLocale();
  const getProviderName = useProviderI18n();
  const { items: officialProviders } = useProviders({ limit: 200, refetchOnWindowFocus: false });
  const { items: customProviders } = useCustomProviders({ limit: 200, refetchOnWindowFocus: false });
  const columnCount = showSourceBreakdown ? 9 : 7;
  const centeredHeadClassName = 'px-3 text-center';
  const centeredCellClassName = 'px-3 text-center';
  const providers = useMemo(
    () => [...officialProviders, ...customProviders],
    [customProviders, officialProviders]
  );
  const providerIdentifierLookup = useMemo(() => {
    const lookup = new Map<string, (typeof providers)[number]>();

    providers.forEach(provider => {
      const key = getProviderIdentifierLookupKey(provider.provider);
      if (!key || lookup.has(key)) return;
      lookup.set(key, provider);
    });

    return lookup;
  }, [providers]);
  const providerNameLookup = useMemo(() => {
    const lookup = new Map<string, (typeof providers)[number]>();

    providers.forEach(provider => {
      const key = getProviderLookupKey(provider.provider_name);
      if (!key || lookup.has(key)) return;
      lookup.set(key, provider);
    });

    return lookup;
  }, [providers]);

  return (
    <div className="overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="min-w-[200px] px-6 text-left">{t('usage.modelDetails.model')}</TableHead>
            <TableHead className="min-w-[120px] px-3 text-center">{t('usage.modelDetails.provider')}</TableHead>
            <TableHead className={centeredHeadClassName}>{t('usage.modelDetails.attempts')}</TableHead>
            <TableHead className={centeredHeadClassName}>{t('usage.modelDetails.inputTokens')}</TableHead>
            <TableHead className={centeredHeadClassName}>{t('usage.modelDetails.outputTokens')}</TableHead>
            {showSourceBreakdown ? (
              <>
                <TableHead className={centeredHeadClassName}>{t('usage.modelDetails.officialPoints')}</TableHead>
                <TableHead className={centeredHeadClassName}>{t('usage.modelDetails.privatePoints')}</TableHead>
                <TableHead className={centeredHeadClassName}>{t('usage.modelDetails.totalPoints')}</TableHead>
              </>
            ) : (
              <TableHead className={centeredHeadClassName}>{t('usage.modelDetails.pointsConsumption')}</TableHead>
            )}
            <TableHead className="px-6 text-center">{t('usage.modelDetails.share')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {models.length === 0 ? (
            <TableRow>
              <TableCell colSpan={columnCount} className="py-10">
                <div className="flex flex-col items-center justify-center text-muted-foreground">
                  <Inbox className="mb-2 size-10" />
                  <div className="text-sm">{t('usage.modelDetails.empty')}</div>
                </div>
              </TableCell>
            </TableRow>
          ) : (
            models.map(model => (
              <TableRow key={`${model.provider_id}-${model.model_id}`}>
                <TableCell className="px-6 text-left">
                  <div className="flex items-center gap-3">
                    <ModelIcon model={model.model_name} size={28} />
                    <div className="font-medium">{model.model_name}</div>
                  </div>
                </TableCell>
                <TableCell className={centeredCellClassName}>
                  {
                    resolveProviderDisplayInfo(
                      resolveMatchedProvider(model, providerIdentifierLookup, providerNameLookup) ?? {
                        provider: model.provider_id,
                        provider_name: model.provider_name,
                      },
                      { locale, getProviderName }
                    ).name
                  }
                </TableCell>
                <TableCell className={centeredCellClassName}>{formatNumber(model.attempt_count)}</TableCell>
                <TableCell className={centeredCellClassName}>{formatNumber(model.prompt_tokens, 2)}</TableCell>
                <TableCell className={centeredCellClassName}>{formatNumber(model.completion_tokens, 2)}</TableCell>
                {showSourceBreakdown ? (
                  <>
                    <TableCell className={centeredCellClassName}>
                      {formatAiCreditValue(model.official_points)}
                    </TableCell>
                    <TableCell className={centeredCellClassName}>
                      {formatAiCreditValue(model.private_points)}
                    </TableCell>
                    <TableCell className={centeredCellClassName}>
                      {formatAiCreditValue(model.total_points)}
                    </TableCell>
                  </>
                ) : (
                  <TableCell className={centeredCellClassName}>
                    {formatAiCreditValue(model.total_points)}
                  </TableCell>
                )}
                <TableCell className="px-6 text-center font-medium">{formatShare(model.points_share)}</TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  );
}

function ChartView({ attemptsData, pointsData, tokensData, totalAttempts, totalPoints, totalTokens }: ChartViewProps) {
  const t = useT('dashboard');
  const [metric, setMetric] = useState<DistributionMetric>('points');
  const [showAll, setShowAll] = useState(false);

  const activeChart = useMemo(() => {
    if (metric === 'attempts') {
      return {
        data: attemptsData,
        title: t('usage.chart.attemptDistribution'),
        totalLabel: t('usage.chart.totalAttempts'),
        totalValue: formatNumber(totalAttempts),
        unitLabel: t('usage.chart.calls'),
        formatValue: (value: number) => formatNumber(value),
      };
    }

    if (metric === 'tokens') {
      return {
        data: tokensData,
        title: t('usage.chart.tokenDistribution'),
        totalLabel: t('usage.chart.totalTokens'),
        totalValue: formatNumber(totalTokens, 2),
        unitLabel: t('usage.chart.tokens'),
        formatValue: (value: number) => formatNumber(value, 2),
      };
    }

    return {
      data: pointsData,
      title: t('usage.chart.pointsDistribution'),
      totalLabel: t('usage.chart.totalPoints'),
      totalValue: formatAiCreditValue(totalPoints),
      unitLabel: t('usage.chart.points'),
      formatValue: (value: number) => formatAiCreditValue(value),
    };
  }, [attemptsData, metric, pointsData, t, tokensData, totalAttempts, totalPoints, totalTokens]);

  const rankedItems = useMemo(() => {
    const items = [...activeChart.data];
    if (showAll || items.length <= 5) {
      return items;
    }

    const visible = items.slice(0, 5);
    const hidden = items.slice(5);
    const hiddenValue = hidden.reduce((sum, item) => sum + item.value, 0);
    const hiddenShare = hidden.reduce(
      (sum, item) => sum + Number(item.percentage.replace('%', '')),
      0
    );

    if (hidden.length === 0) {
      return visible;
    }

    return [
      ...visible,
      {
        color: '#94A3B8',
        name: 'Others',
        percentage: `${formatNumber(hiddenShare, 2)}%`,
        value: hiddenValue,
      },
    ];
  }, [activeChart.data, showAll]);

  return (
    <div className="border-t px-6 py-6">
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h3 className="text-sm font-medium">{activeChart.title}</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('usage.modelDetails.subtitle')}
          </p>
        </div>
        <div className="inline-flex w-fit rounded-lg border p-1">
          {([
            ['points', t('usage.chart.totalPoints')],
            ['attempts', t('usage.chart.totalAttempts')],
            ['tokens', t('usage.chart.totalTokens')],
          ] as const).map(([nextMetric, label]) => (
            <button
              key={nextMetric}
              type="button"
              onClick={() => {
                setMetric(nextMetric);
                setShowAll(false);
              }}
              className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                metric === nextMetric
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground'
              }`}
            >
              {label}
            </button>
          ))}
        </div>
      </div>
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_260px]">
        <div className="space-y-3">
          {rankedItems.map(item => (
            <div key={`${metric}-${item.name}`} className="rounded-lg border border-border/70 bg-muted/10 px-4 py-3">
              <div className="mb-2 flex items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="h-3 w-3 shrink-0 rounded-full" style={{ backgroundColor: item.color }} />
                  <span className="truncate text-sm font-medium text-foreground">{item.name}</span>
                </div>
                <span className="shrink-0 text-xs font-medium text-muted-foreground">{item.percentage}</span>
              </div>
              <Progress value={Number(item.percentage.replace('%', ''))} className="h-2.5" />
              <div className="mt-2 text-xs text-muted-foreground">
                {activeChart.formatValue(item.value)} {activeChart.unitLabel}
              </div>
            </div>
          ))}
          {activeChart.data.length > 5 ? (
            <button
              type="button"
              onClick={() => setShowAll(prev => !prev)}
              className="text-xs font-medium text-primary transition-colors hover:text-primary/80"
            >
              {showAll ? '收起' : `查看全部 ${activeChart.data.length} 个模型`}
            </button>
          ) : null}
        </div>

        <div className="rounded-xl border border-border/70 bg-muted/10 p-5">
          <div className="text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
            {activeChart.totalLabel}
          </div>
          <div className="mt-3 text-4xl font-semibold tracking-tight text-primary">
            {activeChart.totalValue}
          </div>
          <div className="mt-2 text-sm text-muted-foreground">{activeChart.unitLabel}</div>
          {rankedItems[0] ? (
            <div className="mt-6 rounded-lg border border-border/70 bg-background/70 p-4">
              <div className="text-xs font-medium uppercase tracking-[0.08em] text-muted-foreground">
                Top 1
              </div>
              <div className="mt-2 flex items-center gap-2">
                <span
                  className="h-3 w-3 shrink-0 rounded-full"
                  style={{ backgroundColor: rankedItems[0].color }}
                />
                <span className="truncate text-sm font-medium text-foreground">
                  {rankedItems[0].name}
                </span>
              </div>
              <div className="mt-2 text-sm text-muted-foreground">
                {rankedItems[0].percentage} · {activeChart.formatValue(rankedItems[0].value)}{' '}
                {activeChart.unitLabel}
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
