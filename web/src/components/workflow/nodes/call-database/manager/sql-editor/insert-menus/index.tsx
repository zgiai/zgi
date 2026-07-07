'use client';

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { ChevronDown, ChevronRight, Info, Search } from 'lucide-react';
import type { TableRef } from '../../../config';
import { buildColumnToken, formatQualifiedTable } from '../../../utils';
import { useDbTableColumns } from '@/hooks/db/use-db-table-columns';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useDbTables } from '@/hooks/db/use-db-tables';
import { buildTableRef } from '../../../utils';
import { useDatabaseNodePermissions } from '@/components/workflow/hooks';

interface SqlEditorInsertMenusProps {
  dbId?: string;
  tables: TableRef[];
  onInsert: (value: string) => void;
  onColumnsUpdate?: (table: TableRef, columns: string[]) => void;
  className?: string;
  disabled?: boolean;
}

interface ColumnGroupProps {
  dbId?: string;
  table: TableRef;
  forcedOpen: boolean;
  expanded: boolean;
  searchTerm: string;
  onToggle: () => void;
  onInsert: (value: string) => void;
  onColumnsResolved?: (columns: string[]) => void;
  canBrowseDatabaseMetadata: boolean;
}

interface ColumnInfo {
  name: string;
  type?: string;
  description?: string;
}

const ColumnGroup: React.FC<ColumnGroupProps> = ({
  dbId,
  table,
  forcedOpen,
  expanded,
  searchTerm,
  onToggle,
  onInsert,
  onColumnsResolved,
  canBrowseDatabaseMetadata,
}) => {
  const t = useT('nodes');
  const shouldFetch = Boolean(
    canBrowseDatabaseMetadata && dbId && table.id && (forcedOpen || expanded)
  );

  const { columns, isLoading } = useDbTableColumns(dbId ?? '', table.id ?? '', {
    enabled: shouldFetch,
    refetchOnWindowFocus: false,
  });

  const columnItems = useMemo<ColumnInfo[]>(() => {
    if (!columns || columns.length === 0) {
      return [];
    }
    return columns.map(column => ({
      name: column.name,
      type: column.type,
      description: column.description?.trim() || undefined,
    }));
  }, [columns]);

  const columnNames = useMemo<string[]>(() => columnItems.map(item => item.name), [columnItems]);

  useEffect(() => {
    if (columnNames.length > 0 && onColumnsResolved) {
      onColumnsResolved(columnNames);
    }
  }, [columnNames, onColumnsResolved]);

  const normalizedSearch = searchTerm.trim().toLowerCase();
  const filteredColumns = useMemo(() => {
    if (!normalizedSearch) return columnItems;
    return columnItems.filter(item => item.name.toLowerCase().includes(normalizedSearch));
  }, [columnItems, normalizedSearch]);

  if (normalizedSearch && filteredColumns.length === 0) {
    return null;
  }

  const displayColumns = filteredColumns;
  const open = forcedOpen || expanded;

  const tableDisplayName = table.label || formatQualifiedTable(table);

  return (
    <div className="border-b last:border-b-0">
      <button
        type="button"
        className={cn(
          'flex w-full items-center justify-between gap-2 px-2 py-2 text-left text-sm font-medium',
          open ? 'bg-muted/60' : 'hover:bg-muted/40'
        )}
        onClick={onToggle}
        disabled={forcedOpen}
      >
        <span className="truncate">{tableDisplayName}</span>
        {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
      </button>
      {open ? (
        <div className="px-2 pb-2">
          {isLoading ? (
            <div className="space-y-2 pt-2">
              <div className="flex flex-wrap gap-2">
                {Array.from({ length: 12 }).map((_, idx) => (
                  <Skeleton
                    key={idx}
                    className={`h-6 ${idx % 3 === 0 ? 'w-12' : idx % 3 === 1 ? 'w-16' : 'w-20'}`}
                  />
                ))}
              </div>
            </div>
          ) : displayColumns.length === 0 ? (
            <div className="pt-2 text-xs text-muted-foreground">
              {t('callDatabase.empty.noColumns')}
            </div>
          ) : (
            <div className="mt-2 space-y-1">
              {displayColumns.map(column => (
                <button
                  key={column.name}
                  type="button"
                  className="w-full rounded-md px-2 py-1 text-left text-sm hover:bg-muted"
                  onClick={() => onInsert(buildColumnToken(table, column.name))}
                >
                  <div className="flex items-center justify-between gap-3">
                    <span className="truncate">{column.name}</span>
                    <div className="flex shrink-0 items-center gap-2">
                      {column.type ? (
                        <Badge variant="secondary" className="text-[11px] uppercase tracking-wide">
                          {column.type.toUpperCase()}
                        </Badge>
                      ) : null}
                      {column.description ? (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <span
                              className="inline-flex h-5 w-5 items-center justify-center rounded text-muted-foreground hover:text-foreground"
                              onClick={event => event.stopPropagation()}
                              role="button"
                              tabIndex={0}
                            >
                              <Info className="h-3.5 w-3.5" />
                            </span>
                          </TooltipTrigger>
                          <TooltipContent className="max-w-xs text-xs leading-5">
                            {column.description}
                          </TooltipContent>
                        </Tooltip>
                      ) : null}
                    </div>
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>
      ) : null}
    </div>
  );
};

const SqlEditorInsertMenus: React.FC<SqlEditorInsertMenusProps> = ({
  dbId,
  tables,
  onInsert,
  onColumnsUpdate,
  className,
  disabled = false,
}) => {
  const t = useT('nodes');
  const [columnsOpen, setColumnsOpen] = useState(false);
  const [columnsSearch, setColumnsSearch] = useState('');
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const { canReadDatabaseBinding } = useDatabaseNodePermissions();
  const canBrowseDatabaseMetadata = !disabled && canReadDatabaseBinding;

  // Fetch latest tables from API and filter by selected refs from props
  const { tables: apiTables } = useDbTables(dbId ?? '', {
    enabled: Boolean(dbId) && canBrowseDatabaseMetadata,
    refetchOnWindowFocus: false,
    staleTime: 3 * 60 * 1000,
  });

  const menuTables = useMemo<TableRef[]>(() => {
    if (!dbId) return tables;
    if (!apiTables || apiTables.length === 0) return [];
    const selectedKeys = new Set(tables.map(t => `${t.schema}.${t.name}`));
    const refs = apiTables
      .map(t => buildTableRef(t))
      .filter(ref => selectedKeys.has(`${ref.schema}.${ref.name}`));
    return refs;
  }, [apiTables, dbId, tables]);

  // Map of qualified table key to description for tooltip display
  const tableDescMap = useMemo(() => {
    const map = new Map<string, string>();
    if (apiTables && apiTables.length > 0) {
      for (const t of apiTables) {
        const ref = buildTableRef(t);
        const key = `${ref.schema}.${ref.name}`;
        if (t.description && t.description.trim().length > 0) {
          map.set(key, t.description.trim());
        }
      }
    }
    return map;
  }, [apiTables]);

  useEffect(() => {
    if (!columnsOpen) {
      setColumnsSearch('');
      setExpandedKeys(new Set());
    }
  }, [columnsOpen]);

  const sortedTables = useMemo(() => {
    const base = menuTables;
    return [...base].sort((a, b) => {
      const an = a.label || formatQualifiedTable(a);
      const bn = b.label || formatQualifiedTable(b);
      return an.localeCompare(bn);
    });
  }, [menuTables]);

  const disableMenus = !canBrowseDatabaseMetadata;

  const handleToggleExpanded = useCallback((key: string) => {
    setExpandedKeys(prev => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }, []);

  const handleTableInsert = useCallback(
    (tableRef: TableRef) => {
      if (!tableRef.name) return;
      onInsert(formatQualifiedTable(tableRef));
    },
    [onInsert]
  );

  const handleColumnInsert = useCallback(
    (columnName: string) => {
      if (!columnName) return;
      onInsert(columnName);
      setColumnsOpen(false);
    },
    [onInsert]
  );

  const handleColumnsResolved = useCallback(
    (table: TableRef, columns: string[]) => {
      if (!onColumnsUpdate) return;
      onColumnsUpdate(table, columns);
    },
    [onColumnsUpdate]
  );

  const normalizedSearch = columnsSearch.trim();

  return (
    <div className={cn('flex flex-wrap items-center gap-2', className)}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button type="button" variant="outline" size="sm" disabled={disableMenus}>
            {t('callDatabase.actions.insertTableSimple')}
            <ChevronDown className="ml-1 h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-80">
          <DropdownMenuLabel>{t('callDatabase.labels.selectedTables')}</DropdownMenuLabel>
          <DropdownMenuSeparator />
          {sortedTables.length === 0 ? (
            <DropdownMenuItem disabled>{t('callDatabase.empty.noTables')}</DropdownMenuItem>
          ) : (
            sortedTables.map(table => (
              <DropdownMenuItem
                key={`${table.schema}.${table.name}`}
                onSelect={() => handleTableInsert(table)}
              >
                <div className="flex w-full items-center justify-between gap-2">
                  <span className="truncate">{table.label || formatQualifiedTable(table)}</span>
                  {(() => {
                    const desc = tableDescMap.get(`${table.schema}.${table.name}`);
                    if (!desc) return null;
                    return (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className="inline-flex">
                            <Info className="h-4 w-4 shrink-0 text-muted-foreground" />
                          </span>
                        </TooltipTrigger>
                        <TooltipContent side="top" align="center">
                          <div className="max-w-[280px] text-xs">{desc}</div>
                        </TooltipContent>
                      </Tooltip>
                    );
                  })()}
                </div>
              </DropdownMenuItem>
            ))
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <DropdownMenu open={columnsOpen} onOpenChange={setColumnsOpen}>
        <DropdownMenuTrigger asChild>
          <Button type="button" variant="outline" size="sm" disabled={disableMenus}>
            {t('callDatabase.actions.insertColumnSimple')}
            <ChevronDown className="ml-1 h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-[400px] p-0">
          <div className="flex items-center gap-2 border-b px-3 py-2">
            <Search className="h-4 w-4 text-muted-foreground" />
            <Input
              placeholder={t('callDatabase.placeholders.searchColumns')}
              value={columnsSearch}
              onChange={event => setColumnsSearch(event.target.value)}
              className="h-8"
            />
          </div>
          <ScrollArea className="h-72">
            {sortedTables.length === 0 ? (
              <div className="px-3 py-4 text-sm text-muted-foreground">
                {t('callDatabase.empty.noTables')}
              </div>
            ) : (
              <div>
                {sortedTables.map(table => {
                  const key = `${table.schema}.${table.name}`;
                  const expanded = expandedKeys.has(key);
                  return (
                    <ColumnGroup
                      key={key}
                      dbId={dbId}
                      table={table}
                      forcedOpen={Boolean(normalizedSearch)}
                      expanded={expanded}
                      searchTerm={columnsSearch}
                      onToggle={() => handleToggleExpanded(key)}
                      onInsert={handleColumnInsert}
                      onColumnsResolved={columns => handleColumnsResolved(table, columns)}
                      canBrowseDatabaseMetadata={canBrowseDatabaseMetadata}
                    />
                  );
                })}
              </div>
            )}
          </ScrollArea>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
};

export default SqlEditorInsertMenus;
