'use client';

// Step 2: Merge AI columns with existing, edit, and save

import React, { useCallback, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableHeader, TableRow, TableHead } from '@/components/ui/table';
import TableColumnsEditor from '@/components/db/table-columns/table-colums-editor';
import { Plus, Loader } from 'lucide-react';
import { useT } from '@/i18n';
import { useDbTableColumns, useUpdateDbTableColumns } from '@/hooks/db/use-db-table-columns';
import type { DbTableColumn } from '@/services/types/db';
import { Type } from '@/services/types/db';
import { generateClientId } from '@/utils/client-id';

export interface StepTwoProps {
  dbId: string;
  tableId: string;
  aiColumns: DbTableColumn[];
}

// Merge AI columns with existing columns by `name` (case-insensitive)
function mergeColumns(existing: DbTableColumn[], ai: DbTableColumn[]): DbTableColumn[] {
  const byName = new Map<string, DbTableColumn>();
  const order: string[] = [];
  existing.forEach(col => {
    const key = col.name.toLowerCase();
    byName.set(key, col);
    order.push(key);
  });
  ai.forEach(col => {
    const key = col.name.toLowerCase();
    const prev = byName.get(key);
    if (prev) {
      // Prefer existing system fields unchanged; fill description if missing
      const isSystem = Boolean(prev.is_system_field);
      byName.set(key, {
        ...prev,
        description: prev.description || col.description,
        type: isSystem ? prev.type : (prev.type ?? col.type),
        is_required: isSystem ? prev.is_required : prev.is_required,
      });
    } else {
      byName.set(key, { ...col, is_system_field: false });
      order.push(key);
    }
  });
  return order.map(k => byName.get(k)!).filter(Boolean);
}

// Generate a unique local id for client-side editing (do not rely on server ids)
function genLocalId(): string {
  return generateClientId('local');
}

// Ensure each column has a stable unique id; keep server ids, assign local ids for new/AI-only ones
function ensureUniqueIds(columns: DbTableColumn[]): DbTableColumn[] {
  const used = new Set<string>();
  const out: DbTableColumn[] = [];
  for (const col of columns) {
    const currentId = (col.id || '').trim();
    let nextId = currentId.length > 0 ? currentId : '';
    if (nextId.length === 0 || used.has(nextId)) {
      nextId = genLocalId();
    }
    used.add(nextId);
    out.push({ ...col, id: nextId });
  }
  return out;
}

export default function StepTwo({ dbId, tableId, aiColumns }: StepTwoProps) {
  const t = useT('dbs');
  const router = useRouter();

  const { columns: existingColumns, isLoading: isLoadingExisting } = useDbTableColumns(
    dbId,
    tableId,
    {
      includeSystemFields: true,
      enabled: true,
    }
  );
  const { updateColumns, isPending: isSaving } = useUpdateDbTableColumns(dbId, tableId);

  const [editableColumns, setEditableColumns] = useState<DbTableColumn[]>([]);
  const [initializedMerge, setInitializedMerge] = useState<boolean>(false);
  const [validation, setValidation] = useState<{
    hasDuplicateNames: boolean;
    hasInvalidNames: boolean;
  }>({
    hasDuplicateNames: false,
    hasInvalidNames: false,
  });

  useEffect(() => {
    if (!initializedMerge && Array.isArray(existingColumns)) {
      const merged = mergeColumns(existingColumns, aiColumns);
      const withIds = ensureUniqueIds(merged);
      setEditableColumns(withIds);
      setInitializedMerge(true);
    }
  }, [initializedMerge, existingColumns, aiColumns]);

  const onAddColumn = useCallback(() => {
    setEditableColumns(prev => [
      ...prev,
      {
        id: genLocalId(),
        name: '',
        description: '',
        type: Type.Text,
        is_required: false,
        is_system_field: false,
      },
    ]);
  }, []);

  const onSave = useCallback(async () => {
    const normalized = editableColumns.map(c => ({
      ...c,
      is_system_field: Boolean(c.is_system_field),
    }));
    await updateColumns(normalized);
    // Navigate to table structure management page after successful save
    router.push(`/console/db/${dbId}/table/${tableId}/structure`);
  }, [editableColumns, updateColumns, router, dbId, tableId]);

  return (
    <div className="h-0 grow flex flex-col">
      <div className="flex items-center justify-between mb-2">
        <div className="text-sm text-muted-foreground">{t('createPage.step2Header')}</div>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={onAddColumn} className="gap-1">
            <Plus className="h-4 w-4" /> {t('createPage.addField')}
          </Button>
          <Button
            onClick={onSave}
            disabled={isSaving || validation.hasInvalidNames || validation.hasDuplicateNames}
            className="gap-1"
          >
            {isSaving ? (
              <span className="inline-flex items-center gap-2">
                <Loader className="h-4 w-4 animate-spin" /> {t('createPage.saving')}
              </span>
            ) : (
              t('createPage.save')
            )}
          </Button>
        </div>
      </div>

      <div className="border rounded-md overflow-y-auto">
        {isLoadingExisting && !initializedMerge ? (
          <div className="p-4 space-y-2">
            {Array.from({ length: 10 }).map((_, i) => (
              <Skeleton key={i} className="h-6 w-full" />
            ))}
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[200px]">{t('createPage.colFieldName')}</TableHead>
                <TableHead>{t('createPage.colDescription')}</TableHead>
                <TableHead className="w-[160px]">{t('createPage.colType')}</TableHead>
                <TableHead className="w-[100px]">{t('createPage.colRequired')}</TableHead>
                <TableHead className="w-[80px]">{t('createPage.colActions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableColumnsEditor
              columns={editableColumns}
              onChange={setEditableColumns}
              showActions
              onValidationChange={state =>
                setValidation({
                  hasDuplicateNames: state.hasDuplicateNames,
                  hasInvalidNames: state.hasInvalidNames,
                })
              }
            />
          </Table>
        )}
      </div>
    </div>
  );
}
