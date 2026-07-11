'use client';

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { ChevronDown, ChevronRight } from 'lucide-react';
import type { DatabaseSourceRef, TableRef } from '../../nodes/call-database/config';
import type { Db, DbTable } from '@/services/types/db';
import { useDbsBasic } from '@/hooks/db/use-dbs';
import { useDbTables } from '@/hooks/db/use-db-tables';
import { useDbTableColumns } from '@/hooks/db/use-db-table-columns';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import {
  buildTableRef,
  createDatabaseSourceRef,
  extractColumnNames,
  mergeTableSelection,
  removeTableSelection,
  inferDatabaseType,
} from '../../nodes/call-database/utils';
import { useDatabaseNodePermissions } from '../../hooks';

interface PickerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: { dataSource: DatabaseSourceRef | null; tables: TableRef[] };
  onConfirm: (next: { dataSource: DatabaseSourceRef; tables: TableRef[] }) => void;
  initialSchema?: string;
  readOnly?: boolean;
}

interface TableListProps {
  dbId: string;
  fallbackSchema: string;
  selected: TableRef[];
  onChange: (next: TableRef[]) => void;
  canBrowseDatabaseMetadata: boolean;
}

interface TableItemProps {
  dbId: string;
  table: DbTable;
  selected: boolean;
  onToggle: (checked: boolean, table: DbTable) => void;
  canBrowseDatabaseMetadata: boolean;
}

const TABLE_SKELETONS = Array.from({ length: 6 });

const TableItem: React.FC<TableItemProps> = ({
  dbId,
  table,
  selected,
  onToggle,
  canBrowseDatabaseMetadata,
}) => {
  const t = useT('nodes');
  const [expanded, setExpanded] = useState(false);
  const { columns, isLoading } = useDbTableColumns(dbId, table.id, {
    enabled: expanded && canBrowseDatabaseMetadata,
    refetchOnWindowFocus: false,
  });

  const columnNames = useMemo(() => extractColumnNames(columns), [columns]);

  const displayName = table.name || table.table_name || table.id;
  const description = table.description;

  return (
    <div
      className={cn(
        'rounded-xl border transition-all duration-200 w-full overflow-hidden group',
        selected
          ? 'border-primary bg-primary/5 shadow-sm'
          : 'border-neutral-100 bg-white hover:border-neutral-200 hover:bg-neutral-50/50'
      )}
    >
      <div
        className="p-4 flex items-start gap-4 cursor-pointer"
        onClick={() => setExpanded(prev => !prev)}
      >
        <div className="pt-0.5" onClick={e => e.stopPropagation()}>
          <Checkbox
            checked={selected}
            onCheckedChange={checked => onToggle(Boolean(checked), table)}
            className={cn(
              'w-5 h-5 transition-all',
              selected ? 'bg-primary text-white border-primary shadow-sm' : 'border-neutral-300'
            )}
          />
        </div>

        <div className="space-y-1.5 flex-1 min-w-0">
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0">
              <div
                className={cn(
                  'h-7 w-7 flex items-center justify-center rounded-lg transition-colors shrink-0',
                  expanded
                    ? 'bg-primary/10 text-primary'
                    : 'bg-neutral-100 text-neutral-400 group-hover:bg-neutral-200 group-hover:text-neutral-500'
                )}
              >
                {expanded ? (
                  <ChevronDown className="h-4 w-4" />
                ) : (
                  <ChevronRight className="h-4 w-4" />
                )}
              </div>
              <div
                className={cn(
                  'truncate text-sm font-bold transition-colors',
                  selected ? 'text-primary' : 'text-neutral-900'
                )}
                title={displayName}
              >
                {displayName}
              </div>
            </div>
            {selected && (
              <Badge
                variant="default"
                className="bg-primary hover:bg-primary h-5 px-1.5 text-[9px] font-black uppercase tracking-tighter rounded-md shadow-sm animate-in zoom-in-50"
              >
                Selected
              </Badge>
            )}
          </div>

          {description && (
            <div
              className="truncate text-xs text-muted-foreground font-medium pl-9"
              title={description}
            >
              {description}
            </div>
          )}

          {expanded && (
            <div className="mt-4 pl-9 space-y-3 animate-in fade-in slide-in-from-top-1">
              <div className="flex items-center gap-1.5 opacity-60">
                <div className="h-0.5 w-3 bg-neutral-300 rounded-full" />
                <span className="text-[10px] font-bold uppercase tracking-widest text-neutral-400">
                  Columns
                </span>
              </div>

              {isLoading ? (
                <div className="flex flex-wrap gap-2">
                  {Array.from({ length: 8 }).map((_, idx) => (
                    <Skeleton
                      key={idx}
                      className={cn(
                        'h-6 rounded-lg',
                        idx % 3 === 0 ? 'w-16' : idx % 3 === 1 ? 'w-20' : 'w-12'
                      )}
                    />
                  ))}
                </div>
              ) : columnNames.length === 0 ? (
                <div className="text-xs text-neutral-400 font-medium italic">
                  {t('callDatabase.empty.noColumns')}
                </div>
              ) : (
                <div className="flex flex-wrap gap-1.5">
                  {columnNames.map(name => (
                    <Badge
                      key={name}
                      variant="secondary"
                      className="text-[10px] font-bold px-2 py-0.5 rounded-lg bg-white border-neutral-100 shadow-sm text-neutral-600"
                    >
                      {name}
                    </Badge>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

const TableList: React.FC<TableListProps> = ({
  dbId,
  fallbackSchema,
  selected,
  onChange,
  canBrowseDatabaseMetadata,
}) => {
  const t = useT('nodes');
  const [keyword, setKeyword] = useState('');
  const debouncedKeyword = useDebouncedValue(keyword, 200);

  const { tables, isLoading } = useDbTables(dbId, {
    enabled: Boolean(dbId) && canBrowseDatabaseMetadata,
    refetchOnWindowFocus: false,
  });

  const filtered = useMemo(() => {
    if (!debouncedKeyword.trim()) return tables;
    const lower = debouncedKeyword.trim().toLowerCase();
    return tables.filter(table => {
      const bucket = [table.name, table.table_name, table.description?.toString() ?? ''].filter(
        Boolean
      ) as string[];
      return bucket.some(item => item.toLowerCase().includes(lower));
    });
  }, [debouncedKeyword, tables]);

  const handleToggle = useCallback(
    (checked: boolean, table: DbTable) => {
      const baseRef = buildTableRef(table, undefined, fallbackSchema);
      if (!checked) {
        onChange(removeTableSelection(selected, baseRef));
        return;
      }
      onChange(mergeTableSelection(selected, baseRef));
    },
    [fallbackSchema, onChange, selected]
  );

  // No-op: columns are not stored in node data; backend will handle schema

  return (
    <div className="space-y-4 w-full flex flex-col h-full animate-in fade-in slide-in-from-right-2">
      <div className="flex items-center gap-2 px-1">
        <div className="h-4 w-1 bg-primary rounded-full" />
        <Label
          htmlFor="table-search"
          className="text-sm font-bold uppercase tracking-wider text-primary/80"
        >
          {t('callDatabase.fields.tables')}
        </Label>
      </div>

      <Input
        id="table-search"
        placeholder={t('callDatabase.fields.searchTables')}
        className="h-10 shadow-sm font-medium bg-neutral-50/50 border-neutral-200"
        value={keyword}
        onChange={event => setKeyword(event.target.value)}
      />

      <div className="grow bg-neutral-50/30 rounded-2xl border border-neutral-100 shadow-sm overflow-hidden flex flex-col">
        <div className="grow p-3 overflow-y-auto scrollbar-thin">
          {isLoading ? (
            <div className="space-y-3 h-full">
              {TABLE_SKELETONS.map((_, idx) => (
                <Skeleton key={idx} className="h-16 w-full rounded-xl" />
              ))}
            </div>
          ) : filtered.length === 0 ? (
            <div className="py-12 text-center text-sm text-neutral-400 font-medium italic">
              {t('callDatabase.empty.noTables')}
            </div>
          ) : (
            <div className="space-y-3">
              {filtered.map(table => {
                const baseRef = buildTableRef(table, undefined, fallbackSchema);
                const isSelected = selected.some(
                  item => item.schema === baseRef.schema && item.name === baseRef.name
                );
                return (
                  <TableItem
                    key={table.id}
                    dbId={dbId}
                    table={table}
                    selected={isSelected}
                    onToggle={handleToggle}
                    canBrowseDatabaseMetadata={canBrowseDatabaseMetadata}
                  />
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

const PickerDialog: React.FC<PickerDialogProps> = ({
  open,
  onOpenChange,
  value,
  onConfirm,
  initialSchema,
  readOnly = false,
}) => {
  const t = useT('nodes');
  const [keyword, setKeyword] = useState('');
  const debouncedKeyword = useDebouncedValue(keyword, 300);
  const [localSource, setLocalSource] = useState<DatabaseSourceRef | null>(value.dataSource);
  const [tableSelection, setTableSelection] = useState<TableRef[]>(value.tables);
  const { canReadDatabaseBinding } = useDatabaseNodePermissions();
  const canBrowseDatabaseMetadata = !readOnly && canReadDatabaseBinding;

  useEffect(() => {
    if (!open) return;
    setKeyword('');
    setLocalSource(value.dataSource);
    setTableSelection(value.tables);
  }, [open, value.dataSource, value.tables]);

  const { dbs, isLoading } = useDbsBasic(
    { keyword: debouncedKeyword || undefined },
    { enabled: open && canBrowseDatabaseMetadata, refetchOnWindowFocus: false }
  );

  const filteredDbs = useMemo(() => {
    if (!debouncedKeyword.trim()) return dbs;
    const lower = debouncedKeyword.trim().toLowerCase();
    return dbs.filter(db => {
      return (
        db.name.toLowerCase().includes(lower) ||
        (db.description ?? '').toLowerCase().includes(lower) ||
        (db.provider ?? '').toLowerCase().includes(lower)
      );
    });
  }, [dbs, debouncedKeyword]);

  const fallbackSchema = useMemo(() => {
    if (localSource?.schema_name && localSource.schema_name.trim()) {
      return localSource.schema_name.trim();
    }
    if (initialSchema && initialSchema.trim()) {
      return initialSchema.trim();
    }
    return 'public';
  }, [initialSchema, localSource?.schema_name]);

  const handleDbSelect = useCallback(
    (db: Db) => {
      const ref = createDatabaseSourceRef(db);
      if (!ref || !canBrowseDatabaseMetadata) return;
      setLocalSource(ref);
      setTableSelection([]);
    },
    [canBrowseDatabaseMetadata]
  );

  const handleConfirm = useCallback(() => {
    if (!localSource || !canBrowseDatabaseMetadata) return;
    onConfirm({ dataSource: localSource, tables: tableSelection });
    onOpenChange(false);
  }, [canBrowseDatabaseMetadata, localSource, onConfirm, onOpenChange, tableSelection]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-5xl w-full p-0 overflow-hidden" aria-description={undefined}>
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('callDatabase.section.tables')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="p-0">
          <div className="flex w-full h-[540px]">
            {/* Left Panel: Data Sources */}
            <div className="flex-1 min-w-0 border-r border-neutral-100 flex flex-col">
              <div className="p-5 space-y-4 flex flex-col h-full bg-neutral-50/30">
                <div className="flex items-center gap-2 px-1">
                  <div className="h-4 w-1 bg-primary rounded-full" />
                  <Label className="text-sm font-bold uppercase tracking-wider text-primary/80">
                    {t('callDatabase.fields.dataSource')}
                  </Label>
                </div>

                <Input
                  placeholder={t('callDatabase.fields.selectDataSource')}
                  className="h-10 shadow-sm font-medium bg-white"
                  value={keyword}
                  onChange={event => setKeyword(event.target.value)}
                  disabled={!canBrowseDatabaseMetadata}
                />

                <div className="grow bg-white rounded-2xl border border-neutral-100 shadow-sm overflow-hidden flex flex-col">
                  <div className="p-3 overflow-y-auto grow scrollbar-thin">
                    {isLoading ? (
                      <div className="space-y-3 h-full">
                        {TABLE_SKELETONS.map((_, idx) => (
                          <Skeleton key={idx} className="h-16 w-full rounded-xl" />
                        ))}
                      </div>
                    ) : filteredDbs.length === 0 ? (
                      <div className="py-12 text-center text-sm text-neutral-400 font-medium">
                        {t('callDatabase.empty.noDataSource')}
                      </div>
                    ) : (
                      <div className="space-y-3">
                        {filteredDbs.map(db => {
                          const selected = localSource?.id === db.id;
                          const type = inferDatabaseType(db);
                          return (
                            <button
                              key={db.id}
                              type="button"
                              className={cn(
                                'w-full rounded-xl border p-4 text-left transition-all duration-200 group relative',
                                selected
                                  ? 'border-primary bg-primary/5 shadow-premium-sm'
                                  : 'border-neutral-100 hover:border-neutral-200 hover:bg-neutral-50/50',
                                !canBrowseDatabaseMetadata && 'pointer-events-none opacity-60'
                              )}
                              onClick={() => handleDbSelect(db)}
                            >
                              {selected && (
                                <div className="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-6 bg-primary rounded-r-full" />
                              )}
                              <div className="flex items-center justify-between gap-4">
                                <div className="min-w-0 flex-1">
                                  <div
                                    className={cn(
                                      'truncate text-sm font-bold transition-colors',
                                      selected ? 'text-primary' : 'text-neutral-900'
                                    )}
                                  >
                                    {db.name}
                                  </div>
                                  {db.description && (
                                    <div className="truncate text-xs text-muted-foreground font-medium mt-0.5">
                                      {db.description}
                                    </div>
                                  )}
                                </div>
                                <Badge
                                  variant="secondary"
                                  className={cn(
                                    'shrink-0 uppercase text-[10px] font-bold px-2 py-0.5 rounded-lg',
                                    selected
                                      ? 'bg-primary/10 text-primary border-primary/20'
                                      : 'bg-neutral-100 border-neutral-200'
                                  )}
                                >
                                  {type}
                                </Badge>
                              </div>
                            </button>
                          );
                        })}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </div>

            {/* Right Panel: Tables */}
            <div className="flex-1 min-w-0 flex flex-col">
              <div className="p-5 h-full flex flex-col bg-white">
                {localSource ? (
                  <TableList
                    dbId={localSource.id}
                    fallbackSchema={fallbackSchema}
                    selected={tableSelection}
                    onChange={setTableSelection}
                    canBrowseDatabaseMetadata={canBrowseDatabaseMetadata}
                  />
                ) : (
                  <div className="flex-1 flex flex-col items-center justify-center rounded-3xl border-2 border-dashed border-neutral-100 bg-neutral-50/30 p-12 text-center animate-in fade-in zoom-in-95 duration-300">
                    <div className="h-16 w-16 bg-neutral-100 flex items-center justify-center rounded-2xl mb-4 group-hover:scale-110 transition-transform">
                      <ChevronRight className="h-8 w-8 text-neutral-400 rotate-90" />
                    </div>
                    <div className="max-w-[200px] text-sm font-bold text-neutral-400 leading-relaxed">
                      {t('callDatabase.empty.selectDataSourceFirst')}
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t font-medium">
          <Button
            type="button"
            variant="ghost"
            className="font-semibold"
            onClick={() => onOpenChange(false)}
          >
            {t('callDatabase.actions.cancel')}
          </Button>
          <Button
            type="button"
            onClick={handleConfirm}
            size="lg"
            className="px-10 font-bold shadow-sm"
            disabled={!localSource || !canBrowseDatabaseMetadata}
          >
            {t('callDatabase.actions.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default PickerDialog;
