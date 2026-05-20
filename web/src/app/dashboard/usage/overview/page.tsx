'use client';

import { useMemo, useState } from 'react';
import { endOfDay, format, getUnixTime, parseISO, startOfDay, subDays } from 'date-fns';
import { RefreshCw } from 'lucide-react';

import { useT } from '@/i18n';
import { AppTypeDistributionSection } from '@/components/usage/app-type-distribution-section';
import { Badge } from '@/components/ui/badge';
import { ModelDetailsSection } from '@/components/usage/model-details-section';
import { StatsCards } from '@/components/usage/stats-cards';
import { TokenTrendChart } from '@/components/usage/token-trend-chart';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input, SearchInput } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useModelUsage } from '@/hooks/statistics';
import { IS_CLOUD } from '@/lib/config';
import { cn } from '@/lib/utils';
import type { ModelUsageAppType, ModelUsageSummary } from '@/services/types/statistics';

type DateRangeKey = 'last7Days' | 'last30Days' | 'last90Days' | 'custom';
type AppTypeFilter = 'all' | ModelUsageAppType;
type SourceFilter = 'all' | 'official' | 'private';

const DATE_RANGE_DAYS: Record<Exclude<DateRangeKey, 'custom'>, number> = {
  last7Days: 7,
  last30Days: 30,
  last90Days: 90,
};

const EMPTY_SUMMARY: ModelUsageSummary = {
  attempt_count: 0,
  success_count: 0,
  failed_count: 0,
  partial_count: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  total_tokens: 0,
  official_points: 0,
  private_points: 0,
  total_points: 0,
};

function clampDateString(value: string, min?: string, max?: string): string {
  if (!value) return '';
  let nextValue = value;

  if (min && nextValue < min) {
    nextValue = min;
  }

  if (max && nextValue > max) {
    nextValue = max;
  }

  return nextValue;
}

function SectionEyebrow({ children }: { children: React.ReactNode }) {
  return (
    <div className="mb-3 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
      {children}
    </div>
  );
}

export default function UsageOverviewPage() {
  const t = useT('dashboard');
  const tCommon = useT('common');
  const [dateRange, setDateRange] = useState<DateRangeKey>('last7Days');
  const [customStartDate, setCustomStartDate] = useState('');
  const [customEndDate, setCustomEndDate] = useState('');
  const [appType, setAppType] = useState<AppTypeFilter>('all');
  const [sourceFilter, setSourceFilter] = useState<SourceFilter>('all');
  const [modelNameInput, setModelNameInput] = useState('');
  const debouncedModelName = useDebouncedValue(modelNameInput.trim(), 300);
  const todayString = format(new Date(), 'yyyy-MM-dd');

  const isCustomRange = dateRange === 'custom';
  const isCustomRangeValid = !isCustomRange || Boolean(customStartDate && customEndDate);
  const showSourceBreakdown = IS_CLOUD && sourceFilter === 'all';
  const customStartMaxDate = customEndDate ? clampDateString(customEndDate, undefined, todayString) : todayString;
  const hasActiveFilters =
    dateRange !== 'last7Days' ||
    appType !== 'all' ||
    sourceFilter !== 'all' ||
    Boolean(modelNameInput.trim());

  const handleCustomStartDateChange = (value: string) => {
    const nextStartDate = clampDateString(value, undefined, customStartMaxDate);
    setCustomStartDate(nextStartDate);
  };

  const handleCustomEndDateChange = (value: string) => {
    const nextEndDate = clampDateString(value, customStartDate || undefined, todayString);
    setCustomEndDate(nextEndDate);
  };

  const params = useMemo(() => {
    let startDate = new Date(0);
    let endDate = new Date(0);

    if (isCustomRange) {
      if (customStartDate && customEndDate) {
        startDate = startOfDay(parseISO(customStartDate));
        endDate = endOfDay(parseISO(customEndDate));
      }
    } else {
      const days = DATE_RANGE_DAYS[dateRange];
      const now = new Date();
      startDate = startOfDay(subDays(now, days));
      endDate = endOfDay(now);
    }

    return {
      start_time: getUnixTime(startDate),
      end_time: getUnixTime(endDate),
      app_type: appType === 'all' ? undefined : appType,
      model_name: debouncedModelName || undefined,
      use_system_provider:
        IS_CLOUD && sourceFilter !== 'all' ? sourceFilter === 'official' : undefined,
    };
  }, [appType, customEndDate, customStartDate, dateRange, debouncedModelName, isCustomRange, sourceFilter]);

  const { data, isLoading, isFetching, refetch } = useModelUsage(params, {
    enabled: isCustomRangeValid,
  });
  const summary = data?.summary ?? EMPTY_SUMMARY;
  const activeFilterBadges = useMemo(() => {
    const badges: string[] = [];

    badges.push(
      isCustomRange && customStartDate && customEndDate
        ? `${customStartDate} ~ ${customEndDate}`
        : t(`usage.dateRange.${dateRange}`)
    );

    if (appType !== 'all') {
      badges.push(t(`usage.appTypes.${appType}`));
    }

    if (IS_CLOUD && sourceFilter !== 'all') {
      badges.push(
        t(`usage.filters.source${sourceFilter === 'official' ? 'Official' : 'Private'}`)
      );
    }

    if (modelNameInput.trim()) {
      badges.push(`${t('usage.filters.modelName')}: ${modelNameInput.trim()}`);
    }

    return badges;
  }, [appType, customEndDate, customStartDate, dateRange, isCustomRange, modelNameInput, sourceFilter, t]);

  const resetFilters = () => {
    setDateRange('last7Days');
    setCustomStartDate('');
    setCustomEndDate('');
    setAppType('all');
    setSourceFilter('all');
    setModelNameInput('');
  };

  return (
    <div className="flex h-full flex-col overflow-auto">
      <div className="space-y-6 p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h1 className="text-2xl font-semibold">{t('usage.title')}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{t('usage.subtitle')}</p>
          </div>
        </div>

        <Card className="border-border/80 shadow-sm">
          <CardHeader className="pb-3">
            <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
              <div>
                <SectionEyebrow>{tCommon('filter')}</SectionEyebrow>
                <CardTitle className="text-base">{t('usage.filters.appType')}</CardTitle>
                <CardDescription>选择时间范围、应用类型和模型后查看对应数据。</CardDescription>
              </div>
              <div className="flex items-center gap-2 self-start">
                {hasActiveFilters ? (
                  <Button variant="ghost" size="sm" onClick={resetFilters}>
                    {tCommon('resetFilters')}
                  </Button>
                ) : null}
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => refetch()}
                  disabled={isFetching || !isCustomRangeValid}
                  className="gap-2"
                >
                  <RefreshCw className={cn('h-4 w-4', isFetching && 'animate-spin')} />
                  {tCommon('refresh')}
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap items-center gap-3">
              <Select value={dateRange} onValueChange={value => setDateRange(value as DateRangeKey)}>
                <SelectTrigger className="w-[150px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="last7Days">{t('usage.dateRange.last7Days')}</SelectItem>
                  <SelectItem value="last30Days">{t('usage.dateRange.last30Days')}</SelectItem>
                  <SelectItem value="last90Days">{t('usage.dateRange.last90Days')}</SelectItem>
                  <SelectItem value="custom">{t('usage.dateRange.custom')}</SelectItem>
                </SelectContent>
              </Select>

              <Select value={appType} onValueChange={value => setAppType(value as AppTypeFilter)}>
                <SelectTrigger className="w-[170px]">
                  <SelectValue placeholder={t('usage.filters.appTypePlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t('usage.filters.appTypePlaceholder')}</SelectItem>
                  <SelectItem value="workflow">{t('usage.appTypes.workflow')}</SelectItem>
                  <SelectItem value="dataset">{t('usage.appTypes.dataset')}</SelectItem>
                  <SelectItem value="agent">{t('usage.appTypes.agent')}</SelectItem>
                  <SelectItem value="aichat">{t('usage.appTypes.aichat')}</SelectItem>
                  <SelectItem value="unknown">{t('usage.appTypes.unknown')}</SelectItem>
                </SelectContent>
              </Select>

              {IS_CLOUD ? (
                <Select value={sourceFilter} onValueChange={value => setSourceFilter(value as SourceFilter)}>
                  <SelectTrigger className="w-[170px]">
                    <SelectValue placeholder={t('usage.filters.sourcePlaceholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t('usage.filters.sourceAll')}</SelectItem>
                    <SelectItem value="official">{t('usage.filters.sourceOfficial')}</SelectItem>
                    <SelectItem value="private">{t('usage.filters.sourcePrivate')}</SelectItem>
                  </SelectContent>
                </Select>
              ) : null}

              <SearchInput
                value={modelNameInput}
                onChange={event => setModelNameInput(event.target.value)}
                placeholder={t('usage.filters.modelNamePlaceholder')}
                className="w-full min-w-[220px] sm:w-[260px]"
              />
            </div>

            <div className="flex flex-wrap items-center gap-2 border-t border-border/60 pt-3">
              {activeFilterBadges.map(badge => (
                <Badge key={badge} variant="subtle" className="rounded-full px-2">
                  {badge}
                </Badge>
              ))}
              <span className="ml-auto text-xs text-muted-foreground">
                {isLoading
                  ? tCommon('loading')
                  : `当前结果共 ${data?.by_model?.length ?? 0} 个模型`}
              </span>
            </div>

            {isCustomRange ? (
              <div className="grid gap-3 sm:grid-cols-2 lg:max-w-lg">
                <label className="flex flex-col gap-1.5">
                  <span className="text-xs font-medium text-muted-foreground">
                    {t('usage.filters.startDate')}
                  </span>
                  <Input
                    type="date"
                    value={customStartDate}
                    onChange={event => handleCustomStartDateChange(event.target.value)}
                    max={customStartMaxDate}
                  />
                </label>
                <label className="flex flex-col gap-1.5">
                  <span className="text-xs font-medium text-muted-foreground">
                    {t('usage.filters.endDate')}
                  </span>
                  <Input
                    type="date"
                    value={customEndDate}
                    onChange={event => handleCustomEndDateChange(event.target.value)}
                    min={customStartDate || undefined}
                    max={todayString}
                  />
                </label>
              </div>
            ) : null}
          </CardContent>
        </Card>

        {isLoading ? (
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
            {Array.from({ length: showSourceBreakdown ? 5 : 4 }).map((_, index) => (
              <Card key={index}>
                <CardContent className="p-5">
                  <Skeleton className="mb-3 h-4 w-24" />
                  <Skeleton className="mb-3 h-8 w-28" />
                  <Skeleton className="h-4 w-full" />
                </CardContent>
              </Card>
            ))}
          </div>
        ) : (
          <StatsCards summary={summary} models={data?.by_model ?? []} showSourceBreakdown={showSourceBreakdown} />
        )}

        {isLoading ? (
          <LoadingSection />
        ) : (
          <TokenTrendChart dailyData={data?.daily_trend ?? []} showSourceBreakdown={showSourceBreakdown} />
        )}

        {isLoading ? (
          <LoadingSection />
        ) : (
          <ModelDetailsSection
            models={data?.by_model ?? []}
            totalPoints={summary.total_points}
            totalAttempts={summary.attempt_count}
            totalTokens={summary.total_tokens}
            promptTokens={summary.prompt_tokens}
            completionTokens={summary.completion_tokens}
            showSourceBreakdown={showSourceBreakdown}
          />
        )}

        {isLoading ? <LoadingSection /> : <AppTypeDistributionSection items={data?.by_app_type ?? []} />}
      </div>
    </div>
  );
}

function LoadingSection() {
  return (
    <Card>
      <CardContent className="p-6">
        <Skeleton className="mb-2 h-6 w-48" />
        <Skeleton className="mb-6 h-4 w-64" />
        <Skeleton className="h-[300px] w-full" />
      </CardContent>
    </Card>
  );
}
