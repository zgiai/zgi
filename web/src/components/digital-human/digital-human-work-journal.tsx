'use client';

import { useState, type ReactNode } from 'react';
import { ClipboardList, Loader2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { useWorkJournal } from '@/hooks/digital-human/use-work-journal';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useWorkspaceStore } from '@/store/workspace-store';
import { useT } from '@/i18n';

interface Props {
  id: string;
  canEdit: boolean;
  onAppendPrompt: (prompt: string) => void;
}

export function DigitalHumanWorkJournal(props: Props) {
  const account = useAuthStore.use.user();
  const organization = useOrganizationStore.use.currentOrganization();
  const workspace = useWorkspaceStore.use.currentWorkspace();
  return (
    <ScopedWorkJournal
      key={[account?.id, organization?.id, workspace?.id, props.id].join(':')}
      {...props}
    />
  );
}

function ScopedWorkJournal({ id, canEdit, onAppendPrompt }: Props) {
  const t = useT('digitalHumans');
  const canView = useAccountPermissions().hasPermission('digital_human.logs.view');
  const [recordId, setRecordId] = useState<string>();
  const [operationId, setOperationId] = useState<string>();
  const journal = useWorkJournal(id, canView, recordId, operationId);
  const records = canView
    ? (journal.records.data?.pages.flatMap(page => page.data.items) ?? [])
    : [];
  const operations = canView
    ? (journal.operations.data?.pages.flatMap(page => page.data.items) ?? [])
    : [];
  const refresh = () => {
    void journal.records.refetch();
    void journal.operations.refetch();
    if (recordId) {
      void journal.record.refetch();
      void journal.history.refetch();
    }
    if (operationId) {
      void journal.operation.refetch();
    }
  };
  return (
    <section className="mt-5 space-y-4 rounded-xl border bg-card p-4 sm:p-5">
      <div className="flex items-start gap-3">
        <ClipboardList className="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" />
        <div className="min-w-0 space-y-1">
          <h3 className="text-sm font-semibold">{t('workJournal.title')}</h3>
          <p className="text-xs leading-5 text-muted-foreground">{t('workJournal.description')}</p>
        </div>
      </div>
      <div className="rounded-lg bg-muted/40 p-3 text-xs leading-5 text-muted-foreground">
        <p>{t('workJournal.skillGuide')}</p>
        <p className="mt-1">{t('workJournal.scheduleGuide')}</p>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!canEdit}
          onClick={() => onAppendPrompt(t('workJournal.socialPrompt'))}
        >
          {t('workJournal.socialExample')}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!canEdit}
          onClick={() => onAppendPrompt(t('workJournal.supportPrompt'))}
        >
          {t('workJournal.supportExample')}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">{t('workJournal.appendHint')}</p>
      {canView ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={journal.records.isFetching || journal.operations.isFetching}
          onClick={refresh}
        >
          {t('workJournal.refresh')}
        </Button>
      ) : null}
      {!canView ? (
        <p className="text-sm text-muted-foreground">{t('workJournal.permission')}</p>
      ) : (
        <div className="grid min-w-0 gap-4 lg:grid-cols-2">
          <JournalList
            title={t('workJournal.records')}
            loading={journal.records.isPending}
            error={journal.records.isError}
            empty={!records.length}
          >
            {records.map(record => (
              <button
                key={record.id}
                type="button"
                onClick={() => {
                  setRecordId(record.id);
                  setOperationId(undefined);
                }}
                className="flex w-full min-w-0 items-start justify-between gap-2 rounded-md border p-3 text-left text-xs hover:bg-muted/50"
              >
                <span className="min-w-0 break-all">
                  <span className="font-medium">
                    {record.kind} · {record.key}
                  </span>
                  <span className="mt-1 block text-muted-foreground">
                    {t('workJournal.revision', { revision: record.revision })} · {record.updated_at}
                  </span>
                </span>
                <Badge
                  variant="secondary"
                  className="max-w-32 shrink-0 break-all whitespace-normal"
                >
                  {record.state}
                </Badge>
              </button>
            ))}
            {journal.records.hasNextPage ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={journal.records.isFetchingNextPage}
                onClick={() => void journal.records.fetchNextPage()}
              >
                {t('workJournal.loadMore')}
              </Button>
            ) : null}
          </JournalList>
          <JournalList
            title={t('workJournal.operations')}
            loading={journal.operations.isPending}
            error={journal.operations.isError}
            empty={!operations.length}
          >
            <p className="text-xs leading-5 text-muted-foreground">
              {t('workJournal.operationMeaning')}
            </p>
            {operations.map(operation => (
              <button
                key={operation.id}
                type="button"
                onClick={() => {
                  setOperationId(operation.id);
                  setRecordId(undefined);
                }}
                className="flex w-full min-w-0 items-start justify-between gap-2 rounded-md border p-3 text-left text-xs hover:bg-muted/50"
              >
                <span className="min-w-0 break-all">
                  <span className="font-medium">{operation.work_key}</span>
                  <span className="mt-1 block text-muted-foreground">
                    {operation.connector_id} · {operation.operation_id}
                  </span>
                </span>
                <Badge variant={operation.state === 'succeeded' ? 'outline' : 'secondary'}>
                  {t(`workJournal.states.${operation.state}`)}
                </Badge>
              </button>
            ))}
            {journal.operations.hasNextPage ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={journal.operations.isFetchingNextPage}
                onClick={() => void journal.operations.fetchNextPage()}
              >
                {t('workJournal.loadMore')}
              </Button>
            ) : null}
          </JournalList>
        </div>
      )}
      {canView && (recordId || operationId) ? (
        <div className="min-w-0 space-y-3 rounded-lg border p-3">
          <div className="flex items-center justify-between gap-2">
            <h4 className="text-sm font-medium">{t('workJournal.details')}</h4>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => {
                setRecordId(undefined);
                setOperationId(undefined);
              }}
            >
              {t('workJournal.close')}
            </Button>
          </div>
          {(recordId ? journal.record.isPending : journal.operation.isPending) ? (
            <Loader2 aria-label={t('workJournal.loading')} className="h-4 w-4 animate-spin" />
          ) : null}
          {(recordId ? journal.record.isError : journal.operation.isError) ? (
            <p role="alert" className="text-xs text-destructive">
              {t('workJournal.error')}
            </p>
          ) : null}
          <JSONDetail
            value={
              recordId ? journal.record.data?.data.record : journal.operation.data?.data.operation
            }
          />
          {recordId ? (
            <>
              <h4 className="text-sm font-medium">{t('workJournal.history')}</h4>
              {journal.history.isError ? (
                <p role="alert" className="text-xs text-destructive">
                  {t('workJournal.error')}
                </p>
              ) : null}
              {journal.history.isPending ? (
                <Loader2 aria-label={t('workJournal.loading')} className="h-4 w-4 animate-spin" />
              ) : null}
              {journal.history.data?.pages
                .flatMap(page => page.data.items)
                .map(item => (
                  <details key={item.revision} className="rounded border p-2 text-xs">
                    <summary className="cursor-pointer">
                      {t('workJournal.revision', { revision: item.revision })} · {item.state}
                    </summary>
                    <JSONDetail value={item} />
                  </details>
                ))}
              {journal.history.hasNextPage ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={journal.history.isFetchingNextPage}
                  onClick={() => void journal.history.fetchNextPage()}
                >
                  {t('workJournal.loadMore')}
                </Button>
              ) : null}
            </>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function JSONDetail({ value }: { value: unknown }) {
  return value === undefined ? null : (
    <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted/40 p-3 text-xs">
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}

function JournalList({
  title,
  loading,
  error,
  empty,
  children,
}: {
  title: string;
  loading: boolean;
  error: boolean;
  empty: boolean;
  children: ReactNode;
}) {
  const t = useT('digitalHumans');
  return (
    <div className="min-w-0 space-y-2">
      <h4 className="text-sm font-medium">{title}</h4>
      {loading ? (
        <Loader2 aria-label={t('workJournal.loading')} className="h-4 w-4 animate-spin" />
      ) : null}
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {t('workJournal.error')}
        </p>
      ) : !loading && empty ? (
        <p className="text-xs text-muted-foreground">{t('workJournal.empty')}</p>
      ) : null}
      {children}
    </div>
  );
}
