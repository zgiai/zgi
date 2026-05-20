'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { useT } from '@/i18n';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
// Dialog removed; inline add row in table
import { Skeleton } from '@/components/ui/skeleton';
import { useDbTableColumns, useUpdateDbTableColumns } from '@/hooks/db/use-db-table-columns';
import type { DbTableColumn } from '@/services/types/db';
import { Type } from '@/services/types/db';
import { Loader, Plus, Sparkle } from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { BannerKey, hideBanner, isBannerHidden } from '@/utils/ui-local';
import TableColumnsEditor from '@/components/db/table-columns/table-colums-editor';
import { SchemaHealthNotice } from '@/components/db/schema-health';
import { generateClientId } from '@/utils/client-id';

interface TableColumnsProps {
  dbId: string;
  tableId: string;
}

// Helper to generate temporary ID for new columns
function generateTempId(): string {
  return generateClientId('tmp');
}

import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';

export default function TableColumns({ dbId, tableId }: TableColumnsProps) {
  const t = useT('dbs');

  // Permissions
  const { hasPermission } = useAccountPermissions();
  const canManage = hasPermission('database.manage');

  const { columns, isLoading, isFetching } = useDbTableColumns(dbId, tableId, {
    includeSystemFields: true,
  });
  const { updateColumns, isPending } = useUpdateDbTableColumns(dbId, tableId);

  // Always editing mode
  // Initialize with empty array to avoid undefined during first render
  const [localColumns, setLocalColumns] = useState<DbTableColumn[]>([]);
  // View/Edit mode state – default to view (non-editing)
  const [isEditing, setIsEditing] = useState<boolean>(false);
  const [hideDeleteWarning, setHideDeleteWarning] = useState<boolean>(() =>
    isBannerHidden(BannerKey.TableColumnsDeleteWarning)
  );

  // Validation state from editor (duplicate or invalid names)
  const [validation, setValidation] = useState<{
    hasDuplicateNames: boolean;
    hasInvalidNames: boolean;
  }>({ hasDuplicateNames: false, hasInvalidNames: false });

  // Initial sync: populate local state once when columns arrive
  useEffect(() => {
    if (localColumns.length === 0 && columns.length > 0) {
      setLocalColumns(columns);
    }
  }, [columns, localColumns.length]);

  const addEmptyColumn = () => {
    const newCol: DbTableColumn = {
      id: generateTempId(),
      name: '',
      description: '',
      type: Type.Text,
      is_required: false,
    };
    setLocalColumns(prev => [...prev, newCol]);
  };

  const onSave = async () => {
    if (validation.hasDuplicateNames || validation.hasInvalidNames) return;
    const next = await updateColumns(localColumns);
    setLocalColumns(next);
    setIsEditing(false);
  };

  const onCancel = () => {
    setLocalColumns(columns);
    setIsEditing(false);
  };

  if (isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-4 h-full flex flex-col">
      {!isEditing && <SchemaHealthNotice columns={localColumns} compact />}
      {isEditing && !hideDeleteWarning && (
        <Alert variant="destructive" className="flex items-center justify-between py-2">
          <AlertDescription className="pr-3">{t('columns.deleteWarning')}</AlertDescription>
          <Button
            variant="destructive"
            size="sm"
            onClick={() => {
              hideBanner(BannerKey.TableColumnsDeleteWarning);
              setHideDeleteWarning(true);
            }}
          >
            {t('columns.doNotShowAgain')}
          </Button>
        </Alert>
      )}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          {isFetching && (
            <span className="text-xs text-muted-foreground">{t('columns.syncing')}</span>
          )}
        </div>
        {isEditing ? (
          <Button onClick={addEmptyColumn}>
            <Plus className="w-4 h-4" />
            {t('columns.add')}
          </Button>
        ) : (
          <div className="flex items-center gap-2">
            <Button asChild variant="outline">
              <Link href={`/console/db/${dbId}/table/${tableId}`} title={t('actions.viewData')}>
                {t('actions.viewData')}
              </Link>
            </Button>
            {canManage && (
              <Button asChild className="bg-highlight text-white hover:bg-highlight/90">
                <Link
                  href={`/console/db/${dbId}/table/${tableId}/create`}
                  title={t('actions.smartGenerate')}
                >
                  <Sparkle className="w-4 h-4" />
                  {t('actions.smartGenerate')}
                </Link>
              </Button>
            )}
            {canManage && <Button onClick={() => setIsEditing(true)}>{t('actions.edit')}</Button>}
          </div>
        )}
      </div>

      <div className="h-0 grow overflow-auto">
        <Table containerClassName="w-full border rounded-md">
          <TableHeader>
            <TableRow>
              <TableHead className="w-[260px]">{t('columns.name')}</TableHead>
              <TableHead>{t('columns.description')}</TableHead>
              <TableHead className="w-[180px]">{t('columns.type')}</TableHead>
              <TableHead className="w-[140px]">{t('columns.required')}</TableHead>
              {isEditing && <TableHead className="w-[120px]">{t('columns.actions')}</TableHead>}
            </TableRow>
          </TableHeader>
          {isEditing ? (
            <TableColumnsEditor
              columns={localColumns}
              onChange={setLocalColumns}
              showActions
              onValidationChange={state =>
                setValidation({
                  hasDuplicateNames: state.hasDuplicateNames,
                  hasInvalidNames: state.hasInvalidNames,
                })
              }
            />
          ) : (
            <TableBody>
              {localColumns.map(col => (
                <TableRow key={col.id}>
                  <>
                    <TableCell>
                      <span className="truncate" title={col.name}>
                        {col.name || '-'}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className="truncate" title={col.description}>
                        {col.description || '-'}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className="uppercase text-xs text-muted-foreground">{col.type}</span>
                    </TableCell>
                    <TableCell>
                      <Switch checked={col.is_required} disabled />
                    </TableCell>
                  </>
                </TableRow>
              ))}
            </TableBody>
          )}
        </Table>
      </div>

      {isEditing && (
        <div className="flex items-center justify-end gap-2">
          <Button variant="outline" onClick={onCancel} disabled={isPending}>
            {t('columns.cancel')}
          </Button>
          <Button
            onClick={onSave}
            disabled={isPending || validation.hasDuplicateNames || validation.hasInvalidNames}
          >
            {isPending ? <Loader className="w-4 h-4 animate-spin" /> : t('columns.save')}
          </Button>
        </div>
      )}
    </div>
  );
}

// Inline add dialog removed in favor of Add Column button that inserts an empty row
