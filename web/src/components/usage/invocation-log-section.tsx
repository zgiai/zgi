'use client';

import { useEffect, useRef, useState } from 'react';
import { useLocale } from 'next-intl';

import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
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
} from '@/services/types/statistics';
import {
  formatBillingDisplayAmountFromNormalizedCredits,
  type BillingDisplaySettings,
} from '@/utils/billing-display';
import { formatNumber } from '@/utils/format';

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
const knownAppTypes = new Set([
  'workflow',
  'dataset',
  'agent',
  'aichat',
  'image-runtime',
  'data_library_file',
  'prompt_optimizer',
  'prompt_playground',
  'automation_task_draft',
  'unknown',
]);

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
  const [selectedInvocation, setSelectedInvocation] = useState<InvocationLogItem | null>(null);
  const query = useInvocationLog(
    {
      start_time: startTime,
      end_time: endTime,
      invocation_source: source === 'all' ? undefined : source,
      app_type: appType,
      model_name: modelName,
      limit: 20,
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
  const summary = query.data?.summary;
  const items = query.data?.items ?? [];
  const formatCost = (value: number) =>
    formatBillingDisplayAmountFromNormalizedCredits(value, billingDisplay, { locale });

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

        {query.isLoading ? (
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
              value={formatNumber(summary?.total_tokens ?? 0, 0)}
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
                <TableHead>{t('usage.invocations.table.business')}</TableHead>
                <TableHead>{t('usage.invocations.table.status')}</TableHead>
                <TableHead className="text-right">{t('usage.invocations.table.tokens')}</TableHead>
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
                    <TableCell colSpan={9} className="px-6 py-3">
                      <Skeleton className="h-8 w-full" />
                    </TableCell>
                  </TableRow>
                ))
              ) : items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={9} className="h-28 text-center text-muted-foreground">
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
                    <TableCell>{t(`usage.appTypes.${knownAppType(item.app_type)}`)}</TableCell>
                    <TableCell>
                      <StatusBadge status={item.status} attempts={item.attempt_count} />
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatNumber(item.total_tokens, 0)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatDuration(item.duration_ms)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatCost(item.total_points)}
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
        {query.hasNextPage ? (
          <div className="flex justify-center border-t p-4">
            <Button
              variant="outline"
              onClick={() => query.fetchNextPage()}
              disabled={query.isFetchingNextPage}
            >
              {query.isFetchingNextPage
                ? t('usage.invocations.loadingMore')
                : t('usage.invocations.loadMore')}
            </Button>
          </div>
        ) : null}
      </CardContent>
      <InvocationDetailSheet
        item={selectedInvocation}
        billingDisplay={billingDisplay}
        canViewContent={canViewContent}
        onOpenChange={open => {
          if (!open) setSelectedInvocation(null);
        }}
      />
    </Card>
  );
}

function InvocationDetailSheet({
  item,
  billingDisplay,
  onOpenChange,
  canViewContent,
}: {
  item: InvocationLogItem | null;
  billingDisplay: BillingDisplaySettings;
  onOpenChange: (open: boolean) => void;
  canViewContent: boolean;
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
  const formatCost = (value: number) =>
    formatBillingDisplayAmountFromNormalizedCredits(value, billingDisplay, { locale });

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
                [t('usage.invocations.details.promptTokens'), formatNumber(item.prompt_tokens, 0)],
                [
                  t('usage.invocations.details.completionTokens'),
                  formatNumber(item.completion_tokens, 0),
                ],
                [t('usage.invocations.details.totalTokens'), formatNumber(item.total_tokens, 0)],
                [t('usage.invocations.details.cost'), formatCost(item.total_points)],
              ]}
            />
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
            />
          </DetailSection>
        </div>
      </SheetContent>
    </Sheet>
  );
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
}: {
  invocationId: string;
  canViewContent: boolean;
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
  return knownAppTypes.has(value) ? (value as ModelUsageAppType) : 'unknown';
}
