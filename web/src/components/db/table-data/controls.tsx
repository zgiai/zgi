'use client';

// Top controls for the DB table data view: page size, sorting, and edit actions.
// English comments only as per project guidelines.

import type { FC } from 'react';
import React from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ArrowUp, Loader, Plus, Sparkle, RefreshCcw, FileUp, Columns3, Search } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { DbTableColumn } from '@/services/types/db';
import Link from 'next/link';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';

export interface TableDataControlsProps {
  pageSize: number;
  normalizedSortKey: string;
  columns: readonly DbTableColumn[];
  sortDir: 'ASC' | 'DESC';
  isEditing: boolean;
  isCreating: boolean;
  isUpdating: boolean;
  isDeleting: boolean;
  hasDataFields: boolean;
  onPageSizeChange: (value: string) => void;
  onSortKeyChange: (value: string) => void;
  onToggleSortDir: () => void;
  onAddRow: () => void;
  onCancel: () => void;
  onSave: () => void | Promise<void>;
  onStartEdit: () => void;
  dbId: string;
  tableId: string;
  onRefresh?: () => Promise<void>;
  onBatchImport?: () => void;
  canEditData?: boolean;
  canManage?: boolean;
  canBatchImport?: boolean;
  canSmartIngest?: boolean;
  pageSearch: string;
  onPageSearchChange: (value: string) => void;
  visibleColumnNames: readonly string[];
  onVisibleColumnNamesChange: (value: string[]) => void;
}

const Controls: FC<TableDataControlsProps> = ({
  pageSize,
  normalizedSortKey,
  columns,
  sortDir,
  isEditing,
  isCreating,
  isUpdating,
  isDeleting,
  hasDataFields,
  onPageSizeChange,
  onSortKeyChange,
  onToggleSortDir,
  onAddRow,
  onCancel,
  onSave,
  onStartEdit,
  dbId,
  tableId,
  onRefresh,
  canEditData,
  canManage,
  canBatchImport,
  canSmartIngest,
  onBatchImport,
  pageSearch,
  onPageSearchChange,
  visibleColumnNames,
  onVisibleColumnNamesChange,
}) => {
  const t = useT();
  const [reloading, setReloading] = React.useState<boolean>(false);

  const visibleNameSet = React.useMemo(() => new Set(visibleColumnNames), [visibleColumnNames]);
  const setColumnVisible = React.useCallback(
    (name: string, checked: boolean) => {
      if (checked) {
        onVisibleColumnNamesChange([...visibleColumnNames, name]);
        return;
      }

      if (visibleColumnNames.length <= 1) {
        toast.error(t('dbs.tableData.columns.keepOneVisible'));
        return;
      }

      onVisibleColumnNamesChange(visibleColumnNames.filter(item => item !== name));
    },
    [onVisibleColumnNamesChange, t, visibleColumnNames]
  );

  return (
    <div className="flex flex-wrap items-end justify-between gap-3">
      <div className="flex flex-wrap items-end gap-2">
        {isEditing && (
          <Button
            onClick={onAddRow}
            disabled={!hasDataFields}
            title={
              !hasDataFields
                ? t('dbs.tableData.noFields.editDisabledTip')
                : t('dbs.tableData.addRow')
            }
          >
            <Plus className="w-4 h-4" />
            {t('dbs.tableData.addRow')}
          </Button>
        )}
        {!isEditing && (
          <>
            <div className="min-w-[220px]">
              <span className="text-sm text-muted-foreground" title={t('dbs.tableData.search')}>
                {t('dbs.tableData.search')}
              </span>
              <div className="relative">
                <Search className="pointer-events-none absolute left-2 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={pageSearch}
                  onChange={event => onPageSearchChange(event.target.value)}
                  placeholder={t('dbs.tableData.searchPlaceholder')}
                  className="h-8 pl-8"
                />
              </div>
            </div>
            <div>
              <span
                className="text-sm text-muted-foreground"
                title={t('dbs.tableData.rowsPerPage')}
              >
                {t('dbs.tableData.rowsPerPage')}
              </span>
              <Select value={String(pageSize)} onValueChange={onPageSizeChange}>
                <SelectTrigger className="h-8 w-24">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="10">10</SelectItem>
                  <SelectItem value="20">20</SelectItem>
                  <SelectItem value="50">50</SelectItem>
                  <SelectItem value="100">100</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div>
              <span className="text-sm text-muted-foreground" title={t('dbs.tableData.sortBy')}>
                {t('dbs.tableData.sortBy')}
              </span>
              <div className="flex items-center">
                <Select value={normalizedSortKey} onValueChange={onSortKeyChange}>
                  <SelectTrigger className="h-8 w-48 rounded-r-none">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {columns.map(col => (
                      <SelectItem key={col.id} value={col.name} title={col.name}>
                        {col.display_name?.trim() || col.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onToggleSortDir}
                  className="rounded-l-none border-l-0"
                  title={
                    sortDir === 'ASC'
                      ? t('dbs.tableData.descending')
                      : t('dbs.tableData.ascending')
                  }
                  aria-label={
                    sortDir === 'ASC'
                      ? t('dbs.tableData.descending')
                      : t('dbs.tableData.ascending')
                  }
                >
                  <ArrowUp
                    className={cn(
                      'w-4 h-4 transition-transform duration-200',
                      sortDir === 'ASC' ? 'rotate-180' : ''
                    )}
                  />
                </Button>
              </div>
            </div>
            <Button
              variant="outline"
              size="sm"
              disabled={reloading}
              onClick={() => {
                if (!onRefresh) return;
                setReloading(true);
                Promise.resolve(onRefresh())
                  .then(() => {
                    toast.success(
                      t('common.refreshSuccess', { defaultMessage: 'Refresh successful' })
                    );
                  })
                  .finally(() => setReloading(false));
              }}
              title={t('common.refresh', { defaultMessage: 'Refresh' })}
              aria-label={t('common.refresh', { defaultMessage: 'Refresh' })}
            >
              <RefreshCcw className={`w-4 h-4 ${reloading ? 'animate-spin' : ''}`} />
              {t('common.refresh', { defaultMessage: 'Refresh' })}
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm" title={t('dbs.tableData.columns.title')}>
                  <Columns3 className="w-4 h-4" />
                  {t('dbs.tableData.columns.title')}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="max-h-[420px] w-72">
                <DropdownMenuLabel>{t('dbs.tableData.columns.visibleColumns')}</DropdownMenuLabel>
                <DropdownMenuSeparator />
                {columns.map(col => (
                  <DropdownMenuCheckboxItem
                    key={col.id}
                    checked={visibleNameSet.has(col.name)}
                    onCheckedChange={checked => setColumnVisible(col.name, Boolean(checked))}
                    onSelect={event => event.preventDefault()}
                    className="text-xs"
                  >
                    <span className="flex min-w-0 flex-col">
                      <span className="truncate" title={col.name}>
                        {col.name}
                      </span>
                      {(col.source_column_name || col.description || col.is_system_field) && (
                        <span className="truncate text-[10px] text-muted-foreground">
                          {col.is_system_field
                            ? t('dbs.columns.system')
                            : col.source_column_name || col.description}
                        </span>
                      )}
                    </span>
                  </DropdownMenuCheckboxItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          </>
        )}
      </div>

      <div className="flex items-center gap-2">
        {isEditing ? (
          <>
            <Button
              variant="outline"
              onClick={onCancel}
              disabled={isCreating || isUpdating || isDeleting}
              title={t('dbs.tableData.cancel')}
            >
              {t('dbs.tableData.cancel')}
            </Button>
            <Button
              onClick={onSave}
              disabled={isCreating || isUpdating || isDeleting}
              title={t('dbs.tableData.save')}
            >
              {isCreating || isUpdating || isDeleting ? (
                <Loader className="w-4 h-4 animate-spin" />
              ) : (
                t('dbs.tableData.save')
              )}
            </Button>
          </>
        ) : (
          <>
            {canManage && (
              <Button asChild variant="outline">
                <Link
                  href={`/console/db/${dbId}/table/${tableId}/structure`}
                  title={t('dbs.actions.manageStructure')}
                >
                  {t('dbs.actions.manageStructure')}
                </Link>
              </Button>
            )}

            {hasDataFields ? (
              <>
                {canSmartIngest && (
                  <Button asChild className="bg-highlight text-white hover:bg-highlight/90">
                    <Link
                      href={`/console/db/${dbId}/table/${tableId}/data`}
                      title={t('dbs.actions.smartIngest')}
                    >
                      <Sparkle className="w-4 h-4" />
                      {t('dbs.actions.smartIngest')}
                    </Link>
                  </Button>
                )}
                {canEditData && (
                  <Button onClick={onStartEdit} title={t('dbs.tableData.edit')}>
                    {t('dbs.tableData.edit')}
                  </Button>
                )}
                {canBatchImport && (
                  <Button
                    variant="outline"
                    onClick={onBatchImport}
                    title={t('dbs.batchImport.title')}
                  >
                    <FileUp className="w-4 h-4" />
                    {t('dbs.batchImport.title')}
                  </Button>
                )}
              </>
            ) : (
              canEditData && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="inline-flex">
                      <Button
                        onClick={onStartEdit}
                        disabled
                        title={t('dbs.tableData.noFields.editDisabledTip')}
                      >
                        {t('dbs.tableData.edit')}
                      </Button>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent side="top" className="max-w-[280px]">
                    <div className="text-xs">{t('dbs.tableData.noFields.editDisabledTip')}</div>
                  </TooltipContent>
                </Tooltip>
              )
            )}
          </>
        )}
      </div>
    </div>
  );
};

// Memoize to avoid unnecessary re-renders when props are stable
export const TableDataControls = React.memo(Controls);

export default TableDataControls;
