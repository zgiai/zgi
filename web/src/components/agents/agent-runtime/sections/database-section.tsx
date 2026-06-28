'use client';

import { useMemo, useState } from 'react';
import { AlertCircle, Database, Plus, Table2, Trash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useDbTables } from '@/hooks/db/use-db-tables';
import { useDbsBasic } from '@/hooks/db/use-dbs';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import type { AgentDatabaseBinding } from '@/services/types/agent';
import type { DbTable } from '@/services/types/db';
import { AgentRuntimeDatabaseDialog } from '../database-dialog';
import { AgentRuntimeDatabaseTableDialog } from '../database-table-dialog';
import { AgentRuntimeResourceCard, AgentRuntimeResourceSection } from '../resource-section';
import type { AgentConfigSection } from '../types';
import { DATABASE_PERMISSION_ACTIONS } from '@/constants/permissions';

interface AgentRuntimeDatabaseSectionProps {
  open: boolean;
  bindings: AgentDatabaseBinding[];
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeBindings: (value: AgentDatabaseBinding[]) => void;
}

export function AgentRuntimeDatabaseSection({
  open,
  bindings,
  readOnly = false,
  onToggleSection,
  onChangeBindings,
}: AgentRuntimeDatabaseSectionProps) {
  const t = useT('agents.agentRuntime');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [tableDialogDbId, setTableDialogDbId] = useState('');
  const [pendingTableDialogDbIds, setPendingTableDialogDbIds] = useState<string[]>([]);
  const { dbs, isLoading: isDbsLoading } = useDbsBasic({}, { enabled: open });
  const { hasAnyPermission } = useAccountPermissions();
  const canUseAiQuery = hasAnyPermission([
    ...DATABASE_PERMISSION_ACTIONS.aiQueryRead,
    ...DATABASE_PERMISSION_ACTIONS.aiQueryWrite,
  ]);
  const canWriteDatabaseRecords = hasAnyPermission([
    ...DATABASE_PERMISSION_ACTIONS.recordCreate,
    ...DATABASE_PERMISSION_ACTIONS.recordUpdate,
    ...DATABASE_PERMISSION_ACTIONS.recordDelete,
  ]);
  const canEditWritable = !readOnly && canUseAiQuery && canWriteDatabaseRecords;
  const selectedCount = bindings.reduce((count, binding) => count + binding.table_ids.length, 0);
  const dbsByID = useMemo(() => new Map(dbs.map(db => [db.id, db])), [dbs]);
  const tableDialogDb = tableDialogDbId ? dbsByID.get(tableDialogDbId) : undefined;

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
    if (readOnly) return;
    setTableDialogDbId(dataSourceID);
  };

  const handleCloseTableDialog = () => {
    const [nextDbId, ...restDbIds] = pendingTableDialogDbIds;
    setPendingTableDialogDbIds(restDbIds);
    setTableDialogDbId(nextDbId ?? '');
  };

  const handleConfirmDatabases = (dataSourceIDs: string[]) => {
    if (readOnly) return;
    const selected = new Set(dataSourceIDs);
    const existing = new Set(bindings.map(binding => binding.data_source_id));
    const keptBindings = bindings.filter(binding => selected.has(binding.data_source_id));
    const pendingDbIds = dataSourceIDs.filter(dataSourceID => !existing.has(dataSourceID));

    onChangeBindings(keptBindings);
    if (pendingDbIds.length > 0) {
      const [firstDbId, ...restDbIds] = pendingDbIds;
      setPendingTableDialogDbIds(restDbIds);
      setTableDialogDbId(firstDbId ?? '');
    }
  };

  return (
    <>
      <AgentRuntimeResourceSection
        title={t('sections.databases')}
        section="databases"
        open={open}
        count={selectedCount}
        addLabel={t('database.add')}
        addTooltip={t('database.bindTableTooltip')}
        helpText={t('database.helpText')}
        emptyText={t('database.emptySelected')}
        isLoading={isDbsLoading}
        onToggleSection={onToggleSection}
        onAdd={() => setDialogOpen(true)}
        readOnly={readOnly}
      >
        <div className="space-y-2">
          {bindings.map(binding => (
            <DatabaseBindingCard
              key={binding.data_source_id}
              dataSourceID={binding.data_source_id}
              dataSourceName={dbsByID.get(binding.data_source_id)?.name}
              tableIDs={binding.table_ids}
              writableTableIDs={binding.writable_table_ids ?? []}
              readOnly={readOnly}
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
        open={dialogOpen}
        bindings={bindings}
        onOpenChange={setDialogOpen}
        onConfirmDatabases={handleConfirmDatabases}
      />
      <AgentRuntimeDatabaseTableDialog
        open={Boolean(tableDialogDbId)}
        dataSourceId={tableDialogDbId}
        dataSourceName={tableDialogDb?.name}
        bindings={bindings}
        canEditWritable={canEditWritable}
        onOpenChange={open => {
          if (!open) handleCloseTableDialog();
        }}
        onConfirm={onChangeBindings}
      />
    </>
  );
}

function DatabaseBindingCard({
  dataSourceID,
  dataSourceName,
  tableIDs,
  writableTableIDs,
  readOnly,
  canEditWritable,
  onOpenTableDialog,
  onRemoveTable,
  onChangeWritableTable,
  onChangeWritableDatabase,
}: {
  dataSourceID: string;
  dataSourceName?: string;
  tableIDs: string[];
  writableTableIDs: string[];
  readOnly: boolean;
  canEditWritable: boolean;
  onOpenTableDialog: (dataSourceID: string) => void;
  onRemoveTable: (dataSourceID: string, tableID: string) => void;
  onChangeWritableTable: (dataSourceID: string, tableID: string, checked: boolean) => void;
  onChangeWritableDatabase: (dataSourceID: string, checked: boolean) => void;
}) {
  const writableSet = useMemo(() => new Set(writableTableIDs), [writableTableIDs]);
  const t = useT('agents.agentRuntime');
  const { tables, isLoading, error } = useDbTables(dataSourceID, { enabled: true });
  const tablesByID = useMemo(() => new Map(tables.map(table => [table.id, table])), [tables]);
  const databaseLabel = dataSourceName || t('database.databaseUnavailable');
  const allWritable = tableIDs.length > 0 && tableIDs.every(tableID => writableSet.has(tableID));

  return (
    <AgentRuntimeResourceCard
      icon={error ? <AlertCircle className="size-4" /> : <Database className="size-4" />}
      title={databaseLabel}
      error={Boolean(error)}
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
              disabled={readOnly}
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
            disabled={!canEditWritable || tableIDs.length === 0}
            onCheckedChange={checked => onChangeWritableDatabase(dataSourceID, checked)}
            aria-label={t('database.allowWriteAllForDatabase', { name: databaseLabel })}
          />
        </span>
      </div>
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
            return (
              <div
                key={tableID}
                className="flex min-h-11 items-center justify-between gap-2 rounded-md bg-muted/35 px-3 py-2"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 items-center gap-2">
                    <Table2 className="size-3.5 shrink-0 text-muted-foreground" />
                    <div className="truncate text-xs font-medium">{label}</div>
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
                    disabled={!canEditWritable}
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
                    disabled={readOnly}
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
