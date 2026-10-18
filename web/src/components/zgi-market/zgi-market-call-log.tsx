'use client';

import { Fragment, useMemo, useState, type ReactNode } from 'react';
import { endOfDay, getUnixTime, startOfDay, subDays } from 'date-fns';
import { ChevronDown, ChevronRight, RefreshCw } from 'lucide-react';
import MarketEmptyState from '@/components/market/market-empty-state';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { marketAmount } from '@/components/zgi-market/market-format';
import { useMarketLabels } from '@/components/zgi-market/use-market-labels';
import {
  useZgiMarketCallContent,
  useZgiMarketCallLogs,
  useZgiMarketMeta,
} from '@/hooks/use-zgi-market';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type {
  MarketCallBody,
  MarketCallChargeLine,
  MarketCallLog,
  MarketCallStatus,
} from '@/services/types/zgi-market';
import { formatDate, formatNumber } from '@/utils/format';

type DateRangeKey = 'today' | 'last7Days' | 'last30Days';

const DATE_RANGE_DAYS: Record<DateRangeKey, number> = {
  today: 0,
  // The range includes today, so subtract one fewer day than the label count.
  last7Days: 6,
  last30Days: 29,
};
const PER_PAGE = 20;
const COLUMNS = 7;
const STATUSES: MarketCallStatus[] = ['succeeded', 'tool_error', 'refused', 'failed'];
const SOURCES = ['console', 'webapp', 'external-api', 'channel', 'task', 'credential'] as const;
type KnownSource = (typeof SOURCES)[number];

function points(micros: number): string {
  return marketAmount({ unit_price_micros: micros, currency: 'PTS' }).amount;
}

/** Text as indented JSON when it is JSON, else as it was stored. */
function prettyJSON(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

/** Milliseconds below a second, seconds with one decimal above. */
function duration(ms: number): string {
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`;
}

/** The organization's market tool calls: what was sent, what came back, and what it cost. */
export function ZgiMarketCallLog() {
  const t = useT();
  const { locale } = useLocale();
  const { meter } = useMarketLabels();
  const meta = useZgiMarketMeta();
  const admin = meta.data?.admin === true;
  const [dateRange, setDateRange] = useState<DateRangeKey>('today');
  const [status, setStatus] = useState<MarketCallStatus | 'all'>('all');
  const [source, setSource] = useState<KnownSource | 'all'>('all');
  const [page, setPage] = useState(1);
  const [refreshKey, setRefreshKey] = useState(() => Date.now());
  const [open, setOpen] = useState<string | null>(null);

  const query = useMemo(() => {
    const now = new Date(refreshKey);
    return {
      from: getUnixTime(startOfDay(subDays(now, DATE_RANGE_DAYS[dateRange]))),
      // Exclusive, so the last second of today is still inside.
      to: getUnixTime(endOfDay(now)) + 1,
      page,
      per_page: PER_PAGE,
      ...(status === 'all' ? {} : { status }),
      ...(source === 'all' ? {} : { source }),
    };
  }, [dateRange, status, source, page, refreshKey]);
  const logs = useZgiMarketCallLogs(query, admin);
  const data = logs.data;
  const summary = data?.summary;
  const total = data?.total ?? 0;
  const start = total === 0 ? 0 : (page - 1) * PER_PAGE + 1;
  const end = Math.min(page * PER_PAGE, total);

  const subject = (row: MarketCallLog) =>
    `${t(`market.callLogs.subjects.${row.subject_type}`)} · ${
      row.subject_type === 'credential'
        ? row.credential_name ||
          t('market.callLogs.detail.credentialUnnamed', { id: row.credential_id })
        : row.subject_name || t('market.callLogs.deletedSubject')
    }`;
  const sourceLabel = (row: MarketCallLog) =>
    (SOURCES as readonly string[]).includes(row.source)
      ? t(`market.callLogs.sources.${row.source as KnownSource}`)
      : t('market.callLogs.sources.unknown');
  const person = (row: MarketCallLog) =>
    row.user_name ||
    (row.external_user_id
      ? t('market.callLogs.externalUser', { id: row.external_user_id.slice(0, 12) })
      : '');
  const cost = (row: MarketCallLog) => {
    switch (row.charge_state) {
      case 'charged':
        return t('market.callLogs.charge.points', { amount: points(row.charge_micros) });
      case 'pending':
        return t('market.callLogs.charge.pending');
      case 'none':
        return t('market.callLogs.charge.none');
      default:
        return t('market.callLogs.charge.unknown');
    }
  };
  const line = (charge: MarketCallChargeLine) =>
    t('market.callLogs.detail.line', {
      quantity: formatNumber(charge.quantity, 0),
      // The unit meter is called what the offering calls it.
      unit: charge.meter === 'unit' && charge.unit_name ? charge.unit_name : meter(charge.meter),
      amount: points(charge.amount_micros),
    });

  return (
    <div className="flex h-full flex-col overflow-auto">
      <div className="space-y-6 p-6">
        <div>
          <h1 className="text-2xl font-semibold">{t('market.callLogs.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('market.callLogs.subtitle')}</p>
        </div>

        {meta.isLoading ? (
          <Skeleton className="h-40 rounded-xl" />
        ) : !admin ? (
          <MarketEmptyState title={t('market.callLogs.adminOnly')} />
        ) : (
          <>
            <Card className="border-border/80 shadow-sm">
              <CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center">
                <Select
                  value={dateRange}
                  onValueChange={value => {
                    setDateRange(value as DateRangeKey);
                    setPage(1);
                  }}
                >
                  <SelectTrigger className="w-full sm:w-[150px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(Object.keys(DATE_RANGE_DAYS) as DateRangeKey[]).map(key => (
                      <SelectItem key={key} value={key}>
                        {t(`market.callLogs.dateRange.${key}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select
                  value={status}
                  onValueChange={value => {
                    setStatus(value as MarketCallStatus | 'all');
                    setPage(1);
                  }}
                >
                  <SelectTrigger className="w-full sm:w-[150px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t('market.callLogs.allStatuses')}</SelectItem>
                    {STATUSES.map(value => (
                      <SelectItem key={value} value={value}>
                        {t(`market.callLogs.status.${value}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select
                  value={source}
                  onValueChange={value => {
                    setSource(value as KnownSource | 'all');
                    setPage(1);
                  }}
                >
                  <SelectTrigger className="w-full sm:w-[150px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t('market.callLogs.allSources')}</SelectItem>
                    {SOURCES.map(value => (
                      <SelectItem key={value} value={value}>
                        {t(`market.callLogs.sources.${value}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  variant="outline"
                  className="gap-2 sm:ml-auto"
                  onClick={() => setRefreshKey(Date.now())}
                >
                  <RefreshCw className="h-4 w-4" />
                  {t('market.callLogs.refresh')}
                </Button>
              </CardContent>
            </Card>

            <div className="grid gap-3 sm:grid-cols-3">
              <Metric
                label={t('market.callLogs.metrics.calls')}
                value={summary ? formatNumber(summary.calls, 0) : undefined}
              />
              <Metric
                label={t('market.callLogs.metrics.succeeded')}
                value={summary ? formatNumber(summary.succeeded, 0) : undefined}
              />
              <Metric
                label={t('market.callLogs.metrics.cost')}
                value={summary ? points(summary.charge_micros) : undefined}
                detail={
                  summary && summary.pending > 0
                    ? t('market.callLogs.metrics.pending', { count: summary.pending })
                    : undefined
                }
              />
            </div>

            {logs.isError ? (
              <div
                role="alert"
                className="flex items-center justify-between gap-3 rounded-lg border border-destructive/40 bg-destructive/5 p-4"
              >
                <p className="text-sm text-destructive">{t('market.callLogs.loadFailed')}</p>
                <Button variant="outline" size="sm" onClick={() => void logs.refetch()}>
                  {t('market.callLogs.retry')}
                </Button>
              </div>
            ) : (
              <Card className="overflow-hidden border-border/80 shadow-sm">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-8" />
                      <TableHead>{t('market.callLogs.columns.time')}</TableHead>
                      <TableHead>{t('market.callLogs.columns.caller')}</TableHead>
                      <TableHead>{t('market.callLogs.columns.tool')}</TableHead>
                      <TableHead>{t('market.callLogs.columns.result')}</TableHead>
                      <TableHead className="text-right">
                        {t('market.callLogs.columns.duration')}
                      </TableHead>
                      <TableHead className="text-right">
                        {t('market.callLogs.columns.cost')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {logs.isLoading ? (
                      Array.from({ length: 4 }).map((_, index) => (
                        <TableRow key={index}>
                          <TableCell colSpan={COLUMNS}>
                            <Skeleton className="h-6" />
                          </TableCell>
                        </TableRow>
                      ))
                    ) : !data || data.data.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={COLUMNS}
                          className="py-10 text-center text-sm text-muted-foreground"
                        >
                          {t('market.callLogs.empty')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      data.data.map(row => {
                        const expanded = open === row.id;
                        return (
                          <Fragment key={row.id}>
                            <TableRow
                              className="cursor-pointer"
                              aria-expanded={expanded}
                              onClick={() => setOpen(expanded ? null : row.id)}
                            >
                              <TableCell className="text-muted-foreground">
                                {expanded ? (
                                  <ChevronDown className="h-4 w-4" />
                                ) : (
                                  <ChevronRight className="h-4 w-4" />
                                )}
                              </TableCell>
                              <TableCell className="whitespace-nowrap text-sm">
                                {formatDate(row.created_at, 'YYYY-MM-DD HH:mm:ss', { locale })}
                              </TableCell>
                              <TableCell className="max-w-[220px] text-sm">
                                <div className="truncate">{subject(row)}</div>
                                <div className="truncate text-xs text-muted-foreground">
                                  {[sourceLabel(row), person(row)].filter(Boolean).join(' · ')}
                                </div>
                              </TableCell>
                              <TableCell className="text-sm">
                                <div className="font-medium">{row.tool_name}</div>
                                <div className="text-xs text-muted-foreground">
                                  {row.listing_title || row.listing_slug}
                                  {row.offering_slug &&
                                  row.offering_slug !== (row.listing_title || row.listing_slug)
                                    ? ` · ${row.offering_slug}`
                                    : ''}
                                </div>
                              </TableCell>
                              <TableCell>
                                <Badge
                                  variant={row.status === 'succeeded' ? 'secondary' : 'destructive'}
                                >
                                  {t(`market.callLogs.status.${row.status}`)}
                                </Badge>
                              </TableCell>
                              <TableCell className="whitespace-nowrap text-right text-sm tabular-nums">
                                {duration(row.duration_ms)}
                              </TableCell>
                              <TableCell className="whitespace-nowrap text-right text-sm tabular-nums">
                                {cost(row)}
                              </TableCell>
                            </TableRow>
                            {expanded ? (
                              <TableRow className="bg-muted/20 hover:bg-muted/20">
                                <TableCell colSpan={COLUMNS} className="space-y-3 p-4">
                                  {row.source === 'credential' ? (
                                    <MarketKeptContent requestId={row.request_id} />
                                  ) : (
                                    <>
                                      <Detail title={t('market.callLogs.detail.input')}>
                                        <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-all rounded-md bg-background p-3 font-mono text-xs">
                                          {prettyJSON(row.input)}
                                        </pre>
                                      </Detail>
                                      <Detail title={t('market.callLogs.detail.output')}>
                                        <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-all rounded-md bg-background p-3 font-mono text-xs">
                                          {row.output
                                            ? prettyJSON(row.output)
                                            : t('market.callLogs.detail.noOutput')}
                                        </pre>
                                      </Detail>
                                    </>
                                  )}
                                  {row.error_message ? (
                                    <Detail title={t('market.callLogs.detail.error')}>
                                      <p className="text-sm text-destructive">
                                        {row.error_message}
                                      </p>
                                    </Detail>
                                  ) : null}
                                  {row.charge_lines.length > 0 ? (
                                    <Detail title={t('market.callLogs.detail.charges')}>
                                      <ul className="space-y-1 text-sm">
                                        {row.charge_lines.map((charge, index) => (
                                          <li key={index}>{line(charge)}</li>
                                        ))}
                                      </ul>
                                    </Detail>
                                  ) : null}
                                  {row.conversation_id ? (
                                    <p className="text-xs text-muted-foreground">
                                      {t('market.callLogs.detail.conversation')}:{' '}
                                      <span className="font-mono">{row.conversation_id}</span>
                                      {row.message_id ? (
                                        <>
                                          {' · '}
                                          {t('market.callLogs.detail.message')}:{' '}
                                          <span className="font-mono">{row.message_id}</span>
                                        </>
                                      ) : null}
                                    </p>
                                  ) : null}
                                  {row.request_id ? (
                                    <p className="text-xs text-muted-foreground">
                                      {t('market.callLogs.detail.requestId')}:{' '}
                                      <span className="font-mono">{row.request_id}</span>
                                    </p>
                                  ) : null}
                                </TableCell>
                              </TableRow>
                            ) : null}
                          </Fragment>
                        );
                      })
                    )}
                  </TableBody>
                </Table>
                {total > 0 ? (
                  <div className="flex items-center justify-between gap-3 border-t px-4 py-3 text-sm text-muted-foreground">
                    <span>{t('market.callLogs.pager', { start, end, total })}</span>
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={page <= 1}
                        onClick={() => setPage(current => Math.max(1, current - 1))}
                      >
                        {t('market.callLogs.previous')}
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={end >= total}
                        onClick={() => setPage(current => current + 1)}
                      >
                        {t('market.callLogs.next')}
                      </Button>
                    </div>
                  </div>
                ) : null}
              </Card>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function Metric({ label, value, detail }: { label: string; value?: string; detail?: string }) {
  return (
    <Card className="border-border/80 shadow-sm">
      <CardContent className="p-4">
        <p className="text-sm text-muted-foreground">{label}</p>
        {value === undefined ? (
          <Skeleton className="mt-2 h-7 w-20" />
        ) : (
          <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
        )}
        {detail ? <p className="mt-1 text-xs text-muted-foreground">{detail}</p> : null}
      </CardContent>
    </Card>
  );
}

function Detail({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <p className="text-xs font-medium text-muted-foreground">{title}</p>
      {children}
    </div>
  );
}

function marketBodyText(body: MarketCallBody): string {
  return body.encoding === 'text' ? prettyJSON(body.body) : body.body;
}

/** What a call made outside this workspace sent and answered, read from the market. */
function MarketKeptContent({ requestId }: { requestId: string }) {
  const t = useT();
  const content = useZgiMarketCallContent(requestId, true);
  if (content.isLoading) {
    return <Skeleton className="h-24" />;
  }
  if (content.isError || !content.data || !content.data.kept) {
    return <p className="text-sm text-muted-foreground">{t('market.callLogs.detail.notKept')}</p>;
  }
  const blocks: Array<[string, MarketCallBody | null]> = [
    [t('market.callLogs.detail.input'), content.data.request],
    [t('market.callLogs.detail.output'), content.data.response],
  ];
  return (
    <>
      {blocks.map(([title, body]) => (
        <Detail key={title} title={title}>
          {body && body.bytes > 0 ? (
            <>
              <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-all rounded-md bg-background p-3 font-mono text-xs">
                {marketBodyText(body)}
              </pre>
              {body.truncated ? (
                <p className="text-xs text-muted-foreground">
                  {t('market.callLogs.detail.truncated')}
                </p>
              ) : null}
            </>
          ) : (
            <p className="text-sm text-muted-foreground">{t('market.callLogs.detail.noOutput')}</p>
          )}
        </Detail>
      ))}
      <p className="text-xs text-muted-foreground">{t('market.callLogs.detail.fromMarket')}</p>
    </>
  );
}
