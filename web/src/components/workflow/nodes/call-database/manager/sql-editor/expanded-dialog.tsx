'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { Label } from '@/components/ui/label';
import { ChevronLeft, Info } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { TableRef } from '../../config';
import { buildColumnToken, formatQualifiedTable } from '../../utils';
import { useDbTables } from '@/hooks/db/use-db-tables';
import { useDbTableColumns } from '@/hooks/db/use-db-table-columns';
import { buildTableRef } from '../../utils';
import WorkflowValueInserter from '@/components/workflow/common/workflow-value-inserter';
import type { WorkflowVariable } from '@/components/workflow/store/type';
import { useDatabaseNodePermissions } from '@/components/workflow/hooks';
import SqlMonacoEditor, { type SqlMonacoEditorHandle } from './sql-monaco-editor';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import type { DbTable } from '@/services/types/db';

interface ExpandedSqlEditorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dbId?: string;
  tables: TableRef[];
  sql: string;
  onChangeSql: (next: string) => void;
  nodeId: string;
  readOnly?: boolean;
}

interface ColumnInfo {
  name: string;
  type?: string;
  description?: string;
}

interface ColumnBadgesProps {
  dbId?: string;
  table: TableRef;
  expanded: boolean;
  onInsertColumn: (name: string) => void;
  canBrowseDatabaseMetadata: boolean;
}

const ColumnBadges: React.FC<ColumnBadgesProps> = ({
  dbId,
  table,
  expanded,
  onInsertColumn,
  canBrowseDatabaseMetadata,
}) => {
  const t = useT('nodes');
  const shouldFetch = Boolean(canBrowseDatabaseMetadata && dbId && table.id && expanded);
  const { columns, isLoading } = useDbTableColumns(dbId ?? '', table.id ?? '', {
    enabled: shouldFetch,
    refetchOnWindowFocus: false,
  });

  const items = useMemo<ColumnInfo[]>(() => {
    if (!columns || columns.length === 0) return [];
    return columns.map(c => ({
      name: c.name,
      type: c.type,
      description: c.description ?? undefined,
    }));
  }, [columns]);

  return (
    <div className="mt-2">
      {isLoading ? (
        <div className="flex flex-wrap gap-2">
          {Array.from({ length: 12 }).map((_, idx) => (
            <Skeleton
              key={idx}
              className={`h-6 ${idx % 3 === 0 ? 'w-12' : idx % 3 === 1 ? 'w-16' : 'w-20'}`}
            />
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="text-xs text-muted-foreground">{t('callDatabase.empty.noColumns')}</div>
      ) : (
        <div className="flex flex-wrap gap-2">
          {items.map(col =>
            col.description ? (
              <Tooltip key={col.name}>
                <TooltipTrigger asChild>
                  <Badge
                    variant="secondary"
                    className="text-[11px] px-2 py-1 cursor-pointer hover:bg-accent"
                    onClick={e => {
                      e.stopPropagation();
                      onInsertColumn(col.name);
                    }}
                  >
                    {col.name}
                  </Badge>
                </TooltipTrigger>
                <TooltipContent side="top" align="start">
                  <div className="max-w-[280px] text-xs">{col.description}</div>
                </TooltipContent>
              </Tooltip>
            ) : (
              <Badge
                key={col.name}
                variant="secondary"
                className="text-[11px] px-2 py-1 cursor-pointer hover:bg-accent"
                onClick={e => {
                  e.stopPropagation();
                  onInsertColumn(col.name);
                }}
              >
                {col.name}
              </Badge>
            )
          )}
        </div>
      )}
    </div>
  );
};

const ExpandedSqlEditorDialog: React.FC<ExpandedSqlEditorDialogProps> = ({
  open,
  onOpenChange,
  dbId,
  tables,
  sql,
  onChangeSql,
  nodeId,
  readOnly = false,
}) => {
  const t = useT('nodes');
  const editorRef = useRef<SqlMonacoEditorHandle | null>(null);
  const [search, setSearch] = useState('');
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set());
  const { canReadDatabaseBinding } = useDatabaseNodePermissions();

  // Fetch latest tables and filter to current selection
  const { tables: apiTables, isLoading } = useDbTables(dbId ?? '', {
    enabled: Boolean(dbId && open && canReadDatabaseBinding),
    refetchOnWindowFocus: false,
  });

  const selectedKeys = useMemo(() => new Set(tables.map(t => `${t.schema}.${t.name}`)), [tables]);

  const menuTables = useMemo<TableRef[]>(() => {
    if (!apiTables || apiTables.length === 0) return [];
    const refs = apiTables
      .map(t => buildTableRef(t))
      .filter(ref => selectedKeys.has(`${ref.schema}.${ref.name}`));
    if (!search.trim()) return refs;
    const lower = search.trim().toLowerCase();
    return refs.filter(ref => {
      const label = ref.label || formatQualifiedTable(ref);
      return label.toLowerCase().includes(lower);
    });
  }, [apiTables, search, selectedKeys]);

  // Build description map from API tables for tooltip display
  const tableDescMap = useMemo(() => {
    const map = new Map<string, string>();
    (apiTables ?? []).forEach((t: DbTable) => {
      const ref = buildTableRef(t);
      const key = `${ref.schema}.${ref.name}`;
      const desc = String(t.description ?? '').trim();
      if (desc) map.set(key, desc);
    });
    return map;
  }, [apiTables]);

  const insertText = useCallback(
    (text: string) => {
      if (!text.trim() || readOnly) return;
      const editor = editorRef.current;
      if (editor) {
        editor.insertText(text);
      } else {
        const next = sql.length > 0 && !sql.endsWith(' ') ? `${sql} ${text}` : `${sql}${text}`;
        onChangeSql(next);
      }
    },
    [onChangeSql, readOnly, sql]
  );

  const handleInsertTable = useCallback(
    (ref: TableRef) => {
      insertText(formatQualifiedTable(ref));
    },
    [insertText]
  );

  const handleInsertColumn = useCallback(
    (ref: TableRef, name: string) => {
      insertText(buildColumnToken(ref, name));
    },
    [insertText]
  );

  const handleToggle = useCallback((key: string) => {
    setExpandedKeys(prev => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  useEffect(() => {
    if (!open) {
      setSearch('');
      setExpandedKeys(new Set());
    }
  }, [open]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-description={undefined} className="max-w-6xl w-full p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('callDatabase.section.sqlEditor')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="p-0">
          <div className="w-full h-[75vh] flex flex-col">
            <div className="h-0 grow p-5 flex flex-col gap-5 overflow-y-auto scrollbar-thin">
              <div className="bg-neutral-50/50 p-4 rounded-2xl border border-neutral-100 shadow-sm">
                <WorkflowValueInserter
                  nodeId={nodeId}
                  className="w-full"
                  disabled={readOnly}
                  onInsert={(value: {
                    sourceId: string;
                    key: string;
                    type: WorkflowVariable['type'];
                  }) => {
                    const token = value.key
                      ? `{{#${value.sourceId}.${value.key}#}}`
                      : `{{#${value.sourceId}#}}`;
                    insertText(token);
                  }}
                />
              </div>

              <div className="space-y-3 grow flex flex-col px-1">
                <div className="flex items-center gap-2">
                  <div className="h-4 w-1 bg-primary rounded-full" />
                  <Label
                    htmlFor="table-filter"
                    className="text-sm font-bold uppercase tracking-wider text-primary/80"
                  >
                    {t('callDatabase.fields.tables')}
                  </Label>
                </div>

                <div className="grow bg-white rounded-2xl border border-neutral-100 shadow-sm overflow-hidden flex flex-col">
                  <div className="p-4 overflow-y-auto grow scrollbar-thin">
                    {isLoading ? (
                      <div className="space-y-3">
                        {Array.from({ length: 6 }).map((_, idx) => (
                          <Skeleton key={idx} className="h-16 w-full rounded-xl" />
                        ))}
                      </div>
                    ) : menuTables.length === 0 ? (
                      <div className="py-12 text-center text-sm text-neutral-400 font-medium">
                        {t('callDatabase.empty.noTables')}
                      </div>
                    ) : (
                      <div className="space-y-3">
                        {menuTables.map(ref => {
                          const key = `${ref.schema}.${ref.name}`;
                          const openRow = expandedKeys.has(key);
                          return (
                            <div
                              key={key}
                              className={cn(
                                'rounded-xl border transition-all duration-200',
                                openRow
                                  ? 'border-primary/20 bg-primary/5 shadow-sm'
                                  : 'border-neutral-100 hover:border-neutral-200 hover:bg-neutral-50/50'
                              )}
                            >
                              <div
                                className="p-3 cursor-pointer flex items-center gap-2"
                                onClick={() => handleToggle(key)}
                              >
                                <div
                                  className="truncate font-bold text-sm max-w-[240px]"
                                  title={ref.label || formatQualifiedTable(ref)}
                                >
                                  {ref.label || formatQualifiedTable(ref)}
                                </div>
                                {(() => {
                                  const desc = tableDescMap.get(key);
                                  if (!desc) return null;
                                  return (
                                    <Tooltip>
                                      <TooltipTrigger asChild>
                                        <span
                                          className="inline-flex h-6 w-6 items-center justify-center text-muted-foreground hover:text-primary transition-colors"
                                          onClick={e => e.stopPropagation()}
                                        >
                                          <Info className="h-4 w-4" />
                                        </span>
                                      </TooltipTrigger>
                                      <TooltipContent
                                        side="top"
                                        align="start"
                                        className="shadow-premium max-w-[320px] p-3 leading-relaxed"
                                      >
                                        <div className="text-xs font-medium">{desc}</div>
                                      </TooltipContent>
                                    </Tooltip>
                                  );
                                })()}

                                <div className="ml-auto flex items-center gap-2">
                                  <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    disabled={readOnly}
                                    className="h-7 px-3 text-[11px] font-bold rounded-lg hover:bg-primary hover:text-white transition-all"
                                    onClick={e => {
                                      e.stopPropagation();
                                      handleInsertTable(ref);
                                    }}
                                  >
                                    {t('callDatabase.actions.insertTableSimple')}
                                  </Button>
                                  <div className="h-7 w-7 flex items-center justify-center rounded-lg hover:bg-neutral-200/50 transition-colors">
                                    <ChevronLeft
                                      className={cn(
                                        'h-4 w-4 transition-transform duration-200',
                                        openRow && '-rotate-90'
                                      )}
                                    />
                                  </div>
                                </div>
                              </div>
                              {openRow ? (
                                <div className="px-3 pb-3 pt-1 animate-in fade-in slide-in-from-top-1">
                                  <ColumnBadges
                                    dbId={dbId}
                                    table={ref}
                                    expanded={openRow}
                                    onInsertColumn={name => handleInsertColumn(ref, name)}
                                    canBrowseDatabaseMetadata={canReadDatabaseBinding}
                                  />
                                </div>
                              ) : null}
                            </div>
                          );
                        })}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </div>

            <div className="border-t border-neutral-100 bg-neutral-50/30">
              <SqlMonacoEditor
                ref={editorRef}
                value={sql}
                height={320}
                onChange={onChangeSql}
                readOnly={readOnly}
                className="h-full"
              />
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="font-semibold">
            {t('callDatabase.actions.cancel')}
          </Button>
          <Button
            onClick={() => onOpenChange(false)}
            size="lg"
            className="px-10 font-bold shadow-sm"
          >
            {t('callDatabase.actions.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default ExpandedSqlEditorDialog;
