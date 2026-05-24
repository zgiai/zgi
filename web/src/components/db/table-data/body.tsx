'use client';

// Table body rendering with editable cells and skeleton placeholders.
// English comments only as per project guidelines.

import type { FC } from 'react';
import React, { useCallback } from 'react';
import { cn } from '@/lib/utils';
import { withBasePath, withBasePathIfInternal } from '@/lib/config';
import { TableBody, TableCell, TableRow } from '@/components/ui/table';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { TrashIcon, Plus, Sparkles, FileUp, Table2, Eye } from 'lucide-react';
import type { DbTableColumn, DbTableRecord } from '@/services/types/db';
import { Type } from '@/services/types/db';
import { EmptyElement } from '@/components/datasets/empty-element';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import {
  datetimeLocalToWallTime,
  formatTimestampLocalInput,
  formatTimestampWallTime,
} from './timestamp-utils';

export interface TableDataBodyProps {
  loading: boolean;
  pageSize: number;
  columns: readonly DbTableColumn[];
  isEditing: boolean;
  localRows: readonly DbTableRecord[];
  records: readonly DbTableRecord[];
  onDeleteRow: (row: DbTableRecord) => void | Promise<void>;
  updateLocalCell: (
    rowKey: string | number,
    colName: string,
    value: DbTableRecord[keyof DbTableRecord]
  ) => void;
  drafts: Readonly<Record<string, string>>;
  setDrafts: React.Dispatch<React.SetStateAction<Record<string, string>>>;
  onAddRow: () => void;
  onBatchImport?: () => void;
  hasDataFields: boolean;
  manageStructureHref: string;
  smartCreateHref: string;
  smartIngestHref: string;
  canEditData?: boolean;
  canManage?: boolean;
  containerWidth: number;
  onOpenRow?: (row: DbTableRecord) => void;
  stickyColumnNames?: readonly string[];
}

// Render read-only cell content with type-aware formatting
const renderCell = (row: DbTableRecord, col: DbTableColumn): string => {
  const val = row[col.name];
  if (Array.isArray(val)) {
    const arr = val as ReadonlyArray<string | number>;
    return arr.join(', ');
  }
  if (val === null || val === undefined) return '';
  if (col.type === Type.Timestamp) {
    if (typeof val === 'string' && val.trim().length === 0) return '';
    return formatTimestampWallTime(val);
  }
  return String(val);
};

const getRowKey = (row: DbTableRecord, index: number): string | number => {
  const keyedRow = row as { id?: string | number; __temp_id?: string };

  return keyedRow.id ?? keyedRow.__temp_id ?? `unsaved-row-${index}`;
};

const Body: FC<TableDataBodyProps> = ({
  loading,
  pageSize,
  columns,
  isEditing,
  localRows,
  records,
  onDeleteRow,
  updateLocalCell,
  drafts,
  setDrafts,
  onAddRow,
  onBatchImport,
  hasDataFields,
  manageStructureHref,
  smartCreateHref,
  smartIngestHref,
  canEditData,
  canManage,
  containerWidth,
  onOpenRow,
  stickyColumnNames = [],
}) => {
  const isCompleteLocal = useCallback((s: string): boolean => {
    return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(s);
  }, []);
  const rows = isEditing ? localRows : records;
  const t = useT();

  if (loading && records.length === 0) {
    return (
      <TableBody>
        {Array.from({ length: 5 }).map((_, idx) => (
          <TableRow key={idx}>
            {columns.map(col => (
              <TableCell key={col.id}>
                <Skeleton className="h-4 w-full" />
              </TableCell>
            ))}
            {isEditing && (
              <TableCell>
                <Skeleton className="h-8 w-8" />
              </TableCell>
            )}
            {!isEditing && (
              <TableCell>
                <Skeleton className="h-8 w-8" />
              </TableCell>
            )}
          </TableRow>
        ))}
      </TableBody>
    );
  }

  return (
    <TableBody className="text-xs">
      {loading ? (
        Array.from({ length: pageSize }).map((_, rIdx) => (
          <TableRow key={`data-skeleton-row-${rIdx}`}>
            {Array.from({ length: Math.max(columns.length, 6) }).map((__, cIdx) => (
              <TableCell
                key={`data-skeleton-cell-${rIdx}-${cIdx}`}
                className="border-r last:border-r-0"
              >
                <Skeleton className="h-4 w-full" />
              </TableCell>
            ))}
          </TableRow>
        ))
      ) : rows.length === 0 ? (
        <TableRow>
          <TableCell
            colSpan={Math.max(columns.length + (isEditing || onOpenRow ? 1 : 0), 1)}
            className="h-[400px] border-none p-0"
          >
            <div
              className="sticky left-0 flex items-center justify-center h-full"
              style={{ width: containerWidth ? `${containerWidth}px` : '100%' }}
            >
              <EmptyElement
                type="generic"
                title={
                  hasDataFields ? t('dbs.tableData.empty.title') : t('dbs.tableData.noFields.title')
                }
                description={
                  hasDataFields ? t('dbs.tableData.empty.desc') : t('dbs.tableData.noFields.desc')
                }
                illustration={
                  <img
                    src={withBasePath('/window.svg')}
                    alt=""
                    className="mx-auto h-20 w-auto opacity-80"
                  />
                }
                actions={(hasDataFields
                  ? [
                      canEditData && {
                        label: t('dbs.tableData.addRow'),
                        icon: <Plus className="h-4 w-4" />,
                        onClick: onAddRow,
                        variant: 'outline' as const,
                      },
                      canEditData && {
                        label: t('dbs.batchImport.title'),
                        icon: <FileUp className="h-4 w-4" />,
                        onClick: onBatchImport ?? (() => {}),
                        variant: 'outline' as const,
                      },
                      canEditData && {
                        label: t('dbs.actions.smartIngest'),
                        icon: <Sparkles className="h-4 w-4" />,
                        onClick: () => {
                          window.location.href = withBasePathIfInternal(smartIngestHref);
                        },
                        highlight: true,
                      },
                    ]
                  : [
                      canManage && {
                        label: t('dbs.actions.manageStructure'),
                        icon: <Table2 className="h-4 w-4" />,
                        onClick: () => {
                          window.location.href = withBasePathIfInternal(manageStructureHref);
                        },
                        variant: 'outline' as const,
                      },
                      canManage && {
                        label: t('dbs.actions.smartGenerate'),
                        icon: <Sparkles className="h-4 w-4" />,
                        onClick: () => {
                          window.location.href = withBasePathIfInternal(smartCreateHref);
                        },
                        highlight: true,
                      },
                    ]
                ).filter((a): a is Exclude<typeof a, boolean | undefined> => !!a)}
                className="py-12"
              />
            </div>
          </TableCell>
        </TableRow>
      ) : (
        rows.map((row, rIdx) => {
          const rowKey = getRowKey(row, rIdx);
          return (
            <TableRow key={`data-row-${rowKey}`}>
              {columns.map(col => (
                <TableCell
                  key={`data-cell-${rowKey}-${col.id}`}
                  className={cn(
                    'border-r last:border-r-0 hover:bg-highlight/5 bg-background',
                    stickyColumnNames.includes(col.name) &&
                      'sticky left-0 z-10 min-w-[140px] shadow-[1px_0_0_hsl(var(--border))]',
                    isEditing && !col.is_system_field
                      ? 'p-2 overflow-visible'
                      : 'p-0 overflow-hidden'
                  )}
                  onDoubleClick={() => {
                    if (isEditing) {
                      return;
                    }
                    try {
                      navigator.clipboard.writeText(renderCell(row, col));
                      toast.success(t('dbs.tableData.copyToClipboard'));
                    } catch (e) {
                      console.error(e);
                    }
                  }}
                >
                  {isEditing && !col.is_system_field
                    ? (() => {
                        const val = row[col.name];
                        // Boolean editor
                        if (col.type === Type.Boolean) {
                          return (
                            <div className="flex items-center">
                              <Switch
                                checked={Boolean(val)}
                                onCheckedChange={checked =>
                                  updateLocalCell(rowKey as string | number, col.name, !!checked)
                                }
                                title={t('dbs.tableData.inputs.booleanTitle')}
                              />
                            </div>
                          );
                        }

                        // Numeric editor – keep a local draft to allow empty/partial typing
                        if (col.type === Type.Integer || col.type === Type.Numeric) {
                          const cellKey = `${String(rowKey)}:${col.name}`;
                          const draftValue = drafts[cellKey];
                          const inputVal =
                            draftValue !== undefined
                              ? draftValue
                              : typeof val === 'number'
                                ? String(val)
                                : '';
                          const invalidNumeric = col.is_required && inputVal === '';

                          return (
                            <Input
                              type="number"
                              value={inputVal}
                              aria-invalid={invalidNumeric ? true : undefined}
                              className={cn(
                                'aria-invalid:bg-destructive/5 py-0 h-8',
                                invalidNumeric && 'border-destructive ring-1 ring-destructive/30'
                              )}
                              onChange={e => {
                                const local = e.target.value;
                                // Persist draft for inline validation feedback
                                setDrafts(prev => ({ ...prev, [cellKey]: local }));
                                if (local === '') {
                                  // Keep empty to highlight when required; normalize on save
                                  updateLocalCell(rowKey as string | number, col.name, '');
                                  return;
                                }
                                const num = Number(local);
                                if (!Number.isNaN(num)) {
                                  updateLocalCell(rowKey as string | number, col.name, num);
                                }
                              }}
                              onBlur={e => {
                                const local = e.target.value;
                                if (local === '') {
                                  // Leave as empty string; save normalization will handle non-required
                                  return;
                                }
                                const num = Number(local);
                                if (!Number.isNaN(num)) {
                                  updateLocalCell(rowKey as string | number, col.name, num);
                                  // Clear draft to reflect committed numeric value
                                  setDrafts(prev => {
                                    const next = { ...prev };
                                    delete next[cellKey];
                                    return next;
                                  });
                                }
                              }}
                              title={t('dbs.tableData.inputs.numberTitle')}
                            />
                          );
                        }

                        // Timestamp editor
                        if (col.type === Type.Timestamp) {
                          const cellKey = `${String(rowKey)}:${col.name}`;
                          const formatted = formatTimestampLocalInput(val);
                          const draftValue = drafts[cellKey];
                          const inputVal = draftValue !== undefined ? draftValue : formatted;
                          const invalidTimestamp =
                            col.is_required &&
                            (inputVal === '' ||
                              (draftValue !== undefined && !isCompleteLocal(draftValue)) ||
                              formatted === '');

                          return (
                            <Input
                              type="datetime-local"
                              value={inputVal}
                              aria-invalid={invalidTimestamp ? true : undefined}
                              className={cn(
                                'aria-invalid:bg-destructive/5 py-0 h-8',
                                invalidTimestamp && 'border-destructive ring-1 ring-destructive/30'
                              )}
                              onChange={e => {
                                const local = e.target.value; // partial allowed
                                setDrafts(prev => ({ ...prev, [cellKey]: local }));
                                if (local === '') {
                                  updateLocalCell(rowKey as string | number, col.name, '');
                                  return;
                                }
                                if (isCompleteLocal(local)) {
                                  updateLocalCell(
                                    rowKey as string | number,
                                    col.name,
                                    datetimeLocalToWallTime(local)
                                  );
                                }
                              }}
                              onBlur={e => {
                                const local = e.target.value;
                                if (local && isCompleteLocal(local)) {
                                  updateLocalCell(
                                    rowKey as string | number,
                                    col.name,
                                    datetimeLocalToWallTime(local)
                                  );
                                  // Clear draft to reflect committed wall time in state
                                  setDrafts(prev => {
                                    const next = { ...prev };
                                    delete next[cellKey];
                                    return next;
                                  });
                                }
                              }}
                              title={t('dbs.tableData.inputs.timestampTitle')}
                            />
                          );
                        }

                        // Text editor (default)
                        const invalidText =
                          col.is_required &&
                          (typeof val !== 'string' || (val as string).trim().length === 0);
                        return (
                          <Textarea
                            rows={1}
                            value={typeof val === 'string' ? val : ''}
                            aria-invalid={invalidText ? true : undefined}
                            className={cn(
                              'aria-invalid:bg-destructive/5 min-h-[32px] max-h-[200px] min-w-[150px] max-w-[600px] resize px-2 py-1',
                              invalidText && 'border-destructive ring-1 ring-destructive/30'
                            )}
                            onChange={e =>
                              updateLocalCell(rowKey as string | number, col.name, e.target.value)
                            }
                            title={t('dbs.tableData.inputs.textTitle')}
                          />
                        );
                      })()
                    : (() => {
                        const rendered = renderCell(row, col);
                        const isLongText = col.type === Type.Text && rendered.length > 120;
                        return (
                          <div
                            className={cn(
                              'max-h-[96px] min-w-[120px] max-w-[420px] overflow-hidden p-2 leading-5',
                              col.is_system_field || col.type === Type.Timestamp
                                ? 'truncate'
                                : 'whitespace-pre-wrap break-words',
                              isLongText && 'cursor-zoom-in'
                            )}
                            title={rendered}
                            onClick={() => {
                              if (isLongText) {
                                onOpenRow?.(row);
                              }
                            }}
                          >
                            <span className={isLongText ? 'line-clamp-4' : undefined}>
                              {rendered}
                            </span>
                          </div>
                        );
                      })()}
                </TableCell>
              ))}

              {isEditing ? (
                <TableCell className="border-r last:border-r-0">
                  <Button
                    variant="ghost"
                    className="h-7 w-7 hover:bg-destructive/30 hover:text-destructive"
                    isIcon
                    onClick={() => onDeleteRow(row)}
                    title={t('dbs.tableData.deleteRow')}
                  >
                    <TrashIcon className="h-4 w-4" />
                  </Button>
                </TableCell>
              ) : (
                onOpenRow && (
                  <TableCell className="sticky right-0 z-10 border-r bg-background shadow-[-1px_0_0_hsl(var(--border))]">
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-7 gap-1 whitespace-nowrap px-2 text-xs"
                      onClick={() => onOpenRow(row)}
                      title={t('dbs.tableData.rowDetail.open')}
                    >
                      <Eye className="h-4 w-4" />
                      {t('dbs.tableData.rowDetail.openShort')}
                    </Button>
                  </TableCell>
                )
              )}
            </TableRow>
          );
        })
      )}
    </TableBody>
  );
};

export const TableDataBody = React.memo(Body);

export default TableDataBody;
