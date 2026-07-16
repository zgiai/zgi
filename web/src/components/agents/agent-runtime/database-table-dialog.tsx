'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import Link from 'next/link';
import {
  AlertCircle,
  ArrowRight,
  Check,
  Database,
  ExternalLink,
  Loader2,
  RefreshCw,
  SearchX,
  Table2,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { DATABASE_PERMISSION_ACTIONS } from '@/constants/permissions';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import agentService from '@/services/agent.service';
import type {
  AgentDatabaseBinding,
  AgentDatabaseTableBindingCandidate,
} from '@/services/types/agent';
import { normalizeAgentDatabaseBindings } from './database-binding-draft';
import {
  AgentRuntimeSelectionCardIcon,
  AgentRuntimeSelectionDialog,
  AgentRuntimeSelectionEmptyState,
  AgentRuntimeSelectionGrid,
  AgentRuntimeSelectionPagination,
  AgentRuntimeSelectionSkeleton,
} from './selection-dialog';
import { useSelectionDialogDraftGuard } from './use-selection-dialog-draft-guard';

interface DatabaseTableDialogDatabase {
  id: string;
  name?: string;
  tableCount?: number;
}

interface AgentRuntimeDatabaseTableDialogProps {
  agentId: string;
  open: boolean;
  databases: DatabaseTableDialogDatabase[];
  bindings: AgentDatabaseBinding[];
  initialBindings?: AgentDatabaseBinding[];
  canEditWritable: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (value: AgentDatabaseBinding[]) => void;
}

export function AgentRuntimeDatabaseTableDialog({
  agentId,
  open,
  databases,
  bindings,
  initialBindings = bindings,
  canEditWritable,
  onOpenChange,
  onConfirm,
}: AgentRuntimeDatabaseTableDialogProps) {
  const t = useT('agents.agentRuntime');
  const [localBindings, setLocalBindings] = useState<AgentDatabaseBinding[]>(initialBindings);
  const [activeDataSourceId, setActiveDataSourceId] = useState('');
  const [tableSearch, setTableSearch] = useState('');
  const debouncedTableSearch = useDebouncedValue(tableSearch.trim(), 300);
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canManageSchema = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.schemaManage);
  const databaseIds = useMemo(() => databases.map(database => database.id), [databases]);
  const hasTableSearch = debouncedTableSearch.length > 0;

  const activeTableQuery = useInfiniteQuery({
    queryKey: [
      ...AGENT_KEYS.databaseTableBindingCandidates(agentId, activeDataSourceId),
      'dialog',
      debouncedTableSearch,
    ],
    initialPageParam: 1,
    queryFn: ({ pageParam }) =>
      agentService.getAgentDatabaseTableBindingCandidates(agentId, activeDataSourceId, {
        query: debouncedTableSearch || undefined,
        page: pageParam as number,
        limit: 24,
      }),
    getNextPageParam: lastPage => (lastPage.data.has_more ? lastPage.data.page + 1 : undefined),
    enabled: open && Boolean(agentId) && Boolean(activeDataSourceId),
    staleTime: 3 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: 'always',
    retry: false,
  });

  useEffect(() => {
    if (!open) return;
    setLocalBindings(normalizeAgentDatabaseBindings(initialBindings));
    setActiveDataSourceId(databaseIds[0] ?? '');
    setTableSearch('');
  }, [databaseIds, initialBindings, open]);

  useEffect(() => {
    if (!open) return;
    setTableSearch('');
  }, [activeDataSourceId, open]);

  const selectedCounts = useMemo(
    () =>
      new Map(
        localBindings.map(binding => [binding.data_source_id, binding.table_ids.length] as const)
      ),
    [localBindings]
  );
  const databaseStatuses = useMemo(
    () =>
      databases.map(database => {
        const isActive = database.id === activeDataSourceId;
        const tables = isActive
          ? (activeTableQuery.data?.pages.flatMap(page => page.data.data ?? []) ?? [])
          : [];
        const loadedTotal = activeTableQuery.data?.pages[0]?.data.total;
        return {
          ...database,
          tables,
          tableCount: isActive
            ? (loadedTotal ?? database.tableCount ?? 0)
            : (database.tableCount ?? 0),
          isLoading: isActive && activeTableQuery.isLoading,
          isFetching: isActive && activeTableQuery.isFetching,
          error: isActive ? activeTableQuery.error : null,
          refetch: isActive ? activeTableQuery.refetch : undefined,
          selectedCount: selectedCounts.get(database.id) ?? 0,
        };
      }),
    [
      activeDataSourceId,
      activeTableQuery.data?.pages,
      activeTableQuery.error,
      activeTableQuery.isFetching,
      activeTableQuery.isLoading,
      activeTableQuery.refetch,
      databases,
      selectedCounts,
    ]
  );
  const activeIndex = databaseStatuses.findIndex(database => database.id === activeDataSourceId);
  const activeDatabase = activeIndex >= 0 ? databaseStatuses[activeIndex] : undefined;
  const nextDatabase =
    activeIndex >= 0 && activeIndex < databaseStatuses.length - 1
      ? databaseStatuses[activeIndex + 1]
      : undefined;
  const activeTables = useMemo(() => activeDatabase?.tables ?? [], [activeDatabase?.tables]);
  const selectedTableIDs = useMemo(
    () =>
      new Set(
        localBindings.find(binding => binding.data_source_id === activeDataSourceId)?.table_ids ??
          []
      ),
    [activeDataSourceId, localBindings]
  );
  const writableTableIDs = useMemo(
    () =>
      new Set(
        localBindings.find(binding => binding.data_source_id === activeDataSourceId)
          ?.writable_table_ids ?? []
      ),
    [activeDataSourceId, localBindings]
  );
  const activeTableIDs = useMemo(
    () => activeTables.map(table => table.table_id).filter(Boolean),
    [activeTables]
  );
  const selectedActiveTableIDs = useMemo(
    () => activeTableIDs.filter(tableID => selectedTableIDs.has(tableID)),
    [activeTableIDs, selectedTableIDs]
  );
  const allActiveTablesSelected =
    activeTableIDs.length > 0 && activeTableIDs.every(tableID => selectedTableIDs.has(tableID));
  const allSelectedActiveTablesWritable =
    selectedActiveTableIDs.length > 0 &&
    selectedActiveTableIDs.every(tableID => writableTableIDs.has(tableID));

  const fetchNextTablePage = activeTableQuery.fetchNextPage;
  const loadNextTablePage = useCallback(() => fetchNextTablePage(), [fetchNextTablePage]);
  const tableSentinelRef = useInfiniteObserver({
    enabled: open && Boolean(activeDataSourceId),
    hasNextPage: Boolean(activeTableQuery.hasNextPage),
    isFetchingNextPage: activeTableQuery.isFetchingNextPage,
    fetchNextPage: loadNextTablePage,
    rootMargin: '240px',
  });
  const sessionDataSourceSet = useMemo(() => new Set(databaseIds), [databaseIds]);
  const selectedCount = useMemo(
    () =>
      localBindings.reduce(
        (count, binding) =>
          sessionDataSourceSet.has(binding.data_source_id)
            ? count + binding.table_ids.length
            : count,
        0
      ),
    [localBindings, sessionDataSourceSet]
  );
  const normalizedOriginalBindings = useMemo(
    () => normalizeAgentDatabaseBindings(bindings),
    [bindings]
  );
  const normalizedLocalBindings = useMemo(
    () => normalizeAgentDatabaseBindings(localBindings),
    [localBindings]
  );
  const isDirty = useMemo(
    () => JSON.stringify(normalizedLocalBindings) !== JSON.stringify(normalizedOriginalBindings),
    [normalizedLocalBindings, normalizedOriginalBindings]
  );
  const isLoadingAnyDatabase = activeTableQuery.isLoading;
  const commitSelection = useCallback(
    () => onConfirm(normalizedLocalBindings),
    [normalizedLocalBindings, onConfirm]
  );
  const { requestOpenChange, requestClose, saveAndClose, closeGuard } =
    useSelectionDialogDraftGuard({
      open,
      isDirty,
      disabled: isLoadingAnyDatabase,
      onOpenChange,
      onSave: commitSelection,
    });

  const handleToggleTable = (tableID: string, checked: boolean) => {
    if (!activeDataSourceId) return;
    const next = bindingsToMap(localBindings);
    const selected = next.get(activeDataSourceId) ?? {
      readable: new Set<string>(),
      writable: new Set<string>(),
    };
    if (checked) {
      selected.readable.add(tableID);
    } else {
      selected.readable.delete(tableID);
      selected.writable.delete(tableID);
    }
    if (selected.readable.size > 0) {
      next.set(activeDataSourceId, selected);
    } else {
      next.delete(activeDataSourceId);
    }
    setLocalBindings(bindingsFromMap(next));
  };

  const handleToggleAllTables = (checked: boolean) => {
    if (!activeDataSourceId || activeTableIDs.length === 0) return;
    const next = bindingsToMap(localBindings);
    const selected = next.get(activeDataSourceId) ?? {
      readable: new Set<string>(),
      writable: new Set<string>(),
    };
    activeTableIDs.forEach(tableID => {
      if (checked) {
        selected.readable.add(tableID);
      } else {
        selected.readable.delete(tableID);
        selected.writable.delete(tableID);
      }
    });
    if (selected.readable.size > 0) {
      next.set(activeDataSourceId, selected);
    } else {
      next.delete(activeDataSourceId);
    }
    setLocalBindings(bindingsFromMap(next));
  };

  const handleToggleAllWritable = (checked: boolean) => {
    if (!canEditWritable || !activeDataSourceId || selectedActiveTableIDs.length === 0) return;
    const next = bindingsToMap(localBindings);
    const selected = next.get(activeDataSourceId);
    if (!selected) return;
    selectedActiveTableIDs.forEach(tableID => {
      if (checked) {
        selected.writable.add(tableID);
      } else {
        selected.writable.delete(tableID);
      }
    });
    next.set(activeDataSourceId, selected);
    setLocalBindings(bindingsFromMap(next));
  };

  const handleToggleWritable = (tableID: string, checked: boolean) => {
    if (!canEditWritable || !activeDataSourceId) return;
    const next = bindingsToMap(localBindings);
    const selected = next.get(activeDataSourceId);
    if (!selected || !selected.readable.has(tableID)) return;
    if (checked) {
      selected.writable.add(tableID);
    } else {
      selected.writable.delete(tableID);
    }
    next.set(activeDataSourceId, selected);
    setLocalBindings(bindingsFromMap(next));
  };

  return (
    <>
      <AgentRuntimeSelectionDialog
        open={open}
        title={t('database.tableBatchDialogTitle')}
        description={t('database.tableDialogDescription')}
        selectedCount={selectedCount}
        search={tableSearch}
        searchPlaceholder={t('database.searchTable')}
        isSearching={
          tableSearch.trim() !== debouncedTableSearch || Boolean(activeDatabase?.isFetching)
        }
        onOpenChange={requestOpenChange}
        onChangeSearch={setTableSearch}
        footer={
          <>
            <Button type="button" variant="ghost" onClick={requestClose}>
              {t('database.cancel')}
            </Button>
            <Button type="button" disabled={isLoadingAnyDatabase} onClick={saveAndClose}>
              {isLoadingAnyDatabase ? <Loader2 className="size-4 animate-spin" /> : null}
              {t('database.saveAll')}
            </Button>
          </>
        }
      >
        <div className="grid h-full min-h-[460px] gap-4 lg:grid-cols-[240px_minmax(0,1fr)]">
          <aside className="flex min-h-0 flex-col overflow-hidden rounded-lg border bg-muted/15">
            <div className="border-b px-3.5 py-3">
              <div className="text-sm font-semibold">{t('database.selectedDatabases')}</div>
              <div className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('database.noTableBindingRule')}
              </div>
            </div>
            <div className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
              {databaseStatuses.map(database => (
                <DatabaseStatusButton
                  key={database.id}
                  name={database.name || t('database.unnamedDatabase')}
                  active={database.id === activeDataSourceId}
                  isLoading={database.isLoading}
                  isFetching={database.isFetching}
                  hasError={Boolean(database.error)}
                  tableCount={database.tableCount}
                  selectedCount={database.selectedCount}
                  onClick={() => setActiveDataSourceId(database.id)}
                />
              ))}
            </div>
          </aside>

          <section className="flex min-h-[420px] min-w-0 flex-col overflow-hidden rounded-lg border bg-background">
            <div className="flex shrink-0 items-center justify-between gap-3 border-b px-4 py-3">
              <div className="min-w-0">
                <div className="truncate text-sm font-semibold">
                  {activeDatabase?.name || t('database.unnamedDatabase')}
                </div>
                <div className="mt-0.5 text-xs text-muted-foreground">
                  {t('database.databaseProgress', {
                    current: Math.max(activeIndex + 1, 0),
                    total: databaseStatuses.length,
                  })}
                </div>
              </div>
              {activeDatabase ? (
                <Badge variant="subtle" className="shrink-0">
                  {t('database.selectedTablesCount', {
                    count: activeDatabase.selectedCount,
                  })}
                </Badge>
              ) : null}
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto p-4">
              {activeDatabase?.isLoading ||
              (!activeDatabase && isLoadingAnyDatabase) ||
              (activeTables.length === 0 && isPermissionsLoading) ? (
                <AgentRuntimeSelectionSkeleton />
              ) : activeDatabase?.error ? (
                <AgentRuntimeSelectionEmptyState
                  icon={<AlertCircle />}
                  title={t('database.tableLoadFailedTitle')}
                  description={t('database.loadTablesFailed')}
                  className="min-h-[340px]"
                  action={
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={activeDatabase.isFetching}
                      onClick={() => void activeDatabase.refetch?.()}
                    >
                      <RefreshCw
                        className={cn('size-4', activeDatabase.isFetching && 'animate-spin')}
                      />
                      {t('database.retryLoadTables')}
                    </Button>
                  }
                />
              ) : activeTables.length === 0 && hasTableSearch ? (
                <AgentRuntimeSelectionEmptyState
                  variant="search"
                  icon={<SearchX />}
                  title={t('database.tableSearchEmptyTitle')}
                  description={t('database.tableSearchEmptyDescription', {
                    query: debouncedTableSearch,
                  })}
                  className="min-h-[340px]"
                  action={
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setTableSearch('')}
                    >
                      {t('selectionDialog.clearSearch')}
                    </Button>
                  }
                />
              ) : activeTables.length === 0 ? (
                <AgentRuntimeSelectionEmptyState
                  icon={<Table2 />}
                  title={t('database.tableEmptyTitle')}
                  description={t(
                    canManageSchema
                      ? 'database.tableEmptyDescription'
                      : 'database.tableEmptyUnavailableDescription'
                  )}
                  className="min-h-[340px]"
                  action={
                    <div className="flex flex-wrap items-center justify-center gap-2">
                      {canManageSchema && activeDatabase ? (
                        <Button asChild size="sm">
                          <Link
                            href={`/console/db/${activeDatabase.id}`}
                            target="_blank"
                            rel="noreferrer"
                          >
                            {t('database.createTableAction')}
                            <ExternalLink className="size-4" />
                          </Link>
                        </Button>
                      ) : null}
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={activeDatabase?.isFetching}
                        onClick={() => void activeDatabase?.refetch?.()}
                      >
                        <RefreshCw
                          className={cn('size-4', activeDatabase?.isFetching && 'animate-spin')}
                        />
                        {t('database.refreshTables')}
                      </Button>
                      {nextDatabase ? (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={() => setActiveDataSourceId(nextDatabase.id)}
                        >
                          {t('database.nextDatabase')}
                          <ArrowRight className="size-4" />
                        </Button>
                      ) : null}
                    </div>
                  }
                />
              ) : (
                <div className="space-y-4">
                  {!hasTableSearch ? (
                    <div className="flex flex-wrap items-center gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="h-8 px-2.5"
                        disabled={activeTableIDs.length === 0 || activeTableQuery.hasNextPage}
                        onClick={() => handleToggleAllTables(!allActiveTablesSelected)}
                      >
                        {allActiveTablesSelected
                          ? t('database.clearSelectedDatabaseTables')
                          : t('database.selectAllDatabaseTables')}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="h-8 px-2.5"
                        disabled={!canEditWritable || selectedActiveTableIDs.length === 0}
                        onClick={() => handleToggleAllWritable(!allSelectedActiveTablesWritable)}
                      >
                        {allSelectedActiveTablesWritable
                          ? t('database.clearWritableTables')
                          : t('database.makeSelectedTablesWritable')}
                      </Button>
                    </div>
                  ) : null}
                  {!canEditWritable ? (
                    <div className="rounded-md border border-dashed bg-muted/20 p-3 text-xs text-muted-foreground">
                      {t('database.writePermissionRequired')}
                    </div>
                  ) : null}
                  <AgentRuntimeSelectionGrid>
                    {activeTables.map(table => (
                      <TableOption
                        key={table.table_id}
                        table={table}
                        checked={selectedTableIDs.has(table.table_id)}
                        writable={writableTableIDs.has(table.table_id)}
                        canEditWritable={canEditWritable}
                        onToggleTable={handleToggleTable}
                        onToggleWritable={handleToggleWritable}
                      />
                    ))}
                  </AgentRuntimeSelectionGrid>
                  <AgentRuntimeSelectionPagination
                    sentinelRef={tableSentinelRef}
                    isFetchingNextPage={activeTableQuery.isFetchingNextPage}
                    hasNextPage={Boolean(activeTableQuery.hasNextPage)}
                    hasItems={activeTables.length > 0}
                  />
                </div>
              )}
            </div>
          </section>
        </div>
      </AgentRuntimeSelectionDialog>
      {closeGuard}
    </>
  );
}

function DatabaseStatusButton({
  name,
  active,
  isLoading,
  isFetching,
  hasError,
  tableCount,
  selectedCount,
  onClick,
}: {
  name: string;
  active: boolean;
  isLoading: boolean;
  isFetching: boolean;
  hasError: boolean;
  tableCount: number;
  selectedCount: number;
  onClick: () => void;
}) {
  const t = useT('agents.agentRuntime');
  const status = isLoading
    ? t('database.loadingTablesShort')
    : hasError
      ? t('database.loadTablesFailedShort')
      : tableCount === 0
        ? t('database.noTablesShort')
        : selectedCount > 0
          ? t('database.selectedTablesCount', { count: selectedCount })
          : t('database.pendingTableSelection');

  return (
    <button
      type="button"
      aria-pressed={active}
      className={cn(
        'flex w-full items-center gap-2.5 rounded-md border border-transparent px-2.5 py-2 text-left transition-colors hover:bg-muted/60',
        active && 'border-primary/25 bg-primary/5'
      )}
      onClick={onClick}
    >
      <AgentRuntimeSelectionCardIcon
        className={cn(
          'size-7 [&>svg]:size-3.5',
          active && 'border-primary/20 text-primary',
          hasError && 'text-destructive'
        )}
      >
        {hasError ? <AlertCircle /> : <Database />}
      </AgentRuntimeSelectionCardIcon>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-xs font-medium">{name}</span>
        <span
          className={cn(
            'mt-0.5 flex items-center gap-1 truncate text-[11px] text-muted-foreground',
            hasError && 'text-destructive'
          )}
        >
          {isLoading || isFetching ? <Loader2 className="size-3 shrink-0 animate-spin" /> : null}
          {status}
        </span>
      </span>
      {selectedCount > 0 && !isLoading && !hasError ? (
        <Check className="size-3.5 shrink-0 text-primary" />
      ) : null}
    </button>
  );
}

function TableOption({
  table,
  checked,
  writable,
  canEditWritable,
  onToggleTable,
  onToggleWritable,
}: {
  table: AgentDatabaseTableBindingCandidate;
  checked: boolean;
  writable: boolean;
  canEditWritable: boolean;
  onToggleTable: (tableID: string, checked: boolean) => void;
  onToggleWritable: (tableID: string, checked: boolean) => void;
}) {
  const t = useT('agents.agentRuntime');
  const label = tableLabel(table, t('database.unnamedTable'));
  const description = tableDescription(table, t('database.noDescription'));

  return (
    <div
      className={cn(
        'flex min-h-32 w-full cursor-pointer flex-col rounded-lg border bg-background p-3.5 text-left shadow-sm transition-colors hover:border-primary/40 hover:bg-muted/20',
        checked && 'border-primary bg-primary/5 hover:bg-primary/10'
      )}
      role="button"
      tabIndex={0}
      aria-pressed={checked}
      onClick={() => onToggleTable(table.table_id, !checked)}
      onKeyDown={event => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          onToggleTable(table.table_id, !checked);
        }
      }}
    >
      <span className="flex min-h-8 items-center gap-3">
        <AgentRuntimeSelectionCardIcon className="text-primary">
          <Table2 />
        </AgentRuntimeSelectionCardIcon>
        <span className="flex min-h-8 min-w-0 flex-1 items-center">
          <span className="block truncate text-sm font-semibold">{label}</span>
        </span>
        <span
          className={cn(
            'flex size-5 shrink-0 items-center justify-center rounded-full border',
            checked ? 'border-primary bg-primary text-primary-foreground' : 'bg-background'
          )}
          aria-label={t('database.selectTableForBinding', { name: label })}
        >
          {checked ? <Check className="size-3.5" /> : null}
        </span>
      </span>
      <span className="mt-2.5 line-clamp-2 text-xs leading-5 text-muted-foreground">
        {description}
      </span>
      {checked ? (
        <span
          className="mt-auto flex w-fit items-center gap-2 pt-3 text-xs text-muted-foreground"
          onPointerDown={event => event.stopPropagation()}
          onMouseDown={event => event.stopPropagation()}
          onClick={event => event.stopPropagation()}
          onKeyDown={event => event.stopPropagation()}
        >
          <Badge variant="subtle">
            {writable ? t('database.writeEnabled') : t('database.readOnly')}
          </Badge>
          <Switch
            checked={writable}
            disabled={!canEditWritable}
            onCheckedChange={value => onToggleWritable(table.table_id, value === true)}
            aria-label={t('database.allowWriteForTable', { name: label })}
          />
          <span>{t('database.allowWrite')}</span>
        </span>
      ) : null}
    </div>
  );
}

function tableLabel(table: AgentDatabaseTableBindingCandidate, fallback: string) {
  return table.name || table.physical_table_name || fallback;
}

function tableDescription(table: AgentDatabaseTableBindingCandidate, fallback: string) {
  const description = table.description?.trim();
  if (!description) return fallback;

  const technicalNames = [table.name, table.physical_table_name, table.table_id]
    .map(value => value?.trim())
    .filter(Boolean);
  if (technicalNames.includes(description)) return fallback;
  if (/^zgi_base_tbl_/i.test(description)) return fallback;

  return description;
}

function bindingsToMap(bindings: AgentDatabaseBinding[]) {
  const next = new Map<string, { readable: Set<string>; writable: Set<string> }>();
  normalizeAgentDatabaseBindings(bindings).forEach(binding => {
    next.set(binding.data_source_id, {
      readable: new Set(binding.table_ids),
      writable: new Set(binding.writable_table_ids ?? []),
    });
  });
  return next;
}

function bindingsFromMap(
  values: Map<string, { readable: Set<string>; writable: Set<string> }>
): AgentDatabaseBinding[] {
  return Array.from(values.entries())
    .map(([dataSourceID, tableIDs]) => ({
      data_source_id: dataSourceID,
      table_ids: Array.from(tableIDs.readable).sort(),
      writable_table_ids: Array.from(tableIDs.writable)
        .filter(tableID => tableIDs.readable.has(tableID))
        .sort(),
    }))
    .filter(binding => binding.table_ids.length > 0)
    .sort((left, right) => left.data_source_id.localeCompare(right.data_source_id));
}
