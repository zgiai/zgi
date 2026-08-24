'use client';

import { useMemo, useState, type ReactNode } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { AlertCircle, Filter, History, RefreshCw, Search } from 'lucide-react';
import { toast } from 'sonner';
import { StickyDataTable, type StickyDataTableColumn } from '@/components/common/sticky-data-table';
import { DigitalHumanRunDetailDrawer } from '@/components/digital-human/digital-human-run-detail-drawer';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { TableCell, TableRow } from '@/components/ui/table';
import { RunStatusBadge } from '@/components/workflow/ui/run-status-badge';
import {
  useDigitalHumanMutations,
  useDigitalHumanRuns,
} from '@/hooks/digital-human/use-digital-humans';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { DigitalHumanRun, DigitalHumanRunStatus } from '@/services/types/digital-human';

const runParam = 'run';
const activeStatuses = new Set<DigitalHumanRunStatus>(['queued', 'running', 'canceling']);

type StatusFilter = 'all' | 'active' | 'succeeded' | 'failed' | 'canceled';
type SourceFilter = 'all' | 'manual' | 'channel' | 'scheduled' | 'email' | 'external';

export function DigitalHumanRuns({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const sourceLabels: RunSourceLabels = {
    manual: t('runs.sources.manual'),
    scheduled: t('runs.sources.scheduled'),
    email: t('runs.sources.email'),
    channel: t('runs.sources.channel'),
    external: t('runs.sources.external'),
  };
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const query = useDigitalHumanRuns(id);
  const mutations = useDigitalHumanMutations(id);
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [sourceFilter, setSourceFilter] = useState<SourceFilter>('all');
  const runItems = query.data?.data?.items;
  const selectedRunId = searchParams.get(runParam) ?? '';

  const filteredRuns = useMemo(() => {
    const normalized = search.trim().toLowerCase();
    return (runItems ?? []).filter(run => {
      if (!matchesStatus(run.status, statusFilter)) return false;
      if (sourceFilter !== 'all' && normalizedRunSource(run) !== sourceFilter) return false;
      if (!normalized) return true;
      return [run.id, run.query, run.conversation_id, run.result_summary, run.error_code]
        .filter(Boolean)
        .some(value => String(value).toLowerCase().includes(normalized));
    });
  }, [runItems, search, sourceFilter, statusFilter]);

  const columns: StickyDataTableColumn[] = [
    { key: 'source', header: t('runs.columns.source'), className: 'w-[110px]' },
    { key: 'query', header: t('runs.columns.trigger'), className: 'min-w-[240px] pl-4' },
    { key: 'result', header: t('runs.columns.result'), className: 'min-w-[260px]' },
    { key: 'status', header: t('runs.columns.status'), className: 'w-[110px]' },
    { key: 'duration', header: t('runs.columns.duration'), className: 'w-[100px]' },
    { key: 'created', header: t('runs.columns.created'), className: 'w-[170px]' },
  ];

  const setSelectedRun = (runId?: string | null) => {
    const params = new URLSearchParams(searchParams.toString());
    if (runId) params.set(runParam, runId);
    else params.delete(runParam);
    const nextQuery = params.toString();
    router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, { scroll: false });
  };

  const cancel = async (runId: string) => {
    await mutations.cancelRun.mutateAsync({ digitalHumanId: id, runId });
    toast.success(t('runs.cancelRequested'));
  };

  const hasFilters = Boolean(search.trim() || statusFilter !== 'all' || sourceFilter !== 'all');
  const sourceTabs: Array<{ value: SourceFilter; label: string }> = [
    { value: 'all', label: t('runs.filters.allSources') },
    { value: 'manual', label: sourceLabels.manual },
    { value: 'channel', label: sourceLabels.channel },
    { value: 'external', label: sourceLabels.external },
  ];

  return (
    <div className="mx-auto flex h-full min-h-0 max-w-[1600px] flex-col px-4 py-5 md:px-8">
      <div className="mb-4 flex shrink-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-xl font-semibold">
            <History className="size-5 text-primary" />
            <h2>{t('runs.title')}</h2>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">{t('runs.description')}</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void query.refetch()}
          disabled={query.isFetching}
        >
          <RefreshCw className={cn('size-4', query.isFetching && 'animate-spin')} />
          {t('common.refresh')}
        </Button>
      </div>

      <section className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border bg-background">
        <div className="flex shrink-0 flex-col border-b">
          <div className="flex gap-1 overflow-x-auto border-b px-4 pt-2" role="tablist">
            {sourceTabs.map(tab => (
              <button
                key={tab.value}
                type="button"
                role="tab"
                aria-selected={sourceFilter === tab.value}
                className={cn(
                  'relative h-10 shrink-0 px-3 text-sm text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring',
                  sourceFilter === tab.value &&
                    'font-medium text-foreground after:absolute after:inset-x-2 after:bottom-0 after:h-0.5 after:rounded-full after:bg-primary'
                )}
                onClick={() => setSourceFilter(tab.value)}
              >
                {tab.label}
              </button>
            ))}
          </div>
          <div className="flex flex-col gap-3 px-4 py-3">
            <div>
              <div className="text-sm font-semibold">{t('runs.recent')}</div>
              <div className="text-xs text-muted-foreground">{t('runs.selectHint')}</div>
            </div>
            <div className="flex flex-col gap-2 lg:flex-row lg:items-center">
              <Input
                value={search}
                onChange={event => setSearch(event.target.value)}
                placeholder={t('runs.filters.searchPlaceholder')}
                leftIcon={<Search />}
                className="h-9"
                containerClassName="min-w-0 lg:w-[420px]"
              />
              <Select
                value={statusFilter}
                onValueChange={value => setStatusFilter(value as StatusFilter)}
              >
                <SelectTrigger className="lg:w-[190px]" aria-label={t('runs.filters.statusLabel')}>
                  <div className="flex items-center gap-2">
                    <Filter className="size-4 text-muted-foreground" />
                    <SelectValue />
                  </div>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t('runs.filters.all')}</SelectItem>
                  <SelectItem value="active">{t('runs.filters.active')}</SelectItem>
                  <SelectItem value="succeeded">{t('status.succeeded')}</SelectItem>
                  <SelectItem value="failed">{t('status.failed')}</SelectItem>
                  <SelectItem value="canceled">{t('status.canceled')}</SelectItem>
                </SelectContent>
              </Select>
              {hasFilters ? (
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setSearch('');
                    setStatusFilter('all');
                    setSourceFilter('all');
                  }}
                >
                  {t('runs.filters.clear')}
                </Button>
              ) : null}
            </div>
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-hidden">
          {query.isError ? (
            <RunListState
              tone="error"
              icon={<AlertCircle className="size-5" />}
              title={t('runs.loadErrorTitle')}
              description={t('runs.loadErrorDescription')}
              action={
                <Button size="sm" variant="outline" onClick={() => void query.refetch()}>
                  {t('runs.retry')}
                </Button>
              }
            />
          ) : (
            <>
              <div className="h-full overflow-y-auto p-3 md:hidden">
                {query.isLoading ? (
                  <div className="space-y-3">
                    {[0, 1, 2].map(item => (
                      <div key={item} className="h-36 animate-pulse rounded-xl bg-muted/40" />
                    ))}
                  </div>
                ) : filteredRuns.length === 0 ? (
                  <RunListState
                    icon={
                      hasFilters ? <Search className="size-5" /> : <History className="size-5" />
                    }
                    title={hasFilters ? t('runs.noMatches') : t('runs.empty')}
                    description={
                      hasFilters ? t('runs.noMatchesDescription') : t('runs.emptyDescription')
                    }
                  />
                ) : (
                  <div className="space-y-3">
                    {filteredRuns.map(run => (
                      <button
                        key={run.id}
                        type="button"
                        className={cn(
                          'w-full rounded-xl border p-4 text-left outline-none transition-colors hover:bg-muted/30 focus-visible:ring-2 focus-visible:ring-ring',
                          run.id === selectedRunId && 'border-primary/40 bg-primary/5'
                        )}
                        onClick={() => setSelectedRun(run.id)}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <SourceBadge label={runSourceLabel(run, sourceLabels)} />
                          <RunStatusBadge status={run.status} label={t(`status.${run.status}`)} />
                        </div>
                        <p className="mt-3 line-clamp-2 text-sm font-medium leading-6">
                          {run.query}
                        </p>
                        {run.result_summary ? (
                          <p className="mt-2 line-clamp-2 text-xs leading-5 text-muted-foreground">
                            {run.result_summary}
                          </p>
                        ) : null}
                        <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                          <span>{formatRunDuration(run)}</span>
                          <span>{new Date(run.created_at).toLocaleString()}</span>
                        </div>
                      </button>
                    ))}
                  </div>
                )}
              </div>
              <StickyDataTable<DigitalHumanRun>
                className="hidden h-full md:block"
                columns={columns}
                data={filteredRuns}
                getRowKey={run => run.id}
                isLoading={query.isLoading}
                emptyState={
                  <RunListState
                    icon={
                      hasFilters ? <Search className="size-5" /> : <History className="size-5" />
                    }
                    title={hasFilters ? t('runs.noMatches') : t('runs.empty')}
                    description={
                      hasFilters ? t('runs.noMatchesDescription') : t('runs.emptyDescription')
                    }
                    action={
                      hasFilters ? (
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => {
                            setSearch('');
                            setStatusFilter('all');
                            setSourceFilter('all');
                          }}
                        >
                          {t('runs.filters.clear')}
                        </Button>
                      ) : undefined
                    }
                  />
                }
              >
                {filteredRuns.map(run => {
                  const selected = run.id === selectedRunId;
                  return (
                    <TableRow
                      key={run.id}
                      className={cn(
                        'group cursor-pointer border-b border-border/10 transition-colors',
                        selected ? 'bg-primary/5 hover:bg-primary/10' : 'hover:bg-bg-canvas/40'
                      )}
                      tabIndex={0}
                      onClick={() => setSelectedRun(run.id)}
                      onKeyDown={event => {
                        if (event.key === 'Enter' || event.key === ' ') {
                          event.preventDefault();
                          setSelectedRun(run.id);
                        }
                      }}
                    >
                      <TableCell className="py-4">
                        <SourceBadge label={runSourceLabel(run, sourceLabels)} />
                      </TableCell>
                      <TableCell className="max-w-[420px] py-4 pl-4">
                        <span
                          className="line-clamp-2 text-left text-xs leading-5"
                          title={run.query}
                        >
                          {run.query || t('runs.details')}
                        </span>
                      </TableCell>
                      <TableCell className="max-w-[360px] py-4">
                        <span
                          className="line-clamp-2 text-xs leading-5 text-muted-foreground"
                          title={run.result_summary || undefined}
                        >
                          {run.result_summary || t('runs.resultPending')}
                        </span>
                      </TableCell>
                      <TableCell className="py-4">
                        <RunStatusBadge status={run.status} label={t(`status.${run.status}`)} />
                      </TableCell>
                      <TableCell className="py-4">{formatRunDuration(run)}</TableCell>
                      <TableCell className="py-4">
                        {new Date(run.created_at).toLocaleString()}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </StickyDataTable>
            </>
          )}
        </div>
      </section>

      <DigitalHumanRunDetailDrawer
        digitalHumanId={id}
        runId={selectedRunId}
        open={Boolean(selectedRunId)}
        onOpenChange={open => {
          if (!open) setSelectedRun(null);
        }}
        onCancel={cancel}
        cancelPending={mutations.cancelRun.isPending}
      />
    </div>
  );
}

function RunListState({
  icon,
  title,
  description,
  action,
  tone = 'default',
}: {
  icon: ReactNode;
  title: string;
  description: string;
  action?: ReactNode;
  tone?: 'default' | 'error';
}) {
  return (
    <div className="m-4 flex min-h-56 flex-col items-center justify-center rounded-2xl border border-dashed bg-muted/10 px-6 py-10 text-center">
      <div
        className={cn(
          'mb-4 flex size-11 items-center justify-center rounded-xl border bg-background shadow-sm',
          tone === 'error' ? 'border-destructive/30 text-destructive' : 'text-muted-foreground'
        )}
      >
        {icon}
      </div>
      <div className="text-sm font-semibold">{title}</div>
      <div className="mt-1 max-w-md text-sm leading-6 text-muted-foreground">{description}</div>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

function SourceBadge({ label }: { label: string }) {
  return (
    <span className="inline-flex h-6 items-center rounded-full border bg-muted/30 px-2.5 text-xs font-medium text-muted-foreground">
      {label}
    </span>
  );
}

function matchesStatus(status: DigitalHumanRunStatus, filter: StatusFilter) {
  if (filter === 'all') return true;
  if (filter === 'active') return activeStatuses.has(status);
  if (filter === 'failed') return status === 'failed' || status === 'unknown';
  return status === filter;
}

function formatRunDuration(run: DigitalHumanRun) {
  const start = Date.parse(run.started_at ?? run.created_at);
  const end = Date.parse(run.completed_at ?? run.updated_at);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return '—';
  const milliseconds = end - start;
  if (milliseconds < 1_000) return `${milliseconds} ms`;
  if (milliseconds < 60_000) return `${(milliseconds / 1_000).toFixed(1)} s`;
  return `${Math.floor(milliseconds / 60_000)}m ${Math.floor((milliseconds % 60_000) / 1_000)}s`;
}

interface RunSourceLabels {
  manual: string;
  scheduled: string;
  email: string;
  channel: string;
  external: string;
}

function runSourceLabel(run: DigitalHumanRun, labels: RunSourceLabels) {
  const source = normalizedRunSource(run);
  return labels[source];
}

function normalizedRunSource(run: DigitalHumanRun): Exclude<SourceFilter, 'all'> {
  const origin = recordValue(run.invocation_event?.origin);
  const source = stringValue(origin?.kind) ?? stringValue(run.invocation_event?.type) ?? 'manual';
  if (source === 'manual') return 'manual';
  if (source === 'schedule' || source === 'scheduled') return 'scheduled';
  if (source === 'email') return 'email';
  if (source === 'channel' || source === 'chat') return 'channel';
  return 'external';
}

function recordValue(value: unknown): Record<string, unknown> | null {
  return isRecord(value) ? value : null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function stringValue(value: unknown) {
  return typeof value === 'string' && value.trim() ? value.trim() : null;
}
