'use client';

import { AlertCircle, CheckCircle2, Loader2 } from 'lucide-react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { DB_KEYS } from '@/hooks/query-keys';
import { dbService } from '@/services/db.service';

interface Props {
  dbId: string;
  canManage: boolean;
}

export function DbSchemaIndexPanel({ dbId, canManage }: Props) {
  const t = useT('dbs');
  const queryClient = useQueryClient();
  const queryKey = [...DB_KEYS.detail(dbId), 'schema-index'];
  const status = useQuery({
    queryKey,
    queryFn: ({ signal }) => dbService.getSchemaIndex(dbId, signal),
    refetchInterval: 5000,
  });
  const history = useInfiniteQuery({
    queryKey: [...queryKey, 'runs'],
    initialPageParam: 1,
    queryFn: ({ pageParam, signal }) => dbService.getSchemaIndexRuns(dbId, pageParam, 20, signal),
    getNextPageParam: lastPage => (lastPage.data.has_more ? lastPage.data.page + 1 : undefined),
    refetchInterval: 5000,
  });
  const start = useMutation({
    mutationFn: () => dbService.startSchemaIndex(dbId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  });
  const data = status.data?.data;
  const state = data?.state;
  const progress = data && data.total > 0 ? Math.min(100, (data.ready / data.total) * 100) : 0;
  const explanation = (code: string) => {
    switch (code) {
      case 'timeout':
        return t('schemaIndex.errors.timeout');
      case 'embedding_failed':
        return t('schemaIndex.errors.embedding');
      case 'vector_store_failed':
        return t('schemaIndex.errors.storage');
      case 'schema_load_failed':
      case 'fingerprint_changed':
        return t('schemaIndex.errors.schema');
      case 'scope_changed':
        return t('schemaIndex.errors.scope');
      case 'interrupted':
        return t('schemaIndex.errors.interrupted');
      case 'model_unavailable':
        return t('schemaIndex.errors.model');
      case 'preparation_failed':
        return t('schemaIndex.errors.preparation');
      default:
        return t('schemaIndex.errors.build');
    }
  };

  return (
    <div className="h-full overflow-y-auto p-4 sm:p-6">
      <header className="mb-6 space-y-2">
        <h1 className="text-xl font-semibold">{t('schemaIndex.title')}</h1>
        <p className="text-sm text-muted-foreground">{t('schemaIndex.description')}</p>
      </header>
      <section
        aria-label={t('schemaIndex.title')}
        className="flex w-full flex-col gap-6 rounded-lg border bg-card p-4 shadow-sm sm:p-6"
      >
        {status.isPending && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t('schemaIndex.loading')}
          </div>
        )}
        {status.isError && (
          <div role="alert" className="space-y-3 text-sm text-destructive">
            <p>{t('schemaIndex.loadError')}</p>
            <Button variant="outline" onClick={() => void status.refetch()}>
              {t('schemaIndex.reload')}
            </Button>
          </div>
        )}
        {data && !status.isError && (
          <>
            <dl className="space-y-3 border-b pb-5 text-sm">
              <div className="flex justify-between gap-4">
                <dt className="text-muted-foreground">{t('schemaIndex.tables')}</dt>
                <dd>{data.total}</dd>
              </div>
              <div className="flex justify-between gap-4">
                <dt className="shrink-0 text-muted-foreground">{t('schemaIndex.model')}</dt>
                <dd className="break-all text-right">{data.model || t('schemaIndex.noModel')}</dd>
              </div>
            </dl>
            <div className="space-y-3" aria-live="polite">
              <div className="flex items-center gap-2 text-sm font-medium">
                {state === 'building' && <Loader2 className="h-4 w-4 animate-spin text-primary" />}
                {state === 'ready' && <CheckCircle2 className="h-4 w-4 text-green-600" />}
                {state === 'failed' && <AlertCircle className="h-4 w-4 text-destructive" />}
                {t(`schemaIndex.states.${data.state}`)}
              </div>
              {data.enabled && data.total > 0 && (
                <>
                  <p className="text-sm tabular-nums">
                    {t('schemaIndex.progress', { ready: data.ready, total: data.total })}
                  </p>
                  <div
                    role="progressbar"
                    aria-label={t('schemaIndex.title')}
                    aria-valuemin={0}
                    aria-valuemax={data.total}
                    aria-valuenow={data.ready}
                    className="h-2 overflow-hidden rounded-full bg-muted"
                  >
                    <div
                      className="h-full bg-primary transition-[width] motion-reduce:transition-none"
                      style={{ width: `${progress}%` }}
                    />
                  </div>
                </>
              )}
              {data.error_code && (
                <p className="text-sm text-destructive">{explanation(data.error_code)}</p>
              )}
              {state === 'building' && (
                <p className="text-sm text-muted-foreground">{t('schemaIndex.background')}</p>
              )}
              {data.updated_at && (
                <p className="text-xs text-muted-foreground">
                  {t('schemaIndex.checkedAt', { time: new Date(data.updated_at).toLocaleString() })}
                </p>
              )}
            </div>
            {data.failed.length > 0 && (
              <details className="rounded-md border p-3 text-sm">
                <summary className="cursor-pointer font-medium">
                  {t('schemaIndex.failures', { count: data.failed.length })}
                </summary>
                <ul className="mt-3 space-y-3">
                  {data.failed.map((table, index) => (
                    <li key={`${table.name}-${index}`}>
                      <p className="break-all font-medium">{table.name}</p>
                      <p className="mt-1 text-muted-foreground">{explanation(table.code)}</p>
                    </li>
                  ))}
                </ul>
              </details>
            )}
            <div className="space-y-3 border-t pt-5">
              <p className="text-xs leading-relaxed text-muted-foreground">
                {t('schemaIndex.consent')}
              </p>
              {!canManage && (
                <p className="text-xs text-muted-foreground">{t('schemaIndex.readOnly')}</p>
              )}
              {start.isError && (
                <p role="alert" className="text-sm text-destructive">
                  {t('schemaIndex.startError')}
                </p>
              )}
              {canManage && (state === 'disabled' || state === 'failed') && (
                <Button
                  className="w-full sm:w-auto"
                  disabled={start.isPending || data.total === 0 || !data.model}
                  onClick={() => start.mutate()}
                >
                  {start.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  {data.enabled ? t('schemaIndex.retry') : t('schemaIndex.start')}
                </Button>
              )}
            </div>
          </>
        )}
      </section>
      <section aria-labelledby="schema-index-history" className="mt-6 w-full space-y-4">
        <div>
          <h2 id="schema-index-history" className="text-base font-semibold">
            {t('schemaIndex.history.title')}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('schemaIndex.history.description')}
          </p>
        </div>
        <div className="overflow-hidden rounded-lg border bg-card shadow-sm">
          {history.isPending && (
            <div
              className="flex items-center gap-2 p-6 text-sm text-muted-foreground"
              role="status"
            >
              <Loader2 className="h-4 w-4 animate-spin" />
              {t('schemaIndex.history.loading')}
            </div>
          )}
          {history.isError && (
            <div className="space-y-3 p-6 text-sm text-destructive" role="alert">
              <p>{t('schemaIndex.history.loadError')}</p>
              <Button variant="outline" onClick={() => void history.refetch()}>
                {t('schemaIndex.reload')}
              </Button>
            </div>
          )}
          {!history.isPending &&
            !history.isError &&
            history.data.pages.every(page => page.data.items.length === 0) && (
              <p className="p-6 text-sm text-muted-foreground">{t('schemaIndex.history.empty')}</p>
            )}
          {history.data?.pages
            .flatMap(page => page.data.items)
            .map(run => (
              <details key={run.id} className="group border-b p-4 last:border-b-0 sm:p-5">
                <summary className="grid cursor-pointer list-none gap-3 sm:grid-cols-[minmax(10rem,1fr)_minmax(9rem,1fr)_minmax(8rem,1fr)_auto] sm:items-center">
                  <div>
                    <p className="text-sm font-medium">
                      {t(`schemaIndex.history.triggers.${run.trigger}`)}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {new Date(run.started_at).toLocaleString()}
                    </p>
                  </div>
                  <p className="break-all text-sm text-muted-foreground">{run.model}</p>
                  <p className="text-sm text-muted-foreground">
                    {t('schemaIndex.history.tableCount', { count: run.table_count })}
                  </p>
                  <span
                    className={run.status === 'failed' ? 'text-sm text-destructive' : 'text-sm'}
                  >
                    {t(`schemaIndex.history.statuses.${run.status}`)}
                  </span>
                </summary>
                <ul className="mt-4 grid gap-2 border-t pt-4 sm:grid-cols-2 lg:grid-cols-3">
                  {run.tables.map((table, index) => (
                    <li key={`${table.name}-${index}`} className="min-w-0 text-sm">
                      <p className="break-all font-medium">
                        {table.name || t('schemaIndex.history.unknownTable')}
                      </p>
                      {table.code && (
                        <p className="mt-1 text-xs text-destructive">{explanation(table.code)}</p>
                      )}
                    </li>
                  ))}
                </ul>
              </details>
            ))}
          {history.hasNextPage && (
            <div className="border-t p-4 text-center">
              <Button
                variant="outline"
                disabled={history.isFetchingNextPage}
                onClick={() => void history.fetchNextPage()}
              >
                {history.isFetchingNextPage && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {t('schemaIndex.history.loadMore')}
              </Button>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
