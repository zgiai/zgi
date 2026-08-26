'use client';

import { useEffect, useRef, useState } from 'react';
import { useLocale } from 'next-intl';

import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { useInvocationContent, useInvocationLog } from '@/hooks/statistics';
import type {
  InvocationSource,
  InvocationStatus,
  ModelUsageAppType,
  InvocationLogItem,
  InvocationLogCursor,
  InvocationLogSummary,
  InvocationPricingDetails,
} from '@/services/types/statistics';
import {
  formatBillingDisplayAmountFromNormalizedCredits,
  formatRecordedBillingAmount,
  formatRecordedBillingAmountFromUSD,
  DEFAULT_BILLING_DISPLAY,
  type BillingDisplaySettings,
} from '@/utils/billing-display';
import { formatAiCreditValue } from '@/utils/ai-credits';
import { formatNumber } from '@/utils/format';
import { normalizeModelUsageAppType } from '@/utils/model-usage-app-type';
import { formatTokenCount } from '@/utils/token-format';

type SourceFilter = 'all' | Exclude<InvocationSource, 'unknown'>;

interface InvocationLogSectionProps {
  startTime: number;
  endTime: number;
  appType?: string;
  modelName?: string;
  enabled: boolean;
  billingDisplay: BillingDisplaySettings;
  refreshToken?: number;
  canViewContent?: boolean;
}

const sourceFilters: SourceFilter[] = ['all', 'api', 'product'];
const invocationPageSizes = [20, 50, 100] as const;
export function InvocationLogSection({
  startTime,
  endTime,
  appType,
  modelName,
  enabled,
  billingDisplay,
  refreshToken = 0,
  canViewContent = false,
}: InvocationLogSectionProps) {
  const t = useT('dashboard');
  const locale = useLocale();
  const [source, setSource] = useState<SourceFilter>('all');
  const [pageSize, setPageSize] = useState<number>(20);
  const [selectedInvocation, setSelectedInvocation] = useState<InvocationLogItem | null>(null);
  const paginationKey = `${startTime}:${endTime}:${source}:${appType ?? ''}:${modelName ?? ''}:${pageSize}`;
  const [pagination, setPagination] = useState<{
    key: string;
    pageIndex: number;
    cursors: Array<InvocationLogCursor | undefined>;
  }>({ key: paginationKey, pageIndex: 0, cursors: [undefined] });
  const activePagination =
    pagination.key === paginationKey
      ? pagination
      : { key: paginationKey, pageIndex: 0, cursors: [undefined] };
  useEffect(() => {
    if (pagination.key === paginationKey) return;
    setPagination({ key: paginationKey, pageIndex: 0, cursors: [undefined] });
  }, [pagination.key, paginationKey]);
  const cursor = activePagination.cursors[activePagination.pageIndex];
  const query = useInvocationLog(
    {
      start_time: startTime,
      end_time: endTime,
      invocation_source: source === 'all' ? undefined : source,
      app_type: appType,
      model_name: modelName,
      cursor_time: cursor?.time,
      cursor_id: cursor?.id,
      limit: pageSize,
      include_summary: activePagination.pageIndex === 0,
    },
    enabled
  );
  const refetch = query.refetch;
  const lastRefreshToken = useRef(refreshToken);
  useEffect(() => {
    if (lastRefreshToken.current === refreshToken) return;
    lastRefreshToken.current = refreshToken;
    void refetch();
  }, [refetch, refreshToken]);
  const [summaryCache, setSummaryCache] = useState<{
    key: string;
    summary: InvocationLogSummary;
  } | null>(null);
  useEffect(() => {
    if (activePagination.pageIndex !== 0 || !query.data?.summary) return;
    setSummaryCache({ key: paginationKey, summary: query.data.summary });
  }, [activePagination.pageIndex, paginationKey, query.data?.summary]);
  const summary =
    activePagination.pageIndex === 0
      ? query.data?.summary
      : summaryCache?.key === paginationKey
        ? summaryCache.summary
        : undefined;
  const hasInitialLoadError = query.isError && !query.data;
  const items = query.data?.items ?? [];
  const total = summary?.invocation_count ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const pageStart = total === 0 ? 0 : activePagination.pageIndex * pageSize + 1;
  const pageEnd = Math.min((activePagination.pageIndex + 1) * pageSize, total);
  const nextCursor = query.data?.next_cursor;
  const goToPreviousPage = () => {
    setPagination(current => ({ ...current, pageIndex: Math.max(0, current.pageIndex - 1) }));
  };
  const goToNextPage = () => {
    if (!nextCursor) return;
    setPagination(current => {
      const cursors = current.cursors.slice(0, current.pageIndex + 1);
      cursors[current.pageIndex + 1] = nextCursor;
      return { ...current, pageIndex: current.pageIndex + 1, cursors };
    });
  };
  const formatCost = (value: number) =>
    formatBillingDisplayAmountFromNormalizedCredits(value, DEFAULT_BILLING_DISPLAY, { locale });

  return (
    <Card className="border-border/80 shadow-sm">
      <CardHeader className="gap-4 pb-4">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <CardTitle>{t('usage.invocations.title')}</CardTitle>
            <CardDescription className="mt-1">{t('usage.invocations.description')}</CardDescription>
          </div>
          <div className="flex w-fit rounded-lg border bg-muted/30 p-1">
            {sourceFilters.map(value => (
              <Button
                key={value}
                type="button"
                size="sm"
                variant={source === value ? 'secondary' : 'ghost'}
                className="h-8 px-3"
                onClick={() => setSource(value)}
              >
                {t(`usage.invocations.sources.${value}`)}
              </Button>
            ))}
          </div>
        </div>

        {hasInitialLoadError ? (
          <div
            role="alert"
            className="flex flex-col gap-3 rounded-lg border border-destructive/40 bg-destructive/5 p-4 sm:flex-row sm:items-center sm:justify-between"
          >
            <div>
              <p className="text-sm font-medium text-destructive">
                {t('usage.invocations.loadFailed')}
              </p>
              <p className="mt-1 text-sm text-muted-foreground">
                {t('usage.invocations.loadFailedDescription')}
              </p>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="shrink-0"
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              {t(query.isFetching ? 'usage.invocations.retrying' : 'usage.invocations.retry')}
            </Button>
          </div>
        ) : query.isLoading ? (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            {Array.from({ length: 4 }).map((_, index) => (
              <Skeleton key={index} className="h-20 rounded-xl" />
            ))}
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <SummaryMetric
              label={t('usage.invocations.metrics.calls')}
              value={formatNumber(summary?.invocation_count ?? 0, 0)}
            />
            <SummaryMetric
              label={t('usage.invocations.metrics.api')}
              value={formatNumber(summary?.api_count ?? 0, 0)}
            />
            <SummaryMetric
              label={t('usage.invocations.metrics.product')}
              value={formatNumber(summary?.product_count ?? 0, 0)}
            />
            <SummaryMetric
              label={t('usage.invocations.metrics.tokensAndCost')}
              value={formatTokenCount(summary?.total_tokens ?? 0, locale)}
              detail={formatCost(summary?.total_points ?? 0)}
            />
          </div>
        )}
        {!query.isLoading && (summary?.unknown_count ?? 0) > 0 ? (
          <p className="text-xs text-muted-foreground">
            {t('usage.invocations.unknownHint', { count: summary?.unknown_count ?? 0 })}
          </p>
        ) : null}
      </CardHeader>

      {!hasInitialLoadError ? (
        <CardContent className="px-0 pb-0">
          <div className="overflow-x-auto border-t">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="min-w-[156px] pl-6">
                    {t('usage.invocations.table.time')}
                  </TableHead>
                  <TableHead>{t('usage.invocations.table.source')}</TableHead>
                  <TableHead className="min-w-[180px]">
                    {t('usage.invocations.table.model')}
                  </TableHead>
                  <TableHead>{t('usage.invocations.table.channel')}</TableHead>
                  <TableHead>{t('usage.invocations.table.business')}</TableHead>
                  <TableHead>{t('usage.invocations.table.status')}</TableHead>
                  <TableHead>{t('usage.invocations.table.content')}</TableHead>
                  <TableHead className="text-right">
                    {t('usage.invocations.table.tokens')}
                  </TableHead>
                  <TableHead className="text-right">
                    {t('usage.invocations.table.duration')}
                  </TableHead>
                  <TableHead className="text-right">{t('usage.invocations.table.cost')}</TableHead>
                  <TableHead className="pr-6 text-right">
                    {t('usage.invocations.table.details')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {query.isLoading ? (
                  Array.from({ length: 5 }).map((_, index) => (
                    <TableRow key={index}>
                      <TableCell colSpan={11} className="px-6 py-3">
                        <Skeleton className="h-8 w-full" />
                      </TableCell>
                    </TableRow>
                  ))
                ) : items.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={11} className="h-28 text-center text-muted-foreground">
                      {t('usage.invocations.empty')}
                    </TableCell>
                  </TableRow>
                ) : (
                  items.map(item => (
                    <TableRow key={item.invocation_id}>
                      <TableCell className="pl-6 tabular-nums">
                        <div>{new Date(item.started_at).toLocaleString(locale)}</div>
                        <div
                          className="mt-0.5 max-w-[132px] truncate font-mono text-[11px] text-muted-foreground"
                          title={item.invocation_id}
                        >
                          {item.invocation_id}
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            item.invocation_source === 'api'
                              ? 'info'
                              : item.invocation_source === 'product'
                                ? 'secondary'
                                : 'subtle'
                          }
                        >
                          {t(`usage.invocations.sources.${item.invocation_source}`)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className="font-medium">{item.model_name}</div>
                        <div className="text-xs text-muted-foreground">{item.provider_name}</div>
                      </TableCell>
                      <TableCell>{item.channel_name || '-'}</TableCell>
                      <TableCell>{t(`usage.appTypes.${knownAppType(item.app_type)}`)}</TableCell>
                      <TableCell>
                        <StatusBadge status={item.status} attempts={item.attempt_count} />
                      </TableCell>
                      <TableCell>
                        <Badge variant={item.content_available ? 'success' : 'subtle'}>
                          {t(
                            `usage.invocations.contentStatus.${
                              item.content_available ? 'available' : 'unavailable'
                            }`
                          )}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatTokenCount(item.total_tokens, locale)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatDuration(item.duration_ms)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatInvocationCost(item, billingDisplay, locale)}
                      </TableCell>
                      <TableCell className="pr-6 text-right">
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          onClick={() => setSelectedInvocation(item)}
                        >
                          {t('usage.invocations.details.action')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
          {total > 0 ? (
            <div className="flex flex-col gap-3 border-t p-4 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <span>
                  {t('usage.invocations.pagination.summary', {
                    start: pageStart,
                    end: pageEnd,
                    total,
                  })}
                </span>
                <Select
                  value={String(pageSize)}
                  onValueChange={value => setPageSize(Number(value))}
                >
                  <SelectTrigger
                    className="h-8 w-[116px]"
                    aria-label={t('usage.invocations.pagination.pageSizeLabel')}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {invocationPageSizes.map(size => (
                      <SelectItem key={size} value={String(size)}>
                        {t('usage.invocations.pagination.pageSize', { size })}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-center justify-between gap-3 sm:justify-end">
                <span className="text-sm tabular-nums text-muted-foreground">
                  {t('usage.invocations.pagination.pageSummary', {
                    page: activePagination.pageIndex + 1,
                    total: totalPages,
                  })}
                </span>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={goToPreviousPage}
                  disabled={activePagination.pageIndex === 0 || query.isFetching}
                >
                  {t('usage.invocations.pagination.previous')}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={goToNextPage}
                  disabled={!nextCursor || query.isFetching}
                >
                  {t('usage.invocations.pagination.next')}
                </Button>
              </div>
            </div>
          ) : null}
        </CardContent>
      ) : null}
      <InvocationDetailSheet
        item={selectedInvocation}
        canViewContent={canViewContent}
        billingDisplay={billingDisplay}
        onOpenChange={open => {
          if (!open) setSelectedInvocation(null);
        }}
      />
    </Card>
  );
}

function InvocationDetailSheet({
  item,
  onOpenChange,
  canViewContent,
  billingDisplay,
}: {
  item: InvocationLogItem | null;
  onOpenChange: (open: boolean) => void;
  canViewContent: boolean;
  billingDisplay: BillingDisplaySettings;
}) {
  const t = useT('dashboard');
  const locale = useLocale();
  if (!item) return null;

  const sourceDescription =
    item.invocation_source === 'api'
      ? t('usage.invocations.details.sourceApi')
      : item.invocation_source === 'product'
        ? t('usage.invocations.details.sourceProduct')
        : t('usage.invocations.details.sourceUnknown');

  return (
    <Sheet open onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-2xl">
        <SheetHeader className="pr-8">
          <SheetTitle>{t('usage.invocations.details.title')}</SheetTitle>
          <SheetDescription>{t('usage.invocations.details.description')}</SheetDescription>
        </SheetHeader>

        <div className="mt-6 space-y-6">
          {item.invocation_source === 'unknown' ? (
            <div className="rounded-lg border border-warning/30 bg-warning/5 p-3 text-sm text-muted-foreground">
              {t('usage.invocations.historicalExplanation')}
            </div>
          ) : null}

          <DetailSection title={t('usage.invocations.details.overview')}>
            <DetailValues
              values={[
                [t('usage.invocations.details.invocationId'), item.invocation_id],
                [t('usage.invocations.details.source'), sourceDescription],
                [
                  t('usage.invocations.details.business'),
                  t(`usage.appTypes.${knownAppType(item.app_type)}`),
                ],
                [t('usage.invocations.details.appId'), item.app_id],
                [t('usage.invocations.details.model'), item.model_name],
                [t('usage.invocations.details.provider'), item.provider_name],
                [t('usage.invocations.details.channel'), item.channel_name || '-'],
                [
                  t('usage.invocations.details.startedAt'),
                  new Date(item.started_at).toLocaleString(locale),
                ],
                [
                  t('usage.invocations.details.settledAt'),
                  new Date(item.settled_at).toLocaleString(locale),
                ],
                [t('usage.invocations.details.duration'), formatDuration(item.duration_ms)],
                [t('usage.invocations.details.attempts'), formatNumber(item.attempt_count, 0)],
              ]}
            />
          </DetailSection>

          <DetailSection title={t('usage.invocations.details.usage')}>
            <DetailValues
              values={[
                [
                  t('usage.invocations.details.promptTokens'),
                  formatTokenCount(item.prompt_tokens, locale),
                ],
                [
                  t('usage.invocations.details.cacheReadTokens'),
                  formatTokenCount(item.cache_read_tokens ?? 0, locale),
                ],
                [
                  t('usage.invocations.details.cacheWriteTokens'),
                  formatTokenCount(item.cache_write_tokens ?? 0, locale),
                ],
                [
                  t('usage.invocations.details.completionTokens'),
                  formatTokenCount(item.completion_tokens, locale),
                ],
                [
                  t('usage.invocations.details.totalTokens'),
                  formatTokenCount(item.total_tokens, locale),
                ],
                [
                  t('usage.invocations.details.pointsAndPrice'),
                  t('usage.invocations.details.pointsAndPriceValue', {
                    points: formatAiCreditValue(item.total_points, { locale }),
                    price: formatInvocationCost(item, billingDisplay, locale),
                  }),
                ],
              ]}
            />
            <BillingBreakdown item={item} billingDisplay={billingDisplay} />
          </DetailSection>

          {item.error_code ? (
            <DetailSection title={t('usage.invocations.details.error')}>
              <div className="rounded-lg bg-muted/50 p-3">
                <div className="text-sm font-medium">
                  {t(`usage.invocations.errorCodes.${knownInvocationError(item.error_code)}`)}
                </div>
                <code className="mt-1 block break-all text-xs text-muted-foreground">
                  {item.error_code}
                </code>
              </div>
            </DetailSection>
          ) : null}

          <DetailSection title={t('usage.invocations.details.content')}>
            <InvocationContentPanel
              key={item.invocation_id}
              invocationId={item.invocation_id}
              canViewContent={canViewContent}
              contentAvailable={item.content_available}
            />
          </DetailSection>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function BillingBreakdown({
  item,
  billingDisplay,
}: {
  item: InvocationLogItem;
  billingDisplay: BillingDisplaySettings;
}) {
  const t = useT('dashboard');
  const locale = useLocale();
  const details = item.pricing_details;
  const formulaTotalCost = formatInvocationCost(item, billingDisplay, locale);
  const isPlatform = details?.billing_lane === 'platform';
  const components = pricingComponents(item, details);
  const completeComponents = components.filter(isCompletePricingComponent);
  const formula = isPlatform
    ? billingDisplay.currency === 'CNY' && details?.cny_per_usd
      ? t('usage.invocations.details.platformFormulaCNY', {
          points: formatAiCreditValue(item.total_points, { locale }),
          rate: details.cny_per_usd,
          cost: formulaTotalCost,
        })
      : t('usage.invocations.details.platformFormula', {
          points: formatAiCreditValue(item.total_points, { locale }),
          cost: formulaTotalCost,
        })
    : completeComponents.length > 0
      ? `${completeComponents
          .map(component =>
            t('usage.invocations.details.componentFormula', {
              tokens: formatTokenCount(component.tokens, locale),
              unitPrice: formatHistoricalUSD(
                component.unitPrice,
                billingDisplay,
                details?.cny_per_usd,
                locale
              ),
            })
          )
          .join(
            ' + '
          )} ${t('usage.invocations.details.componentTotalFormula', { cost: formulaTotalCost })}`
      : undefined;

  return (
    <div className="mt-3 space-y-3 rounded-lg border p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-sm font-medium">
          {t('usage.invocations.details.settlementDetails')}
        </div>
        <Badge variant="outline">
          {!details
            ? t('usage.invocations.details.settlementUnknown')
            : isPlatform
              ? t('usage.invocations.details.platformSettlement')
              : t('usage.invocations.details.privateSettlement')}
        </Badge>
      </div>

      {isPlatform ? (
        <p className="text-xs text-muted-foreground">
          {t('usage.invocations.details.platformBreakdownUnavailable')}
        </p>
      ) : !details ? (
        <p className="text-xs text-muted-foreground">
          {t('usage.invocations.details.historicalBreakdownUnavailable')}
        </p>
      ) : null}

      {item.total_cost_usd || item.total_cost_cny || details?.cny_per_usd ? (
        <DetailValues
          values={[
            [
              t('usage.invocations.details.settledUSD'),
              item.total_cost_usd
                ? formatRecordedBillingAmount(item.total_cost_usd, 'USD', { locale })
                : undefined,
            ],
            [
              t('usage.invocations.details.settledCNY'),
              item.total_cost_cny
                ? formatRecordedBillingAmount(item.total_cost_cny, 'CNY', { locale })
                : undefined,
            ],
            [
              t('usage.invocations.details.callTimeExchangeRate'),
              details?.cny_per_usd
                ? t('usage.invocations.details.callTimeExchangeRateValue', {
                    rate: details.cny_per_usd,
                  })
                : undefined,
            ],
            [
              t('usage.invocations.details.callTimeCurrency'),
              details?.billing_display_currency || undefined,
            ],
          ]}
        />
      ) : null}

      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('usage.invocations.details.item')}</TableHead>
              <TableHead className="text-right">{t('usage.invocations.details.tokens')}</TableHead>
              <TableHead className="text-right">
                {t('usage.invocations.details.unitPrice')}
              </TableHead>
              <TableHead className="text-right">
                {t('usage.invocations.details.subtotal')}
              </TableHead>
              <TableHead>{t('usage.invocations.details.pricingSource')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {components.map(component => (
              <TableRow key={component.key}>
                <TableCell>{t(component.labelKey)}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatTokenCount(component.tokens, locale)}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {component.unitPrice === undefined
                    ? t('usage.invocations.details.detailUnavailable')
                    : t('usage.invocations.details.perMillionTokens', {
                        price: formatHistoricalUSD(
                          component.unitPrice,
                          billingDisplay,
                          details?.cny_per_usd,
                          locale
                        ),
                      })}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {component.cost === undefined
                    ? t('usage.invocations.details.detailUnavailable')
                    : formatHistoricalUSD(
                        component.cost,
                        billingDisplay,
                        details?.cny_per_usd,
                        locale
                      )}
                </TableCell>
                <TableCell>{t(pricingSourceKey(component.source, isPlatform))}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {formula ? (
        <div className="rounded-md bg-muted/50 p-3 text-xs">
          <div className="text-muted-foreground">{t('usage.invocations.details.formula')}</div>
          <div className="mt-1 break-words font-mono text-foreground">{formula}</div>
        </div>
      ) : null}
    </div>
  );
}

interface PricingComponent {
  key: 'input' | 'cacheRead' | 'cacheWrite' | 'output';
  labelKey:
    | 'usage.invocations.details.promptTokens'
    | 'usage.invocations.details.cacheReadTokens'
    | 'usage.invocations.details.cacheWriteTokens'
    | 'usage.invocations.details.completionTokens';
  tokens: number;
  unitPrice?: string;
  cost?: string;
  source?: string;
}

interface CompletePricingComponent extends PricingComponent {
  unitPrice: string;
  cost: string;
}

function isCompletePricingComponent(
  component: PricingComponent
): component is CompletePricingComponent {
  return component.tokens > 0 && component.unitPrice !== undefined && component.cost !== undefined;
}

function pricingComponents(
  item: InvocationLogItem,
  details: InvocationPricingDetails | undefined
): PricingComponent[] {
  const labelKeys = {
    input: 'usage.invocations.details.promptTokens',
    cacheRead: 'usage.invocations.details.cacheReadTokens',
    cacheWrite: 'usage.invocations.details.cacheWriteTokens',
    output: 'usage.invocations.details.completionTokens',
  } as const;
  const components: PricingComponent[] = [
    {
      key: 'input',
      labelKey: labelKeys.input,
      tokens: item.prompt_tokens,
      unitPrice: details?.input_price_usd_per_1m_tokens,
      cost: details?.input_cost_usd,
      source: details?.input_price_source ?? details?.pricing_source,
    },
    {
      key: 'cacheRead',
      labelKey: labelKeys.cacheRead,
      tokens: item.cache_read_tokens,
      unitPrice: details?.cache_read_price_usd_per_1m_tokens,
      cost: details?.cache_read_cost_usd,
      source: details?.cache_read_price_source ?? details?.pricing_source,
    },
    {
      key: 'cacheWrite',
      labelKey: labelKeys.cacheWrite,
      tokens: item.cache_write_tokens,
      unitPrice: details?.cache_write_price_usd_per_1m_tokens,
      cost: details?.cache_write_cost_usd,
      source: details?.cache_write_price_source ?? details?.pricing_source,
    },
    {
      key: 'output',
      labelKey: labelKeys.output,
      tokens: item.completion_tokens,
      unitPrice: details?.output_price_usd_per_1m_tokens,
      cost: details?.output_cost_usd,
      source: details?.output_price_source ?? details?.pricing_source,
    },
  ];
  return components.filter(
    component =>
      component.tokens > 0 || component.unitPrice !== undefined || component.cost !== undefined
  );
}

function formatHistoricalUSD(
  value: string,
  billingDisplay: BillingDisplaySettings,
  recordedRate: string | undefined,
  locale?: Intl.LocalesArgument
): string {
  return formatRecordedBillingAmountFromUSD(
    value,
    billingDisplay.currency,
    recordedRate,
    { locale }
  );
}

function formatInvocationCost(
  item: InvocationLogItem,
  billingDisplay: BillingDisplaySettings,
  locale?: Intl.LocalesArgument
): string {
  if (billingDisplay.currency === 'CNY' && item.total_cost_cny) {
    return formatRecordedBillingAmount(item.total_cost_cny, 'CNY', { locale });
  }
  if (item.total_cost_usd) {
    return formatRecordedBillingAmountFromUSD(
      item.total_cost_usd,
      billingDisplay.currency,
      item.pricing_details?.cny_per_usd,
      { locale }
    );
  }
  return formatBillingDisplayAmountFromNormalizedCredits(
    item.total_points,
    DEFAULT_BILLING_DISPLAY,
    { locale }
  );
}

function pricingSourceKey(
  source: string | undefined,
  isPlatform: boolean
):
  | 'usage.invocations.details.platformSettlement'
  | 'usage.invocations.details.pricingSourceUpstream'
  | 'usage.invocations.details.pricingSourceConfigured'
  | 'usage.invocations.details.pricingSourceOrganization'
  | 'usage.invocations.details.pricingSourceAdminFallback'
  | 'usage.invocations.details.pricingSourceCodeFallback'
  | 'usage.invocations.details.pricingSourceUnknown' {
  if (isPlatform) return 'usage.invocations.details.platformSettlement';
  switch (source) {
    case 'synced_model':
      return 'usage.invocations.details.pricingSourceUpstream';
    case 'upstream_model_price':
      return 'usage.invocations.details.pricingSourceConfigured';
    case 'organization_override':
      return 'usage.invocations.details.pricingSourceOrganization';
    case 'admin_fallback':
      return 'usage.invocations.details.pricingSourceAdminFallback';
    case 'code_default_fallback':
      return 'usage.invocations.details.pricingSourceCodeFallback';
    default:
      return 'usage.invocations.details.pricingSourceUnknown';
  }
}

type InvocationErrorKey =
  | 'requestInvalid'
  | 'modelNotFound'
  | 'providerAuthFailed'
  | 'providerRateLimited'
  | 'providerTimeout'
  | 'providerUnavailable'
  | 'noProviderAvailable'
  | 'invocationFailed'
  | 'billingFailed';

function knownInvocationError(code: string): InvocationErrorKey {
  const known: Record<string, InvocationErrorKey> = {
    'llm.request.invalid': 'requestInvalid',
    'llm.model.not_found': 'modelNotFound',
    'llm.provider.auth_failed': 'providerAuthFailed',
    'llm.provider.rate_limited': 'providerRateLimited',
    'llm.provider.timeout': 'providerTimeout',
    'llm.provider.unavailable': 'providerUnavailable',
    'llm.provider.none_available': 'noProviderAvailable',
    'llm.invocation.failed': 'invocationFailed',
    SETTLE_FAILED: 'billingFailed',
    BILLING_PREDEDUCT_FAILED: 'billingFailed',
  };
  return known[code] ?? 'invocationFailed';
}

function InvocationContentPanel({
  invocationId,
  canViewContent,
  contentAvailable,
}: {
  invocationId: string;
  canViewContent: boolean;
  contentAvailable: boolean;
}) {
  const t = useT('dashboard');
  const locale = useLocale();
  const [requested, setRequested] = useState(false);
  const query = useInvocationContent(invocationId, canViewContent && requested);
  const content = query.data?.data;

  if (!canViewContent) {
    return (
      <div className="rounded-lg border border-dashed p-4">
        <div className="text-sm font-medium">
          {t('usage.invocations.details.contentRestricted')}
        </div>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('usage.invocations.details.contentRestrictedDescription')}
        </p>
      </div>
    );
  }

  if (!contentAvailable) {
    return (
      <div className="rounded-lg border border-dashed p-4">
        <div className="text-sm font-medium">
          {t('usage.invocations.details.contentUnavailableTitle')}
        </div>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('usage.invocations.details.contentUnavailableDescription')}
        </p>
      </div>
    );
  }

  if (!requested) {
    return (
      <div className="rounded-lg border border-dashed p-4">
        <div className="text-sm font-medium">{t('usage.invocations.details.sensitiveTitle')}</div>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('usage.invocations.details.sensitiveDescription')}
        </p>
        <Button className="mt-3" size="sm" variant="outline" onClick={() => setRequested(true)}>
          {t('usage.invocations.details.loadContent')}
        </Button>
      </div>
    );
  }

  if (query.isLoading) return <Skeleton className="h-32 w-full rounded-lg" />;
  if (query.isError || !content) {
    return (
      <div className="rounded-lg border border-dashed p-4">
        <div className="text-sm font-medium">
          {t('usage.invocations.details.contentUnavailableTitle')}
        </div>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('usage.invocations.details.contentUnavailableDescription')}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <ContentSnapshot
        label={t('usage.invocations.details.userQuestion')}
        value={content.input_text}
      />
      <ContentSnapshot
        label={t('usage.invocations.details.aiAnswer')}
        value={content.output_text}
      />
      {(content.input_truncated || content.output_truncated) && (
        <p className="text-xs text-warning">{t('usage.invocations.details.contentTruncated')}</p>
      )}
      <p className="text-xs text-muted-foreground">
        {t('usage.invocations.details.contentExpiresAt', {
          time: new Date(content.expires_at).toLocaleString(locale),
        })}
      </p>
      <details className="rounded-lg border p-3">
        <summary className="cursor-pointer text-sm font-medium">
          {t('usage.invocations.details.advancedContent')}
        </summary>
        <div className="mt-3 grid gap-3">
          <ContentSnapshot
            label={t('usage.invocations.details.rawInput')}
            value={content.input_json}
          />
          <ContentSnapshot
            label={t('usage.invocations.details.rawOutput')}
            value={content.output_json}
          />
        </div>
      </details>
    </div>
  );
}

function DetailSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-3">
      <h3 className="text-sm font-semibold">{title}</h3>
      {children}
    </section>
  );
}

function DetailValues({ values }: { values: Array<[string, string | undefined]> }) {
  return (
    <dl className="grid gap-x-4 gap-y-3 rounded-lg bg-muted/40 p-4 text-xs sm:grid-cols-2">
      {values
        .filter(([, value]) => Boolean(value))
        .map(([label, value]) => (
          <div key={`${label}-${value}`} className="min-w-0">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="mt-1 break-all text-foreground">{value}</dd>
          </div>
        ))}
    </dl>
  );
}

function ContentSnapshot({ label, value }: { label: string; value: unknown }) {
  return (
    <div>
      <div className="mb-1.5 text-xs font-medium text-muted-foreground">{label}</div>
      <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted/50 p-3 text-xs">
        {typeof value === 'string' ? value : JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}

function SummaryMetric({
  label,
  value,
  detail,
}: {
  label: string;
  value: string;
  detail?: string;
}) {
  return (
    <div className="rounded-xl border bg-card px-4 py-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 flex items-baseline gap-2">
        <span className="text-xl font-semibold tabular-nums">{value}</span>
        {detail ? <span className="text-xs text-muted-foreground">{detail}</span> : null}
      </div>
    </div>
  );
}

function StatusBadge({ status, attempts }: { status: InvocationStatus; attempts: number }) {
  const t = useT('dashboard');
  const variant =
    status === 'success' ? 'success' : status === 'partial' ? 'warning' : 'destructive';
  return (
    <div className="flex items-center gap-2">
      <Badge variant={variant}>{t(`usage.invocations.status.${status}`)}</Badge>
      {attempts > 1 ? (
        <span className="text-xs text-muted-foreground">
          {t('usage.invocations.retried', { count: attempts })}
        </span>
      ) : null}
    </div>
  );
}

function formatDuration(value: number): string {
  return value < 1000 ? `${value} ms` : `${(value / 1000).toFixed(value < 10_000 ? 1 : 0)} s`;
}

function knownAppType(value: string): ModelUsageAppType {
  return normalizeModelUsageAppType(value);
}
