'use client';

import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { AlertCircle, Database, Plus, Table2, Trash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useDbTables } from '@/hooks/db/use-db-tables';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import agentService from '@/services/agent.service';
import type {
  AgentBindingHealth,
  AgentBindingHealthItem,
  AgentDatabaseBinding,
  AgentDatabaseBindingCandidate,
} from '@/services/types/agent';
import type { DbTable } from '@/services/types/db';
import { AgentRuntimeDatabaseDialog } from '../database-dialog';
import { AgentRuntimeDatabaseTableDialog } from '../database-table-dialog';
import { AgentBindingHealthBadge } from '../binding-health';
import { planAgentDatabaseSelection } from '../database-binding-draft';
import { AgentRuntimeResourceCard, AgentRuntimeResourceSection } from '../resource-section';
import type { AgentConfigSection } from '../types';
import {
  DATABASE_PERMISSION_ACTIONS,
  DATABASE_READ_BINDING_PERMISSION_CODES,
} from '@/constants/permissions';
import { tablesForDataSource } from '../utils';

interface AgentRuntimeDatabaseSectionProps {
  agentId: string;
  open: boolean;
  bindings: AgentDatabaseBinding[];
  bindingHealth?: AgentBindingHealth;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeBindings: (value: AgentDatabaseBinding[]) => void;
}

interface DatabaseTableDialogSession {
  databases: Array<{ id: string; name?: string; tableCount?: number }>;
  initialBindings: AgentDatabaseBinding[];
}

export function AgentRuntimeDatabaseSection({
  agentId,
  open,
  bindings,
  bindingHealth,
  readOnly = false,
  onToggleSection,
  onChangeBindings,
}: AgentRuntimeDatabaseSectionProps) {
  const t = useT('agents.agentRuntime');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [tableDialogSession, setTableDialogSession] = useState<DatabaseTableDialogSession | null>(
    null
  );
  const { hasAnyPermission, hasAllPermissions } = useAccountPermissions();
  const canBindReadableDatabase = hasAllPermissions(DATABASE_READ_BINDING_PERMISSION_CODES);
  const databaseCandidatesQuery = useQuery({
    queryKey: [...AGENT_KEYS.databaseBindingCandidates(agentId), 'section'],
    queryFn: () =>
      agentService.getAgentDatabaseBindingCandidates(agentId, {
        page: 1,
        limit: 100,
        available_only: false,
      }),
    enabled: open && canBindReadableDatabase && Boolean(agentId),
    staleTime: 60_000,
    retry: false,
  });
  const canUseAiQuery = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.aiQueryRead);
  const canWriteDatabaseRecords = hasAnyPermission([
    ...DATABASE_PERMISSION_ACTIONS.recordCreate,
    ...DATABASE_PERMISSION_ACTIONS.recordUpdate,
    ...DATABASE_PERMISSION_ACTIONS.recordDelete,
  ]);
  const canEditWritable =
    !readOnly && canBindReadableDatabase && canUseAiQuery && canWriteDatabaseRecords;
  const selectedCount = bindings.reduce((count, binding) => count + binding.table_ids.length, 0);
  const dbsByID = useMemo(() => {
    const byID = new Map<string, AgentDatabaseBindingCandidate>();
    (databaseCandidatesQuery.data?.data.data ?? []).forEach(db => {
      if (db.data_source_id) byID.set(db.data_source_id, db);
    });
    return byID;
  }, [databaseCandidatesQuery.data?.data.data]);
  const isDbsLoading = databaseCandidatesQuery.isLoading;

  const removeTable = (dataSourceID: string, tableID: string) => {
    if (readOnly) return;
    onChangeBindings(
      bindings
        .map(binding =>
          binding.data_source_id === dataSourceID
            ? {
                ...binding,
                table_ids: binding.table_ids.filter(id => id !== tableID),
                writable_table_ids: (binding.writable_table_ids ?? []).filter(id => id !== tableID),
              }
            : binding
        )
        .filter(binding => binding.table_ids.length > 0)
    );
  };

  const updateWritableForTable = (dataSourceID: string, tableID: string, checked: boolean) => {
    if (!canEditWritable) return;
    onChangeBindings(
      bindings.map(binding => {
        if (binding.data_source_id !== dataSourceID) return binding;
        const writable = new Set(binding.writable_table_ids ?? []);
        if (checked && binding.table_ids.includes(tableID)) {
          writable.add(tableID);
        } else {
          writable.delete(tableID);
        }
        return {
          ...binding,
          writable_table_ids: Array.from(writable)
            .filter(id => binding.table_ids.includes(id))
            .sort(),
        };
      })
    );
  };

  const updateWritableForDatabase = (dataSourceID: string, checked: boolean) => {
    if (!canEditWritable) return;
    onChangeBindings(
      bindings.map(binding =>
        binding.data_source_id === dataSourceID
          ? {
              ...binding,
              writable_table_ids: checked ? [...binding.table_ids].sort() : [],
            }
          : binding
      )
    );
  };

  const openTableDialog = (dataSourceID: string) => {
    const database = dbsByID.get(dataSourceID);
    const databaseHealthItem = bindingHealth?.items.find(
      item => item.binding_type === 'database' && item.resource_id === dataSourceID
    );
    if (readOnly || (!database && databaseHealthItem?.status !== 'active')) return;
    setTableDialogSession({
      databases: [
        {
          id: dataSourceID,
          name: database?.name ?? databaseHealthItem?.display_name,
          tableCount: database?.table_count,
        },
      ],
      initialBindings: bindings,
    });
  };

  const handleConfirmDatabases = (
    dataSourceIDs: string[],
    selectedDatabases: AgentDatabaseBindingCandidate[]
  ) => {
    if (readOnly) return;
    const plan = planAgentDatabaseSelection(bindings, dataSourceIDs);

    if (plan.newDataSourceIds.length === 0) {
      onChangeBindings(plan.initialBindings);
      return;
    }

    const selectedDatabasesByID = new Map(selectedDatabases.map(db => [db.data_source_id, db]));
    setTableDialogSession({
      databases: plan.newDataSourceIds.map(dataSourceID => ({
        id: dataSourceID,
        name: selectedDatabasesByID.get(dataSourceID)?.name ?? dbsByID.get(dataSourceID)?.name,
        tableCount:
          selectedDatabasesByID.get(dataSourceID)?.table_count ??
          dbsByID.get(dataSourceID)?.table_count,
      })),
      initialBindings: plan.initialBindings,
    });
  };

  return (
    <>
      <AgentRuntimeResourceSection
        title={t('sections.databases')}
        section="databases"
        open={open}
        count={selectedCount}
        addLabel={t('database.add')}
        addTooltip={
          canBindReadableDatabase
            ? t('database.bindTableTooltip')
            : t('database.bindingPermissionRequired')
        }
        helpText={t('database.helpText')}
        emptyText={t('database.emptySelected')}
        isLoading={isDbsLoading}
        onToggleSection={onToggleSection}
        onAdd={() => {
          if (!canBindReadableDatabase) return;
          setDialogOpen(true);
        }}
        readOnly={readOnly || !canBindReadableDatabase}
      >
        <div className="space-y-2">
          {bindings.map(binding => (
            <DatabaseBindingCard
              key={binding.data_source_id}
              dataSourceID={binding.data_source_id}
              dataSourceName={dbsByID.get(binding.data_source_id)?.name}
              isScopedDatabase={dbsByID.has(binding.data_source_id)}
              candidateLoadFailed={databaseCandidatesQuery.isError}
              tableIDs={binding.table_ids}
              writableTableIDs={binding.writable_table_ids ?? []}
              databaseHealthItem={bindingHealth?.items.find(
                item =>
                  item.binding_type === 'database' && item.resource_id === binding.data_source_id
              )}
              tableHealthItems={bindingHealth?.items.filter(
                item =>
                  item.binding_type === 'database_table' &&
                  item.parent_resource_id === binding.data_source_id
              )}
              readOnly={readOnly}
              canReadBinding={canBindReadableDatabase}
              canEditWritable={canEditWritable}
              onOpenTableDialog={openTableDialog}
              onRemoveTable={removeTable}
              onChangeWritableTable={updateWritableForTable}
              onChangeWritableDatabase={updateWritableForDatabase}
            />
          ))}
        </div>
      </AgentRuntimeResourceSection>

      <AgentRuntimeDatabaseDialog
        agentId={agentId}
        open={dialogOpen && canBindReadableDatabase}
        bindings={bindings}
        onOpenChange={setDialogOpen}
        onConfirmDatabases={handleConfirmDatabases}
      />
      <AgentRuntimeDatabaseTableDialog
        agentId={agentId}
        open={Boolean(tableDialogSession)}
        databases={tableDialogSession?.databases ?? []}
        bindings={bindings}
        initialBindings={tableDialogSession?.initialBindings ?? bindings}
        canEditWritable={canEditWritable}
        onOpenChange={open => {
          if (!open) setTableDialogSession(null);
        }}
        onConfirm={value => {
          onChangeBindings(value);
          setTableDialogSession(null);
        }}
      />
    </>
  );
}

function DatabaseBindingCard({
  dataSourceID,
  dataSourceName,
  isScopedDatabase,
  candidateLoadFailed,
  tableIDs,
  writableTableIDs,
  databaseHealthItem,
  tableHealthItems,
  readOnly,
  canReadBinding,
  canEditWritable,
  onOpenTableDialog,
  onRemoveTable,
  onChangeWritableTable,
  onChangeWritableDatabase,
}: {
  dataSourceID: string;
  dataSourceName?: string;
  isScopedDatabase: boolean;
  candidateLoadFailed: boolean;
  tableIDs: string[];
  writableTableIDs: string[];
  databaseHealthItem?: AgentBindingHealthItem;
  tableHealthItems?: AgentBindingHealthItem[];
  readOnly: boolean;
  canReadBinding: boolean;
  canEditWritable: boolean;
  onOpenTableDialog: (dataSourceID: string) => void;
  onRemoveTable: (dataSourceID: string, tableID: string) => void;
  onChangeWritableTable: (dataSourceID: string, tableID: string, checked: boolean) => void;
  onChangeWritableDatabase: (dataSourceID: string, checked: boolean) => void;
}) {
  const writableSet = useMemo(() => new Set(writableTableIDs), [writableTableIDs]);
  const t = useT('agents.agentRuntime');
  const {
    tables: rawTables,
    isLoading,
    error,
  } = useDbTables(dataSourceID, {
    enabled:
      canReadBinding &&
      (isScopedDatabase || databaseHealthItem?.status === 'active') &&
      tableIDs.length > 0,
  });
  const tables = useMemo(
    () => tablesForDataSource(rawTables, dataSourceID),
    [dataSourceID, rawTables]
  );
  const tablesByID = useMemo(() => new Map(tables.map(table => [table.id, table])), [tables]);
  const databaseUnavailable = databaseHealthItem
    ? databaseHealthItem.status === 'unavailable'
    : !candidateLoadFailed && !isScopedDatabase;
  const databaseLabel =
    dataSourceName ||
    databaseHealthItem?.display_name ||
    t(databaseUnavailable ? 'database.databaseUnavailable' : 'database.loadFailedTitle');
  const allWritable = tableIDs.length > 0 && tableIDs.every(tableID => writableSet.has(tableID));
  const cannotReadBinding = !canReadBinding;

  return (
    <AgentRuntimeResourceCard
      icon={
        error || candidateLoadFailed || databaseUnavailable || cannotReadBinding ? (
          <AlertCircle className="size-4" />
        ) : (
          <Database className="size-4" />
        )
      }
      title={databaseLabel}
      description={candidateLoadFailed ? t('database.loadFailedDescription') : undefined}
      healthItem={databaseHealthItem}
      error={Boolean(error) || candidateLoadFailed || databaseUnavailable || cannotReadBinding}
      action={
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              isIcon
              className="size-8 shrink-0 text-muted-foreground hover:text-foreground"
              aria-label={t('database.addTableForDatabase', { name: databaseLabel })}
              disabled={readOnly || databaseUnavailable || cannotReadBinding}
              onClick={() => onOpenTableDialog(dataSourceID)}
            >
              <Plus className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('database.addTable')}</TooltipContent>
        </Tooltip>
      }
    >
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-md bg-muted/25 px-3 py-2 text-xs text-muted-foreground">
        <span>{t('database.selectedTablesCount', { count: tableIDs.length })}</span>
        <span className="flex items-center gap-2">
          <span>{t('database.allowWriteAll')}</span>
          <Switch
            checked={allWritable}
            disabled={databaseUnavailable || !canEditWritable || tableIDs.length === 0}
            onCheckedChange={checked => onChangeWritableDatabase(dataSourceID, checked)}
            aria-label={t('database.allowWriteAllForDatabase', { name: databaseLabel })}
          />
        </span>
      </div>
      {cannotReadBinding ? (
        <div className="mb-2 rounded-md border border-destructive/20 bg-destructive/5 p-2 text-xs text-destructive">
          {t('database.bindingPermissionRequired')}
        </div>
      ) : null}
      {error ? (
        <div className="mb-2 rounded-md border border-destructive/20 bg-destructive/5 p-2 text-xs text-destructive">
          {t('database.loadTablesFailed')}
        </div>
      ) : null}
      {isLoading ? (
        <div className="space-y-2">
          {tableIDs.map(tableID => (
            <Skeleton key={tableID} className="h-9 w-full" />
          ))}
        </div>
      ) : (
        <div className="space-y-2">
          {tableIDs.map(tableID => {
            const table = tablesByID.get(tableID);
            const label = table
              ? tableLabel(table, t('database.unnamedTable'))
              : t('database.tableUnavailable');
            const missing = tables.length > 0 && !table;
            const tableHealthItem = tableHealthItems?.find(item => item.resource_id === tableID);
            return (
              <div
                key={tableID}
                className="flex min-h-11 items-center justify-between gap-2 rounded-md bg-muted/35 px-3 py-2"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 items-center gap-2">
                    <Table2 className="size-3.5 shrink-0 text-muted-foreground" />
                    <div className="truncate text-xs font-medium">{label}</div>
                    <AgentBindingHealthBadge item={tableHealthItem} />
                  </div>
                  <div className="mt-1 flex min-w-0 items-center gap-2">
                    {missing ? (
                      <span className="truncate text-[11px] text-muted-foreground/70">
                        {t('database.tableUnavailable')}
                      </span>
                    ) : null}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <Badge variant="subtle">
                    {writableSet.has(tableID) ? t('database.writeEnabled') : t('database.readOnly')}
                  </Badge>
                  <Switch
                    checked={writableSet.has(tableID)}
                    disabled={databaseUnavailable || !canEditWritable}
                    onCheckedChange={checked =>
                      onChangeWritableTable(dataSourceID, tableID, checked)
                    }
                    aria-label={t('database.allowWriteForTable', { name: label })}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    isIcon
                    className="size-7 text-muted-foreground hover:text-destructive"
                    aria-label={t('database.removeTable', { name: label })}
                    disabled={readOnly || cannotReadBinding}
                    onClick={() => onRemoveTable(dataSourceID, tableID)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </AgentRuntimeResourceCard>
  );
}

function tableLabel(table: DbTable, fallback: string) {
  return table.name || table.table_name || fallback;
}
