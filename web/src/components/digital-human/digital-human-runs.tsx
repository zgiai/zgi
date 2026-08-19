'use client';

import { useMemo, useState, type ReactNode } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { AlertCircle, Copy, Filter, History, RefreshCw, Search } from 'lucide-react';
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
  const runItems = query.data?.data?.items;
  const selectedRunId = searchParams.get(runParam) ?? '';

  const filteredRuns = useMemo(() => {
    const normalized = search.trim().toLowerCase();
    return (runItems ?? []).filter(run => {
      if (!matchesStatus(run.status, statusFilter)) return false;
      if (!normalized) return true;
      return [run.id, run.query, run.conversation_id, run.result_summary, run.error_code]
        .filter(Boolean)
        .some(value => String(value).toLowerCase().includes(normalized));
    });
  }, [runItems, search, statusFilter]);

  const columns: StickyDataTableColumn[] = [
    { key: 'run', header: t('runs.columns.run'), className: 'min-w-[150px] pl-4' },
    { key: 'source', header: t('runs.columns.source'), className: 'min-w-[110px]' },
    { key: 'query', header: t('runs.columns.trigger'), className: 'min-w-[260px]' },
    { key: 'status', header: t('runs.columns.status'), className: 'min-w-[120px]' },
    { key: 'steps', header: t('runs.columns.steps'), className: 'min-w-[90px]' },
    { key: 'duration', header: t('runs.columns.duration'), className: 'min-w-[110px]' },
    { key: 'created', header: t('runs.columns.created'), className: 'min-w-[170px]' },
    {
      key: 'conversation',
      header: t('runs.columns.conversation'),
      className: 'min-w-[150px] pr-4',
    },
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

  const copy = async (value: string, successMessage: string) => {
    await navigator.clipboard.writeText(value);
    toast.success(successMessage);
  };

  const hasFilters = Boolean(search.trim() || statusFilter !== 'all');

  return (
    <div className="mx-auto flex h-[calc(100vh-13rem)] min-h-[620px] max-w-7xl flex-col px-4 py-5 md:px-8">
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
        <div className="flex shrink-0 flex-col gap-3 border-b px-4 py-3">
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
                }}
              >
                {t('runs.filters.clear')}
              </Button>
            ) : null}
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
            <StickyDataTable<DigitalHumanRun>
              className="h-full"
              columns={columns}
              data={filteredRuns}
              getRowKey={run => run.id}
              isLoading={query.isLoading}
              emptyState={
                <RunListState
                  icon={hasFilters ? <Search className="size-5" /> : <History className="size-5" />}
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
                const conversationID = run.conversation_id;
                return (
                  <TableRow
                    key={run.id}
                    role="button"
                    tabIndex={0}
                    aria-pressed={selected}
                    className={cn(
                      'group cursor-pointer border-b border-border/10 transition-colors interactive-subtle',
                      selected ? 'bg-primary/5 hover:bg-primary/10' : 'hover:bg-bg-canvas/40'
                    )}
                    onClick={() => setSelectedRun(run.id)}
                    onKeyDown={event => {
                      if (event.key !== 'Enter' && event.key !== ' ') return;
                      event.preventDefault();
                      setSelectedRun(run.id);
                    }}
                  >
                    <TableCell className="py-4 pl-4">
                      <div className="flex items-center gap-1">
                        <span className="font-medium" title={run.id}>
                          {shortID(run.id)}
                        </span>
                        <Button
                          type="button"
                          size="xs"
                          variant="ghost"
                          isIcon
                          className="opacity-0 transition-opacity group-hover:opacity-100 focus:opacity-100"
                          aria-label={t('runs.copyRun')}
                          onClick={event => {
                            event.stopPropagation();
                            void copy(run.id, t('runs.runCopied'));
                          }}
                        >
                          <Copy className="size-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                    <TableCell className="py-4">
                      <SourceBadge label={runSourceLabel(run, sourceLabels)} />
                    </TableCell>
                    <TableCell className="max-w-[360px] py-4">
                      <div className="line-clamp-2 text-xs leading-5" title={run.query}>
                        {run.query}
                      </div>
                    </TableCell>
                    <TableCell className="py-4">
                      <RunStatusBadge status={run.status} label={t(`status.${run.status}`)} />
                    </TableCell>
                    <TableCell className="py-4">{runStepCount(run)}</TableCell>
                    <TableCell className="py-4">{formatRunDuration(run)}</TableCell>
                    <TableCell className="py-4">
                      {new Date(run.created_at).toLocaleString()}
                    </TableCell>
                    <TableCell className="py-4 pr-4">
                      <div className="flex items-center gap-1">
                        <span className="truncate" title={run.conversation_id ?? ''}>
                          {shortID(conversationID)}
                        </span>
                        {conversationID ? (
                          <Button
                            type="button"
                            size="xs"
                            variant="ghost"
                            isIcon
                            className="opacity-0 transition-opacity group-hover:opacity-100 focus:opacity-100"
                            aria-label={t('runs.copyConversation')}
                            onClick={event => {
                              event.stopPropagation();
                              void copy(conversationID, t('runs.conversationCopied'));
                            }}
                          >
                            <Copy className="size-3.5" />
                          </Button>
                        ) : null}
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </StickyDataTable>
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

function shortID(value?: string | null) {
  if (!value) return '—';
  if (value.length <= 13) return value;
  return `${value.slice(0, 8)}…${value.slice(-4)}`;
}

function runStepCount(run: DigitalHumanRun) {
  return (
    3 +
    recordArray(run.execution_trace?.skill_invocations).length +
    recordArray(run.execution_trace?.workflow_runs).length +
    recordArray(run.execution_trace?.conversation_artifacts).length +
    recordArray(run.execution_trace?.generated_files).length
  );
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
  const origin = recordValue(run.invocation_event?.origin);
  const source = stringValue(origin?.kind) ?? stringValue(run.invocation_event?.type) ?? 'manual';
  if (source === 'manual') return labels.manual;
  if (source === 'schedule' || source === 'scheduled') return labels.scheduled;
  if (source === 'email') return labels.email;
  if (source === 'channel' || source === 'chat') return labels.channel;
  if (source === 'webhook' || source === 'external') return labels.external;
  return source;
}

function recordArray(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value.filter(isRecord) : [];
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
