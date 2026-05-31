'use client';

import { useEffect, useMemo, useState } from 'react';
import { Check, Search, Table2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { useDbTables } from '@/hooks/db/use-db-tables';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { AgentDatabaseBinding } from '@/services/types/agent';
import type { DbTable } from '@/services/types/db';

interface AgentRuntimeDatabaseTableDialogProps {
  open: boolean;
  dataSourceId: string;
  dataSourceName?: string;
  bindings: AgentDatabaseBinding[];
  canEditWritable: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (value: AgentDatabaseBinding[]) => void;
}

export function AgentRuntimeDatabaseTableDialog({
  open,
  dataSourceId,
  dataSourceName,
  bindings,
  canEditWritable,
  onOpenChange,
  onConfirm,
}: AgentRuntimeDatabaseTableDialogProps) {
  const t = useT('agents.agentRuntime');
  const [localBindings, setLocalBindings] = useState<AgentDatabaseBinding[]>(bindings);
  const [tableSearch, setTableSearch] = useState('');
  const { tables, isLoading } = useDbTables(dataSourceId, {
    enabled: open && Boolean(dataSourceId),
  });

  useEffect(() => {
    if (!open) return;
    setLocalBindings(normalizeBindings(bindings));
    setTableSearch('');
  }, [bindings, open]);

  const selectedTableIDs = useMemo(
    () =>
      new Set(
        localBindings.find(binding => binding.data_source_id === dataSourceId)?.table_ids ?? []
      ),
    [dataSourceId, localBindings]
  );
  const writableTableIDs = useMemo(
    () =>
      new Set(
        localBindings.find(binding => binding.data_source_id === dataSourceId)
          ?.writable_table_ids ?? []
      ),
    [dataSourceId, localBindings]
  );
  const selectedDbTableIDs = useMemo(
    () => tables.map(table => table.id).filter(Boolean),
    [tables]
  );
  const selectedDbCheckedTableIDs = useMemo(
    () => selectedDbTableIDs.filter(tableID => selectedTableIDs.has(tableID)),
    [selectedDbTableIDs, selectedTableIDs]
  );
  const allSelectedDbTablesChecked =
    selectedDbTableIDs.length > 0 &&
    selectedDbTableIDs.every(tableID => selectedTableIDs.has(tableID));
  const allSelectedDbCheckedTablesWritable =
    selectedDbCheckedTableIDs.length > 0 &&
    selectedDbCheckedTableIDs.every(tableID => writableTableIDs.has(tableID));

  const filteredTables = useMemo(() => {
    const keyword = tableSearch.trim().toLowerCase();
    if (!keyword) return tables;
    return tables.filter(table =>
      [table.name, table.table_name, table.description]
        .filter(Boolean)
        .some(value => String(value).toLowerCase().includes(keyword))
    );
  }, [tableSearch, tables]);

  const handleToggleTable = (tableID: string, checked: boolean) => {
    if (!dataSourceId) return;
    const next = bindingsToMap(localBindings);
    const selected = next.get(dataSourceId) ?? {
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
      next.set(dataSourceId, selected);
    } else {
      next.delete(dataSourceId);
    }
    setLocalBindings(bindingsFromMap(next));
  };

  const handleToggleAllTables = (checked: boolean) => {
    if (!dataSourceId || selectedDbTableIDs.length === 0) return;
    const next = bindingsToMap(localBindings);
    const selected = next.get(dataSourceId) ?? {
      readable: new Set<string>(),
      writable: new Set<string>(),
    };
    selectedDbTableIDs.forEach(tableID => {
      if (checked) {
        selected.readable.add(tableID);
      } else {
        selected.readable.delete(tableID);
        selected.writable.delete(tableID);
      }
    });
    if (selected.readable.size > 0) {
      next.set(dataSourceId, selected);
    } else {
      next.delete(dataSourceId);
    }
    setLocalBindings(bindingsFromMap(next));
  };

  const handleToggleAllWritable = (checked: boolean) => {
    if (!canEditWritable || !dataSourceId || selectedDbCheckedTableIDs.length === 0) return;
    const next = bindingsToMap(localBindings);
    const selected = next.get(dataSourceId);
    if (!selected) return;
    selectedDbCheckedTableIDs.forEach(tableID => {
      if (checked) {
        selected.writable.add(tableID);
      } else {
        selected.writable.delete(tableID);
      }
    });
    next.set(dataSourceId, selected);
    setLocalBindings(bindingsFromMap(next));
  };

  const handleToggleWritable = (tableID: string, checked: boolean) => {
    if (!canEditWritable || !dataSourceId) return;
    const next = bindingsToMap(localBindings);
    const selected = next.get(dataSourceId);
    if (!selected || !selected.readable.has(tableID)) return;
    if (checked) {
      selected.writable.add(tableID);
    } else {
      selected.writable.delete(tableID);
    }
    next.set(dataSourceId, selected);
    setLocalBindings(bindingsFromMap(next));
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{dataSourceName || t('database.databaseUnavailable')}</DialogTitle>
          <DialogDescription>{t('database.tableDialogDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-[560px]">
          <div className="space-y-3">
            <Input
              value={tableSearch}
              onChange={event => setTableSearch(event.target.value)}
              placeholder={t('database.searchTable')}
              leftIcon={<Search className="size-4" />}
            />
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-8 px-2.5"
                disabled={isLoading || selectedDbTableIDs.length === 0}
                onClick={() => handleToggleAllTables(!allSelectedDbTablesChecked)}
              >
                {allSelectedDbTablesChecked
                  ? t('database.clearSelectedDatabaseTables')
                  : t('database.selectAllDatabaseTables')}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-8 px-2.5"
                disabled={
                  !canEditWritable || isLoading || selectedDbCheckedTableIDs.length === 0
                }
                onClick={() => handleToggleAllWritable(!allSelectedDbCheckedTablesWritable)}
              >
                {allSelectedDbCheckedTablesWritable
                  ? t('database.clearWritableTables')
                  : t('database.makeSelectedTablesWritable')}
              </Button>
            </div>
            {!canEditWritable ? (
              <div className="rounded-md border border-dashed bg-muted/20 p-3 text-xs text-muted-foreground">
                {t('database.writePermissionRequired')}
              </div>
            ) : null}
            {isLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 6 }).map((_, index) => (
                  <Skeleton key={index} className="h-16 w-full" />
                ))}
              </div>
            ) : filteredTables.length === 0 ? (
              <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                {t('database.noTables')}
              </div>
            ) : (
              <div className="space-y-2">
                {filteredTables.map(table => {
                  const checked = selectedTableIDs.has(table.id);
                  const writable = writableTableIDs.has(table.id);
                  return (
                    <button
                      key={table.id}
                      type="button"
                      className={cn(
                        'flex w-full items-start gap-3 rounded-md border bg-background p-3 text-left transition-colors hover:border-primary/50 hover:bg-muted/30',
                        checked && 'border-primary bg-primary/5'
                      )}
                      onClick={() => handleToggleTable(table.id, !checked)}
                    >
                      <Checkbox
                        checked={checked}
                        onCheckedChange={value => handleToggleTable(table.id, value === true)}
                        onClick={event => event.stopPropagation()}
                        className="mt-0.5"
                      />
                      <span className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted text-primary">
                        <Table2 className="size-4" />
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">
                          {tableLabel(table, t('database.unnamedTable'))}
                        </span>
                        {table.description || table.table_name ? (
                          <span className="mt-1 block truncate text-xs text-muted-foreground">
                            {table.description || table.table_name}
                          </span>
                        ) : null}
                        {checked ? (
                          <span
                            className="mt-2 flex items-center gap-2 text-xs text-muted-foreground"
                            onClick={event => event.stopPropagation()}
                          >
                            <Switch
                              checked={writable}
                              disabled={!canEditWritable}
                              onCheckedChange={value =>
                                handleToggleWritable(table.id, value === true)
                              }
                              aria-label={t('database.allowWriteForTable', {
                                name: tableLabel(table, t('database.unnamedTable')),
                              })}
                            />
                            <span>{t('database.allowWrite')}</span>
                          </span>
                        ) : null}
                      </span>
                      {checked ? (
                        <span className="mt-1 flex size-5 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground">
                          <Check className="size-3.5" />
                        </span>
                      ) : null}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {t('database.cancel')}
          </Button>
          <Button
            type="button"
            onClick={() => {
              onConfirm(normalizeBindings(localBindings));
              onOpenChange(false);
            }}
          >
            {t('database.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function tableLabel(table: DbTable, fallback: string) {
  return table.name || table.table_name || fallback;
}

function normalizeBindings(input: AgentDatabaseBinding[]): AgentDatabaseBinding[] {
  const next = new Map<string, { readable: Set<string>; writable: Set<string> }>();
  input.forEach(binding => {
    const dbID = binding.data_source_id.trim();
    if (!dbID) return;
    const tableIDs = next.get(dbID) ?? {
      readable: new Set<string>(),
      writable: new Set<string>(),
    };
    binding.table_ids.forEach(tableID => {
      const normalized = tableID.trim();
      if (normalized) {
        tableIDs.readable.add(normalized);
      }
    });
    (binding.writable_table_ids ?? []).forEach(tableID => {
      const normalized = tableID.trim();
      if (normalized && tableIDs.readable.has(normalized)) {
        tableIDs.writable.add(normalized);
      }
    });
    if (tableIDs.readable.size > 0) {
      next.set(dbID, tableIDs);
    }
  });
  return bindingsFromMap(next);
}

function bindingsToMap(bindings: AgentDatabaseBinding[]) {
  const next = new Map<string, { readable: Set<string>; writable: Set<string> }>();
  normalizeBindings(bindings).forEach(binding => {
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
