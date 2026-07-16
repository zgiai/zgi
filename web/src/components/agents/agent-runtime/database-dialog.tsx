'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { AlertCircle, Check, Database, ExternalLink, RefreshCw, SearchX } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { DATABASE_PERMISSION_ACTIONS } from '@/constants/permissions';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import agentService from '@/services/agent.service';
import type { AgentDatabaseBinding, AgentDatabaseBindingCandidate } from '@/services/types/agent';
import {
  AgentRuntimeSelectionDialog,
  AgentRuntimeSelectionCardIcon,
  AgentRuntimeSelectionEmptyState,
  AgentRuntimeSelectionGrid,
  AgentRuntimeSelectionPagination,
  AgentRuntimeSelectionSkeleton,
} from './selection-dialog';
import { useSelectionDialogDraftGuard } from './use-selection-dialog-draft-guard';

interface AgentRuntimeDatabaseDialogProps {
  agentId: string;
  open: boolean;
  bindings: AgentDatabaseBinding[];
  onOpenChange: (open: boolean) => void;
  onConfirmDatabases: (dbIds: string[], databases: AgentDatabaseBindingCandidate[]) => void;
}

export function AgentRuntimeDatabaseDialog({
  agentId,
  open,
  bindings,
  onOpenChange,
  onConfirmDatabases,
}: AgentRuntimeDatabaseDialogProps) {
  const t = useT('agents.agentRuntime');
  const [selectedDbIds, setSelectedDbIds] = useState<string[]>([]);
  const [localSelectedDatabases, setLocalSelectedDatabases] = useState<
    AgentDatabaseBindingCandidate[]
  >([]);
  const [dbSearch, setDbSearch] = useState('');
  const [showAvailableOnly, setShowAvailableOnly] = useState(true);
  const debouncedSearch = useDebouncedValue(dbSearch.trim(), 300);
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canCreateDatabase = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.create);
  const canManageSchema = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.schemaManage);
  const hasSearch = debouncedSearch.length > 0;
  const databaseQuery = useInfiniteQuery({
    queryKey: [
      ...AGENT_KEYS.databaseBindingCandidates(agentId),
      'dialog',
      {
        query: debouncedSearch,
        available_only: showAvailableOnly,
      },
    ],
    initialPageParam: 1,
    queryFn: ({ pageParam }) =>
      agentService.getAgentDatabaseBindingCandidates(agentId, {
        query: debouncedSearch || undefined,
        available_only: showAvailableOnly,
        page: pageParam as number,
        limit: 24,
      }),
    getNextPageParam: lastPage => (lastPage.data.has_more ? lastPage.data.page + 1 : undefined),
    enabled: open && Boolean(agentId),
    staleTime: 3 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: 'always',
    retry: false,
  });
  const dbs = useMemo(
    () => databaseQuery.data?.pages.flatMap(page => page.data.data ?? []) ?? [],
    [databaseQuery.data?.pages]
  );
  const selectedSet = useMemo(() => new Set(selectedDbIds), [selectedDbIds]);

  useEffect(() => {
    if (!open) return;
    setDbSearch('');
    setShowAvailableOnly(true);
    setSelectedDbIds(bindings.map(binding => binding.data_source_id));
    setLocalSelectedDatabases([]);
  }, [bindings, open]);

  useEffect(() => {
    if (!open) return;
    setLocalSelectedDatabases(current => {
      const byID = new Map(current.map(db => [db.data_source_id, db]));
      dbs.forEach(db => {
        if (selectedSet.has(db.data_source_id)) byID.set(db.data_source_id, db);
      });
      return Array.from(byID.values());
    });
  }, [dbs, open, selectedSet]);

  const fetchNextDatabasePage = databaseQuery.fetchNextPage;
  const loadNextDatabasePage = useCallback(() => fetchNextDatabasePage(), [fetchNextDatabasePage]);
  const databaseSentinelRef = useInfiniteObserver({
    enabled: open && Boolean(agentId),
    hasNextPage: Boolean(databaseQuery.hasNextPage),
    isFetchingNextPage: databaseQuery.isFetchingNextPage,
    fetchNextPage: loadNextDatabasePage,
    rootMargin: '240px',
  });
  const selectedCounts = useMemo(
    () => new Map(bindings.map(binding => [binding.data_source_id, binding.table_ids.length])),
    [bindings]
  );

  const toggleDatabase = (db: AgentDatabaseBindingCandidate, checked: boolean) => {
    const dbId = db.data_source_id;
    if (checked) {
      setLocalSelectedDatabases(current => {
        const byID = new Map(current.map(item => [item.data_source_id, item]));
        byID.set(dbId, db);
        return Array.from(byID.values());
      });
    }
    setSelectedDbIds(current =>
      checked
        ? Array.from(new Set([...current, dbId]))
        : current.filter(selectedId => selectedId !== dbId)
    );
  };
  const originalDbIds = useMemo(() => bindings.map(binding => binding.data_source_id), [bindings]);
  const isDirty = useMemo(() => {
    if (selectedDbIds.length !== originalDbIds.length) return true;
    const original = new Set(originalDbIds);
    return selectedDbIds.some(dbId => !original.has(dbId));
  }, [originalDbIds, selectedDbIds]);
  const commitSelection = useCallback(
    () =>
      onConfirmDatabases(
        selectedDbIds,
        localSelectedDatabases.filter(db => selectedSet.has(db.data_source_id))
      ),
    [localSelectedDatabases, onConfirmDatabases, selectedDbIds, selectedSet]
  );
  const saveDisabled = databaseQuery.isLoading || databaseQuery.isError || !agentId;
  const { requestOpenChange, requestClose, saveAndClose, closeGuard } =
    useSelectionDialogDraftGuard({
      open,
      isDirty,
      disabled: saveDisabled,
      onOpenChange,
      onSave: commitSelection,
    });

  return (
    <>
      <AgentRuntimeSelectionDialog
        open={open}
        title={t('database.dialogTitle')}
        description={t('database.dialogDescription')}
        selectedCount={selectedDbIds.length}
        search={dbSearch}
        searchPlaceholder={t('database.searchDatabase')}
        isSearching={
          dbSearch.trim() !== debouncedSearch ||
          (databaseQuery.isFetching && !databaseQuery.isFetchingNextPage)
        }
        onOpenChange={requestOpenChange}
        onChangeSearch={setDbSearch}
        toolbar={
          <label className="flex h-9 shrink-0 cursor-pointer items-center gap-2 rounded-md border bg-background px-3 text-sm">
            <Checkbox
              checked={showAvailableOnly}
              onCheckedChange={value => setShowAvailableOnly(value === true)}
            />
            {t('database.availableOnly')}
          </label>
        }
        footer={
          <>
            <Button type="button" variant="ghost" onClick={requestClose}>
              {t('database.cancel')}
            </Button>
            <Button type="button" disabled={saveDisabled} onClick={saveAndClose}>
              {t('database.confirm')}
            </Button>
          </>
        }
      >
        {databaseQuery.isLoading ||
        (!hasSearch && dbs.length === 0 && isPermissionsLoading && !databaseQuery.isError) ? (
          <AgentRuntimeSelectionSkeleton />
        ) : databaseQuery.isError ? (
          <AgentRuntimeSelectionEmptyState
            icon={<AlertCircle />}
            title={t('database.loadFailedTitle')}
            description={t('database.loadFailedDescription')}
            action={
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void databaseQuery.refetch()}
              >
                <RefreshCw className="size-4" />
                {t('database.retryLoad')}
              </Button>
            }
          />
        ) : dbs.length === 0 ? (
          hasSearch ? (
            <AgentRuntimeSelectionEmptyState
              variant="search"
              icon={<SearchX />}
              title={t('database.searchEmptyTitle')}
              description={t('database.searchEmptyDescription', { query: debouncedSearch })}
              action={
                <Button type="button" variant="outline" size="sm" onClick={() => setDbSearch('')}>
                  {t('selectionDialog.clearSearch')}
                </Button>
              }
            />
          ) : showAvailableOnly ? (
            <AgentRuntimeSelectionEmptyState
              icon={<Database />}
              title={t('database.availableFilterEmptyTitle')}
              description={t('database.availableFilterEmptyDescription')}
              action={
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setShowAvailableOnly(false)}
                >
                  {t('database.showAllDatabases')}
                </Button>
              }
            />
          ) : (
            <AgentRuntimeSelectionEmptyState
              icon={<Database />}
              title={t('database.emptyTitle')}
              description={t(
                canCreateDatabase
                  ? 'database.emptyDescription'
                  : 'database.emptyUnavailableDescription'
              )}
              action={
                canCreateDatabase ? (
                  <Button asChild size="sm">
                    <Link href="/console/db" target="_blank" rel="noreferrer">
                      {t('database.createAction')}
                      <ExternalLink className="size-4" />
                    </Link>
                  </Button>
                ) : undefined
              }
            />
          )
        ) : (
          <>
            <AgentRuntimeSelectionGrid>
              {dbs.map(db => (
                <DatabaseOption
                  key={db.data_source_id}
                  db={db}
                  selected={selectedSet.has(db.data_source_id)}
                  selectedCount={selectedCounts.get(db.data_source_id) ?? 0}
                  canManageSchema={canManageSchema}
                  isPermissionsLoading={isPermissionsLoading}
                  onSelect={(_id, checked) => toggleDatabase(db, checked)}
                />
              ))}
            </AgentRuntimeSelectionGrid>
            <AgentRuntimeSelectionPagination
              sentinelRef={databaseSentinelRef}
              isFetchingNextPage={databaseQuery.isFetchingNextPage}
              hasNextPage={Boolean(databaseQuery.hasNextPage)}
              hasItems={dbs.length > 0}
            />
          </>
        )}
      </AgentRuntimeSelectionDialog>
      {closeGuard}
    </>
  );
}

function DatabaseOption({
  db,
  selected,
  selectedCount,
  canManageSchema,
  isPermissionsLoading,
  onSelect,
}: {
  db: AgentDatabaseBindingCandidate;
  selected: boolean;
  selectedCount: number;
  canManageSchema: boolean;
  isPermissionsLoading: boolean;
  onSelect: (id: string, checked: boolean) => void;
}) {
  const t = useT('agents.agentRuntime');
  const label = db.name || t('database.unnamedDatabase');
  const tableCount = Math.max(0, db.table_count);
  const hasTables = tableCount > 0;

  return (
    <article
      className={cn(
        'flex min-h-36 flex-col overflow-hidden rounded-lg border bg-background text-left shadow-sm transition-colors',
        hasTables && 'hover:border-primary/40 hover:bg-muted/20',
        selected && 'border-primary bg-primary/5 hover:bg-primary/10',
        !hasTables && 'border-dashed bg-muted/10'
      )}
    >
      <button
        type="button"
        aria-pressed={selected}
        disabled={!hasTables && !selected}
        className="flex flex-1 flex-col p-3.5 text-left disabled:cursor-not-allowed"
        onClick={() => onSelect(db.data_source_id, !selected)}
      >
        <span className="flex min-h-8 items-center gap-3">
          <AgentRuntimeSelectionCardIcon className={hasTables ? 'text-primary' : undefined}>
            <Database />
          </AgentRuntimeSelectionCardIcon>
          <span className="flex min-h-8 min-w-0 flex-1 items-center">
            <span className="block truncate text-sm font-semibold">{label}</span>
          </span>
          <span
            className={cn(
              'flex size-5 shrink-0 items-center justify-center rounded-full border',
              selected ? 'border-primary bg-primary text-primary-foreground' : 'bg-background',
              !hasTables && 'bg-muted text-muted-foreground'
            )}
          >
            {selected ? <Check className="size-3.5" /> : null}
          </span>
        </span>
        <span className="mt-2.5 line-clamp-2 text-xs leading-5 text-muted-foreground">
          {db.description || t('database.noDescription')}
        </span>
      </button>
      <div className="flex min-h-11 items-center justify-between gap-2 border-t px-3.5 py-2">
        <div className="flex min-w-0 flex-wrap items-center gap-1.5">
          <Badge variant="subtle" className="w-fit">
            {hasTables
              ? t('database.availableTablesCount', { count: tableCount })
              : t('database.noAvailableTables')}
          </Badge>
          {hasTables && selected && selectedCount > 0 ? (
            <Badge variant="outline" className="w-fit">
              {t('database.selectedTablesCount', { count: selectedCount })}
            </Badge>
          ) : null}
        </div>
        {!hasTables && !isPermissionsLoading && canManageSchema ? (
          <Button asChild type="button" variant="ghost" size="sm" className="h-7 shrink-0 px-2">
            <Link
              href={`/console/db/${db.data_source_id}?createTable=1`}
              target="_blank"
              rel="noreferrer"
            >
              {t('database.createTableAction')}
              <ExternalLink className="size-3.5" />
            </Link>
          </Button>
        ) : null}
      </div>
    </article>
  );
}
