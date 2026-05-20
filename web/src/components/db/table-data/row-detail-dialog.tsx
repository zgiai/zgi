'use client';

import type { FC } from 'react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Badge } from '@/components/ui/badge';
import type { DbTableColumn, DbTableRecord } from '@/services/types/db';
import { Type } from '@/services/types/db';
import { formatDate as formatDateDisplay } from '@/utils/format';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

interface RowDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  row: DbTableRecord | null;
  columns: readonly DbTableColumn[];
}

function formatCellValue(row: DbTableRecord, col: DbTableColumn): string {
  const val = row[col.name];
  if (Array.isArray(val)) return val.join(', ');
  if (val === null || val === undefined) return '';
  if (col.type === Type.Timestamp) {
    if (typeof val === 'string' && val.trim().length === 0) return '';
    return formatDateDisplay(val as number | string | Date, 'YYYY-MM-DD HH:mm:ss');
  }
  return String(val);
}

const RowDetailDialog: FC<RowDetailDialogProps> = ({ open, onOpenChange, row, columns }) => {
  const t = useT();
  const businessColumns = columns.filter(col => !col.is_system_field);
  const systemColumns = columns.filter(col => col.is_system_field);

  const renderFields = (items: readonly DbTableColumn[]) => (
    <div className="grid gap-3">
      {items.map(col => {
        const value = row ? formatCellValue(row, col) : '';
        const isLong = value.length > 160;
        return (
          <div
            key={col.id}
            className={cn('rounded-md border bg-background p-3', isLong && 'md:col-span-2')}
          >
            <div className="mb-2 flex min-w-0 items-center gap-2">
              <span className="truncate text-sm font-medium" title={col.name}>
                {col.name}
              </span>
              <Badge variant="secondary" className="text-[10px] uppercase">
                {col.type}
              </Badge>
              {col.is_system_field && (
                <Badge variant="outline" className="text-[10px]">
                  {t('dbs.columns.system')}
                </Badge>
              )}
            </div>
            {(col.source_column_name || col.description) && (
              <div className="mb-2 text-xs text-muted-foreground">
                {col.source_column_name ? `${col.source_column_name} · ` : ''}
                {col.description}
              </div>
            )}
            <div
              className={cn(
                'overflow-auto whitespace-pre-wrap break-words rounded bg-muted/30 px-3 py-2 text-sm leading-6',
                isLong ? 'max-h-[420px]' : 'max-h-[180px]'
              )}
            >
              {value || <span className="text-muted-foreground">-</span>}
            </div>
          </div>
        );
      })}
    </div>
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="xl" className="max-h-[calc(100vh-2rem)]">
        <DialogHeader>
          <DialogTitle>{t('dbs.tableData.rowDetail.title')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="px-6 pb-6">
          {row && (
            <div className="space-y-5">
              <section>
                <div className="mb-2 text-sm font-medium">
                  {t('dbs.tableData.rowDetail.businessFields')}
                </div>
                {renderFields(businessColumns)}
              </section>
              {systemColumns.length > 0 && (
                <section>
                  <div className="mb-2 text-sm font-medium text-muted-foreground">
                    {t('dbs.tableData.rowDetail.systemFields')}
                  </div>
                  {renderFields(systemColumns)}
                </section>
              )}
            </div>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
};

export default RowDetailDialog;
