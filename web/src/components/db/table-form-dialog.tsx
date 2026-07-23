'use client';

import { useEffect, useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { useCreateDbTable, useUpdateDbTable } from '@/hooks/db/use-db-tables';
import { useT } from '@/i18n';
import type { DbTable } from '@/services/types/db';
import { getTableNameValidationErrors, type TableNameErrorCode } from '@/utils/validation';

interface DbTableFormDialogProps {
  dbId: string;
  mode: 'create' | 'edit';
  open: boolean;
  onOpenChange: (open: boolean) => void;
  table?: DbTable | null;
  tables: DbTable[];
}

/**
 * @component DbTableFormDialog
 * @category Feature
 * @status Stable
 * @description Shared create/edit dialog for DB tables with consistent validation and submit flow.
 * @usage Use in DB overview and sidebar actions to create or rename tables.
 * @example
 * <DbTableFormDialog dbId={dbId} mode="create" open={open} onOpenChange={setOpen} tables={tables} />
 */
export function DbTableFormDialog({
  dbId,
  mode,
  open,
  onOpenChange,
  table,
  tables,
}: DbTableFormDialogProps) {
  const router = useRouter();
  const t = useT();
  const createMutation = useCreateDbTable(dbId);
  const updateMutation = useUpdateDbTable(dbId, table?.id);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');

  const isEdit = mode === 'edit';
  const title = isEdit ? t('dbs.tableModal.editTitle') : t('dbs.tableModal.createTitle');
  const submitLabel = isEdit ? t('common.save') : t('common.create');
  const currentTableId = table ? String(table.id) : null;

  useEffect(() => {
    if (!open) return;
    setName(table?.name || table?.table_name || '');
    setDescription(table?.description ?? '');
  }, [open, table]);

  const isDuplicateTableName = useMemo(() => {
    const candidate = name.trim().toLowerCase();
    if (!candidate) return false;

    return tables.some(item => {
      const itemId = String(item.id);
      if (itemId.startsWith('optimistic-')) return false;
      if (currentTableId && itemId === currentTableId) return false;

      return (item.name || item.table_name || '').toLowerCase() === candidate;
    });
  }, [currentTableId, name, tables]);

  const tableNameErrors: TableNameErrorCode[] = useMemo(() => {
    const errors = getTableNameValidationErrors(name);
    if (isDuplicateTableName) errors.push('duplicate');
    return errors;
  }, [isDuplicateTableName, name]);

  const tableNameErrorMessages = useMemo(
    () => tableNameErrors.map(code => t(`dbs.validation.tableName.${code}`)),
    [tableNameErrors, t]
  );

  const isPending = isEdit ? updateMutation.isPending : createMutation.isPending;

  const handleSubmit = async () => {
    if (tableNameErrors.length > 0) return;

    if (isEdit) {
      if (!table?.id) return;
      await updateMutation.mutateAsync({ name, description });
      onOpenChange(false);
      return;
    }

    const result = await createMutation.mutateAsync({ name, description });
    onOpenChange(false);
    router.push(`/console/db/${dbId}/table/${result.data.id}/structure`);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-3">
          <div className="space-y-1">
            <div className="flex items-center justify-between gap-2">
              <Label htmlFor="db-table-name">{t('common.name')}</Label>
              {!isEdit && (
                <span className="text-xs text-muted-foreground">
                  {t('dbs.tableModal.nameLimitHint', { count: name.length })}
                </span>
              )}
            </div>
            <Input
              id="db-table-name"
              value={name}
              onChange={e => setName(e.target.value)}
              maxLength={isEdit ? undefined : 63}
              aria-invalid={tableNameErrors.length > 0}
            />
            {tableNameErrors.length > 0 && (
              <div className="mt-1 space-y-1 text-[13px] text-destructive">
                {tableNameErrorMessages.map((message, index) => (
                  <div key={index}>{message}</div>
                ))}
              </div>
            )}
          </div>
          <div className="space-y-1">
            <Label htmlFor="db-table-description">{t('common.description')}</Label>
            <Textarea
              id="db-table-description"
              rows={3}
              className="resize-none"
              value={description}
              onChange={e => setDescription(e.target.value)}
            />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={isPending || tableNameErrors.length > 0}>
            {submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
