'use client';

// Data tab for a DB table: paginated list (no infinite scroll)
// English comments only as per project guidelines.

import type { FC } from 'react';
import { useCallback, useEffect, useMemo, useState, useRef } from 'react';
// UI and hooks split across subcomponents for readability and performance.
import { Table } from '@/components/ui/table';
import { Pagination } from '@/components/ui/pagination';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useDbTableColumns } from '@/hooks/db/use-db-table-columns';
import {
  useDbTableRecords,
  useCreateDbTableRecords,
  useUpdateDbTableRecords,
  useDeleteDbTableRecord,
} from '@/hooks/db/use-db-table-records';
import type { GetDbTableRecordsParams } from '@/services/types/db';
import { type DbTableRecord, Type } from '@/services/types/db';
import type { DbRecordValue, DbTableColumn } from '@/services/types/db';
import { useDbTableDetail } from '@/hooks/db/use-db-tables';

import { TableDataControls } from './controls';
import { TableDataHeader } from './header';
import { TableDataBody } from './body';
import BatchImportDialog from './batch-import-dialog';
import RowDetailDialog from './row-detail-dialog';
import { isBusinessTimeColumn, SchemaHealthNotice } from '@/components/db/schema-health';
import { toast } from 'sonner';
import { getErrorMessage } from '@/utils/error-notifications';
import {
  datetimeLocalToWallTime,
  formatTimestampWallTime,
  isTimestampValueValid,
} from './timestamp-utils';

interface TableDataProps {
  dbId: string;
  tableId: string;
}

const DEFAULT_PAGE_SIZE = 20;

import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { DATABASE_PERMISSION_ACTIONS } from '@/constants/permissions';

const TableData: FC<TableDataProps> = ({ dbId, tableId }) => {
  const t = useT();

  // Permissions
  const { hasAnyPermission } = useAccountPermissions();
  const canCreateRecord = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.recordCreate);
  const canUpdateRecord = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.recordUpdate);
  const canDeleteRecord = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.recordDelete);
  const canEditData = canCreateRecord || canUpdateRecord || canDeleteRecord;
  const canManageSchema = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.schemaManage);
  const canAnalyzeImport = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.importAnalyze);
  const canExecuteImport = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.importExecute);
  const canBatchImport = canExecuteImport;
  const canSmartIngest = canAnalyzeImport && canCreateRecord;

  // Fetch table columns (structure).
  const {
    columns,
    isLoading: colsLoading,
    refetch: colsRefetch,
  } = useDbTableColumns(dbId, tableId, {
    includeSystemFields: true,
    staleTime: 60_000,
    gcTime: 600_000,
  });

  // Fetch table detail (metadata) in parallel with columns and records.
  const {
    data: _tableDetailResp,
    isLoading: detailLoading,
    refetch: detailRefetch,
  } = useDbTableDetail(dbId, tableId, {
    staleTime: 60_000,
    gcTime: 600_000,
  });

  // Pagination state
  const [pageSize, setPageSize] = useState<number>(DEFAULT_PAGE_SIZE);
  const [page, setPage] = useState<number>(1);
  const [discardConfirmOpen, setDiscardConfirmOpen] = useState(false);

  // Sorting state: prefer recency fields for record-heavy business tables.
  const defaultSortKey = useMemo(() => {
    const preferred = ['updated_time', 'operation_time', 'admission_time', 'created_time', 'id'];
    const preferredCol = preferred.find(name => columns.some(c => c.name === name));
    if (preferredCol) return preferredCol;
    const inferredTimeCol = columns.find(col =>
      isBusinessTimeColumn(col, ['操作时间', '入院时间', 'operation_time', 'admission_time'])
    );
    if (inferredTimeCol) return inferredTimeCol.name;
    const firstDataCol = columns.find(c => !c.is_system_field) ?? columns[0];
    return firstDataCol?.name ?? 'id';
  }, [columns]);
  const [sortKey, setSortKey] = useState<string>(defaultSortKey);
  const [sortDir, setSortDir] = useState<'ASC' | 'DESC'>('DESC');

  // Ensure sortKey stays valid when columns change
  const normalizedSortKey = useMemo(() => {
    const names = new Set(columns.map(c => c.name));
    return names.has(sortKey) ? sortKey : defaultSortKey;
  }, [columns, sortKey, defaultSortKey]);

  const params: GetDbTableRecordsParams = useMemo(
    () => ({
      limit: pageSize,
      offset: (page - 1) * pageSize,
      order: `${normalizedSortKey} ${sortDir}`,
    }),
    [pageSize, page, normalizedSortKey, sortDir]
  );

  // Fetch records with current pagination and sort
  const {
    records,
    total,
    isLoading,
    refetch: recordsRefetch,
  } = useDbTableRecords(dbId, tableId, params, {
    staleTime: 30_000,
    gcTime: 600_000,
  });

  // Mutations (CRUD) with optimistic updates handled inside hooks
  const { createRecords, isPending: isCreating } = useCreateDbTableRecords(dbId, tableId, params);
  const { updateRecords, isPending: isUpdating } = useUpdateDbTableRecords(dbId, tableId, params);
  const { deleteRecords, isPending: isDeleting } = useDeleteDbTableRecord(dbId, tableId, params);

  const totalPages = useMemo(() => {
    return pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
  }, [total, pageSize]);

  // Handlers
  const onPageSizeChange = useCallback((value: string) => {
    const size = Number(value) || DEFAULT_PAGE_SIZE;
    // Reset to page 1 when page size changes
    setPageSize(size);
    setPage(1);
  }, []);

  const onSortKeyChange = useCallback((value: string) => {
    setSortKey(value);
    setPage(1);
  }, []);

  const onToggleSortDir = useCallback(() => {
    setSortDir(prev => (prev === 'ASC' ? 'DESC' : 'ASC'));
    setPage(1);
  }, []);

  // Unified refresh: refetch columns, detail and records in parallel
  const onRefresh = useCallback(async () => {
    await Promise.all([colsRefetch(), detailRefetch(), recordsRefetch()]);
  }, [colsRefetch, detailRefetch, recordsRefetch]);

  // Editing state and local buffer
  const [isEditing, setIsEditing] = useState<boolean>(false);
  const [localRows, setLocalRows] = useState<DbTableRecord[]>([]);
  const [pendingDeletes, setPendingDeletes] = useState<number[]>([]);
  // Keep draft input values for timestamp fields to allow partial typing
  const [drafts, setDrafts] = useState<Record<string, string>>({});

  // Initialize localRows when records arrive (only populate when not editing).
  // Do not refresh localRows until all parallel requests finish to avoid early cache-driven UI updates.
  useEffect(() => {
    const bootstrapping = colsLoading || detailLoading || isLoading;
    if (!isEditing && !bootstrapping) {
      setLocalRows(records);
    }
  }, [records, isEditing, colsLoading, detailLoading, isLoading]);

  const nonSystemColumns = useMemo(() => columns.filter(c => !c.is_system_field), [columns]);
  const hasDataFields = useMemo(() => nonSystemColumns.length > 0, [nonSystemColumns]);

  // Generate a temp id for unsaved rows (client-only)
  const generateTempId = () => `tmp-${Date.now()}-${Math.floor(Math.random() * 100000)}`;

  const addEmptyRow = () => {
    if (!canCreateRecord) return;
    if (!hasDataFields) return;
    const newRow: DbTableRecord = nonSystemColumns.reduce<DbTableRecord>((acc, col) => {
      let defaultValue: DbTableRecord[keyof DbTableRecord] = null;
      switch (col.type) {
        case Type.Boolean:
          defaultValue = false;
          break;
        case Type.Integer:
        case Type.Numeric:
          // Use empty string in edit mode to trigger required highlight
          defaultValue = '';
          break;
        case Type.Timestamp:
          defaultValue = formatTimestampWallTime(new Date());
          break;
        case Type.Text:
        default:
          defaultValue = '';
      }
      acc[col.name] = defaultValue;
      return acc;
    }, {} as DbTableRecord);
    newRow.__temp_id = generateTempId();
    setLocalRows(prev => [...prev, newRow]);
  };

  const onAddRowFromEmpty = () => {
    if (!canCreateRecord) return;
    setIsEditing(true);
    addEmptyRow();
  };

  const updateLocalCell = (
    rowKey: string | number,
    colName: string,
    value: DbTableRecord[keyof DbTableRecord]
  ) => {
    setLocalRows(prev =>
      prev.map(r =>
        String(r.id ?? r.__temp_id) === String(rowKey) ? { ...r, [colName]: value } : r
      )
    );
  };

  const removeLocalRow = (rowKey: string | number) => {
    setLocalRows(prev => prev.filter(r => String(r.id ?? r.__temp_id) !== String(rowKey)));
  };

  // Build diffs for update/create on save
  const originalById = useMemo(() => {
    const m = new Map<string, DbTableRecord>();
    records.forEach(r => m.set(String(r.id), r));
    return m;
  }, [records]);

  // Validate required fields and normalize timestamp values for save
  const isEmptyValue = (val: DbRecordValue): boolean => {
    if (val === undefined || val === null) return true;
    if (typeof val === 'string') return val.trim() === '';
    if (Array.isArray(val)) return val.length === 0;
    return false;
  };

  const normalizeValueForColumn = (val: DbRecordValue, col: DbTableColumn): DbRecordValue => {
    // Normalize numeric values: empty string -> null (if not required), numeric strings -> numbers
    if (col.type === Type.Integer || col.type === Type.Numeric) {
      if (val === undefined || val === null) return val;
      if (typeof val === 'string') {
        const trimmed = val.trim();
        if (trimmed === '') {
          return col.is_required ? '' : null;
        }
        const num = Number(trimmed);
        return Number.isNaN(num) ? val : num;
      }
      return val;
    }
    // For non-required timestamp fields, empty string should become null
    if (col.type === Type.Timestamp) {
      if (val === undefined || val === null) return col.is_required ? undefined : null;
      if (typeof val === 'string' && val.trim() === '') return col.is_required ? undefined : null;
      if (typeof val === 'string') {
        const formatted = formatTimestampWallTime(val);
        return formatted ? datetimeLocalToWallTime(formatted.replace(' ', 'T')) : val;
      }
      return val;
    }
    return val;
  };

  const onSave = async () => {
    // Orchestrate create, update, delete with a single success/error toast
    // 1) Validate required fields and normalize values
    const errors: string[] = [];
    const sanitizedRows: DbTableRecord[] = localRows.map(r => {
      const next: DbTableRecord = { ...r };
      nonSystemColumns.forEach(col => {
        const raw = r[col.name] as DbRecordValue;
        const normalized = normalizeValueForColumn(raw, col);
        next[col.name] = normalized;
        // Required validation
        if (col.is_required) {
          const v = next[col.name];
          switch (col.type) {
            case Type.Boolean: {
              // false is valid; only undefined/null/empty string is invalid
              const invalid = v === undefined || v === null || (typeof v === 'string' && v === '');
              if (invalid) {
                errors.push(t('dbs.tableData.validation.fieldMissing', { field: col.name }));
              }
              break;
            }
            case Type.Integer:
            case Type.Numeric: {
              const invalid =
                v === undefined || v === null || typeof v !== 'number' || Number.isNaN(v as number);
              if (invalid) {
                errors.push(t('dbs.tableData.validation.fieldMissing', { field: col.name }));
              }
              break;
            }
            case Type.Text: {
              const invalid = typeof v !== 'string' || v.trim().length === 0;
              if (invalid) {
                errors.push(t('dbs.tableData.validation.fieldMissing', { field: col.name }));
              }
              break;
            }
            case Type.Timestamp: {
              if (v === undefined || v === null || (typeof v === 'string' && v.trim() === '')) {
                errors.push(t('dbs.tableData.validation.fieldMissing', { field: col.name }));
                break;
              }
              if (typeof v === 'string') {
                if (!isTimestampValueValid(v)) {
                  errors.push(t('dbs.tableData.validation.timestampInvalid', { field: col.name }));
                }
              }
              break;
            }
            default: {
              if (isEmptyValue(v)) {
                errors.push(t('dbs.tableData.validation.fieldMissing', { field: col.name }));
              }
              break;
            }
          }
        } else {
          // Non-required timestamp: ensure null for empty
          if (col.type === Type.Timestamp) {
            const v = next[col.name];
            if (v === undefined || (typeof v === 'string' && v.trim() === '')) {
              next[col.name] = null;
            }
          }
        }
      });
      return next;
    });

    if (errors.length > 0) {
      toast.error(t('dbs.tableData.validation.saveBlocked'), {
        description: errors.slice(0, 6).join(', '),
      });
      return; // Block save until fields are complete
    }

    // 2) Build create/update payloads from sanitized rows
    // New rows: no id
    const toCreate = sanitizedRows
      .filter(r => !r.id)
      .map(r => {
        const payload: Record<string, unknown> = {};
        nonSystemColumns.forEach(c => {
          payload[c.name] = r[c.name];
        });
        return payload as Omit<DbTableRecord, 'id'>;
      });

    // Updated rows: id exists and fields changed
    const toUpdate = sanitizedRows
      .filter(r => !!r.id)
      .map(r => {
        const original = originalById.get(String(r.id));
        const patch: Partial<DbTableRecord> = {};
        nonSystemColumns.forEach(c => {
          if (!original || original[c.name] !== r[c.name]) {
            patch[c.name] = r[c.name];
          }
        });
        return { id: r.id as number, ...patch };
      })
      .filter(p => Object.keys(p).length > 1); // at least one field other than id

    // Deleted rows: collected ids to remove
    const toDeleteIds = pendingDeletes;
    if (toCreate.length > 0 && !canCreateRecord) {
      toast.error(t('common.unauthorizedDescription'));
      return;
    }
    if (toUpdate.length > 0 && !canUpdateRecord) {
      toast.error(t('common.unauthorizedDescription'));
      return;
    }
    if (toDeleteIds.length > 0 && !canDeleteRecord) {
      toast.error(t('common.unauthorizedDescription'));
      return;
    }
    const didChange = toCreate.length > 0 || toUpdate.length > 0 || toDeleteIds.length > 0;
    try {
      if (toCreate.length > 0) {
        await createRecords(toCreate);
      }
      if (toUpdate.length > 0) {
        await updateRecords(toUpdate);
      }
      if (toDeleteIds.length > 0) {
        await deleteRecords(toDeleteIds);
      }
      if (didChange) {
        toast.success(t('dbs.recordsEditSuccess', { defaultMessage: 'Changes saved' }));
      }
      setIsEditing(false);
      setPendingDeletes([]);
      setDrafts({});
    } catch (err) {
      const message = getErrorMessage(err);
      toast.error(message || t('dbs.failed'));
    }
  };

  const discardChanges = () => {
    setLocalRows(records);
    setIsEditing(false);
    setPendingDeletes([]);
    setDrafts({});
  };

  const onCancel = () => setDiscardConfirmOpen(true);

  const onDeleteRow = async (row: DbTableRecord) => {
    if (!canDeleteRecord) return;
    if (row.id) {
      // Defer deletion: collect id and remove locally only
      setPendingDeletes(prev => {
        const idNum = Number(row.id);
        return prev.includes(idNum) ? prev : [...prev, idNum];
      });
      removeLocalRow(row.id as number);
    } else if ((row as { __temp_id?: string }).__temp_id) {
      // Unsaved temp row – just remove locally
      removeLocalRow((row as { __temp_id?: string }).__temp_id as string);
    }
  };

  // Batch import dialog state (shared between controls and body)
  const [batchImportOpen, setBatchImportOpen] = useState<boolean>(false);
  const [pageSearch, setPageSearch] = useState<string>('');
  const [visibleColumnNames, setVisibleColumnNames] = useState<string[]>([]);
  const [selectedRow, setSelectedRow] = useState<DbTableRecord | null>(null);
  const [rowDetailOpen, setRowDetailOpen] = useState<boolean>(false);

  const loading = colsLoading || detailLoading || isLoading;

  useEffect(() => {
    if (columns.length === 0) return;
    setVisibleColumnNames(prev => {
      const names = columns.map(col => col.name);
      const defaultNames = columns.some(col => !col.is_system_field)
        ? columns.filter(col => !col.is_system_field).map(col => col.name)
        : names;
      if (prev.length === 0) return defaultNames;
      const next = prev.filter(name => names.includes(name));
      const added = defaultNames.filter(name => !prev.includes(name));
      const normalized = [...next, ...added];
      if (
        normalized.length === prev.length &&
        normalized.every((name, index) => name === prev[index])
      ) {
        return prev;
      }
      return normalized;
    });
  }, [columns]);

  const visibleColumns = useMemo(() => {
    if (visibleColumnNames.length === 0) return columns;
    const visibleNameSet = new Set(visibleColumnNames);
    const next = columns.filter(col => visibleNameSet.has(col.name));
    return next.length > 0 ? next : columns;
  }, [columns, visibleColumnNames]);

  const stickyColumnNames = useMemo(() => {
    const candidates = [
      'patient_name',
      'inpatient_no',
      'admission_no',
      'medical_record_no',
      'column_1',
      'id',
    ];
    const names = visibleColumns.map(col => col.name);
    const first = candidates.find(name => names.includes(name)) ?? names[0];
    return first ? [first] : [];
  }, [visibleColumns]);

  const filterRows = useCallback(
    (rows: readonly DbTableRecord[]) => {
      const query = pageSearch.trim().toLowerCase();
      if (!query) return rows;
      return rows.filter(row =>
        columns.some(col => {
          const value = row[col.name];
          if (value === null || value === undefined) return false;
          return String(Array.isArray(value) ? value.join(', ') : value)
            .toLowerCase()
            .includes(query);
        })
      );
    },
    [columns, pageSearch]
  );

  const displayedRecords = useMemo(() => filterRows(records), [filterRows, records]);

  // Ref for table container to measure visible width for sticky empty state
  const [containerWidth, setContainerWidth] = useState<number>(0);
  const tableContainerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (tableContainerRef.current) {
      setContainerWidth(tableContainerRef.current.clientWidth);
      const observer = new ResizeObserver(entries => {
        if (entries[0]) {
          setContainerWidth(entries[0].contentRect.width);
        }
      });
      observer.observe(tableContainerRef.current);
      return () => observer.disconnect();
    }
  }, []);

  return (
    <div className="space-y-4">
      <h1 className="text-base font-semibold">{t('dbs.tableData.title')}</h1>
      <SchemaHealthNotice
        columns={columns}
        compact
      />
      {/* Controls: page size, sort key, sort dir, edit actions */}
      <TableDataControls
        dbId={dbId}
        tableId={tableId}
        pageSize={pageSize}
        normalizedSortKey={normalizedSortKey}
        columns={columns}
        sortDir={sortDir}
        isEditing={isEditing}
        isCreating={isCreating}
        isUpdating={isUpdating}
        isDeleting={isDeleting}
        hasDataFields={hasDataFields}
        onPageSizeChange={onPageSizeChange}
        onSortKeyChange={onSortKeyChange}
        onToggleSortDir={onToggleSortDir}
        onAddRow={addEmptyRow}
        onCancel={onCancel}
        onSave={onSave}
        onStartEdit={() => setIsEditing(true)}
        onRefresh={onRefresh}
        canEditData={canEditData}
        canManage={canManageSchema}
        canBatchImport={canBatchImport}
        canSmartIngest={canSmartIngest}
        onBatchImport={() => {
          if (canBatchImport) setBatchImportOpen(true);
        }}
        pageSearch={pageSearch}
        onPageSearchChange={value => {
          setPageSearch(value);
          setPage(1);
        }}
        visibleColumnNames={
          visibleColumnNames.length > 0 ? visibleColumnNames : columns.map(c => c.name)
        }
        onVisibleColumnNamesChange={setVisibleColumnNames}
      />
      <div ref={tableContainerRef} className="overflow-x-auto border rounded-md">
        <Table containerClassName="w-full text-xs" className="border-collapse">
          <TableDataHeader
            columns={visibleColumns}
            isEditing={isEditing}
            actionsTitle={t('dbs.tableData.actions')}
            showRowActions={!isEditing}
            stickyColumnNames={stickyColumnNames}
          />
          <TableDataBody
            loading={loading}
            pageSize={pageSize}
            columns={visibleColumns}
            isEditing={isEditing}
            localRows={localRows}
            records={displayedRecords}
            onDeleteRow={onDeleteRow}
            updateLocalCell={updateLocalCell}
            drafts={drafts}
            setDrafts={setDrafts}
            onAddRow={onAddRowFromEmpty}
            onBatchImport={() => {
              if (canBatchImport) setBatchImportOpen(true);
            }}
            hasDataFields={hasDataFields}
            manageStructureHref={`/console/db/${dbId}/table/${tableId}/structure`}
            smartCreateHref={`/console/db/${dbId}/table/${tableId}/create`}
            smartIngestHref={`/console/db/${dbId}/table/${tableId}/data`}
            canEditData={canEditData}
            canManage={canManageSchema}
            canBatchImport={canBatchImport}
            canSmartIngest={canSmartIngest}
            canDeleteData={canDeleteRecord}
            containerWidth={containerWidth}
            stickyColumnNames={stickyColumnNames}
            onOpenRow={row => {
              setSelectedRow(row);
              setRowDetailOpen(true);
            }}
          />
        </Table>
      </div>

      {!isEditing && (
        <Pagination
          currentPage={page}
          totalPages={totalPages}
          total={total}
          pageSize={pageSize}
          onPageChange={p => {
            const next = Math.max(1, Math.min(totalPages, p));
            setPage(next);
          }}
        />
      )}

      {/* Batch Import Dialog - shared between controls and empty state */}
      <BatchImportDialog
        open={batchImportOpen}
        onOpenChange={setBatchImportOpen}
        dbId={dbId}
        tableId={tableId}
        onSuccess={onRefresh}
      />
      <RowDetailDialog
        open={rowDetailOpen}
        onOpenChange={setRowDetailOpen}
        row={selectedRow}
        columns={columns}
      />
      <ConfirmDialog
        open={discardConfirmOpen}
        onOpenChange={setDiscardConfirmOpen}
        title={t('dbs.tableData.discardConfirmTitle')}
        description={t('dbs.tableData.discardConfirmDescription')}
        confirmText={t('dbs.tableData.discardConfirmAction')}
        cancelText={t('common.cancel')}
        variant="warning"
        onConfirm={discardChanges}
      />
    </div>
  );
};

export default TableData;
