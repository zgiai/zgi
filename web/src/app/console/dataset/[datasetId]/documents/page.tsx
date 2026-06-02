'use client';

import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { useParams } from 'next/navigation';
import { useT } from '@/i18n';
import { Plus, RefreshCcw, Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { toast } from 'sonner';
import { useDataset } from '@/hooks/dataset/use-datasets';
import {
  useDocuments,
  useBulkEnableDocuments,
  useBulkDisableDocuments,
  useDownloadDocument,
} from '@/hooks/dataset/use-documents';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import type { Document, DocumentIndexingStatus, DocumentStatus } from '@/services/types/dataset';
import { DocumentTable } from '@/components/datasets/document/document-table';
import { DocumentListSkeleton } from '@/components/datasets/document/document-list-skeleton';
// removed mode selection dialog
import { IndexFailedBanner } from '@/components/datasets/document/index-failed';
import { useErrorDocs, useRetryErrorDocs } from '@/hooks/dataset/use-error-docs';
import { DocumentEmptyState } from '@/components/datasets/document/document-empty-state';
import { DatasetFileAssetDialog } from '@/components/datasets/document/dataset-file-asset-dialog';
import { DatasetFileRefPanel } from '@/components/datasets/document/dataset-file-ref-panel';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import {
  useDatasetFileRefs,
  useDeleteDatasetFileRef,
  useRetryDatasetFileRefSync,
} from '@/hooks/dataset/use-dataset-file-refs';
import type { DatasetFileRef } from '@/services/types/dataset';

// New: data source union shared with child components
export type DataSourceType = 'file' | 'notion' | 'web' | 'api';

const TERMINAL_STATUSES = new Set(['completed', 'error', 'paused']);

export default function DatasetDocumentsPage() {
  const t = useT();
  const params = useParams();
  const datasetId = params.datasetId as string;

  /* -------------------------------- state -------------------------------- */
  const sentinelRef = useRef<HTMLDivElement>(null);

  const [refToRemove, setRefToRemove] = useState<DatasetFileRef | null>(null);

  const bulkEnableMutation = useBulkEnableDocuments(datasetId);
  const bulkDisableMutation = useBulkDisableDocuments(datasetId);

  /* ------------------------------- dataset ------------------------------- */
  const { data: datasetData } = useDataset(datasetId);
  const isExternalDataSource = !!datasetData?.data?.external_knowledge_info?.external_knowledge_id;

  // Permission checking - use new permission system
  const { hasPermission } = useAccountPermissions();
  const canEdit = hasPermission('knowledge_base.manage');

  /* ------------------------------ documents ------------------------------ */
  const [sortField, setSortField] = useState<
    'created_at' | 'updated_at' | '-created_at' | '-updated_at'
  >('created_at');
  const [statusFilter, setStatusFilter] = useState<
    DocumentIndexingStatus[keyof DocumentIndexingStatus] | 'all'
  >('all');
  // Map UI filter values to backend enum values for indexing_status
  const indexingStatusParam = statusFilter === 'all' ? undefined : statusFilter;

  // Conditional polling: auto-refresh when documents are still processing
  const [pollingEnabled, setPollingEnabled] = useState(false);
  const [fileRefPollingEnabled, setFileRefPollingEnabled] = useState(false);

  const {
    documents,
    total,
    isLoading,
    refetch,
    isFetching,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    setSearchKeyword,
    searchKeyword,
  } = useDocuments(
    datasetId,
    { limit: 20, sort: sortField, indexing_status: indexingStatusParam },
    { debounceDelay: 500, refetchInterval: pollingEnabled ? 5000 : false }
  );

  const {
    refs: fileRefs,
    refetch: refetchFileRefs,
    isFetching: isFetchingFileRefs,
  } = useDatasetFileRefs(
    datasetId,
    { limit: 100 },
    {
      refetchInterval: pollingEnabled || fileRefPollingEnabled ? 5000 : false,
      enabled: true,
    }
  );
  const retryFileRefMutation = useRetryDatasetFileRefSync(datasetId);
  const deleteFileRefMutation = useDeleteDatasetFileRef(datasetId);

  // Detect in-progress documents and toggle polling
  useEffect(() => {
    const hasInProgress = (documents ?? []).some(
      doc => doc.indexing_status && !TERMINAL_STATUSES.has(doc.indexing_status)
    );
    setPollingEnabled(hasInProgress);
  }, [documents]);

  useEffect(() => {
    const hasSyncInProgress = (fileRefs ?? []).some(ref =>
      ['pending', 'syncing'].includes(ref.sync_status)
    );
    setFileRefPollingEnabled(hasSyncInProgress);
  }, [fileRefs]);

  // Normalize status for display compatibility
  const visibleDocuments = useMemo(() => {
    return (documents ?? []).map(doc => ({
      ...doc,
      status: (doc.status ??
        doc.display_status ??
        doc.indexing_status ??
        'pending') as DocumentStatus,
    }));
  }, [documents]);

  /* -------------------------- selection & sorting -------------------------- */
  const [selectedIds, setSelectedIds] = useState<string[]>([]);

  // useDocuments depends on sort & status via params above; no extra effect required

  /* --------------------------- error docs banner -------------------------- */
  // Only fetch error docs when user has edit permission
  const {
    data: errorDocsRes,
    isLoading: isErrorDocsLoading,
    refetch: refetchErrorDocs,
  } = useErrorDocs(datasetId, { enabled: canEdit });
  const retryFailedDocsMutation = useRetryErrorDocs(datasetId);
  const failedDocIds = useMemo(
    () => (errorDocsRes?.data?.data ?? []).map(d => d.id),
    [errorDocsRes?.data?.data]
  );
  const failedTotal = errorDocsRes?.data?.total ?? 0;
  const handleRetryFailedDocs = useCallback(async () => {
    try {
      await retryFailedDocsMutation.mutateAsync({ documentIds: failedDocIds });
      await refetch();
    } catch (_e) {
      // Error toast handled in mutation
    }
  }, [retryFailedDocsMutation, failedDocIds, refetch]);

  /* -------------------------- infinite scroll --------------------------- */
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el) return;

    const observer = new IntersectionObserver(
      entries => {
        if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
          fetchNextPage();
        }
      },
      { rootMargin: '200px' }
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  /* ---------------------------- doc actions ----------------------------- */
  const downloadMutation = useDownloadDocument();

  const handleDocumentAction = useCallback(
    async (document: Document, action: 'download' | 'reprocess') => {
      try {
        switch (action) {
          case 'download':
            const sourceFileId =
              document.data_source_info?.upload_file_id ||
              document.data_source_info?.source_file_id ||
              document.file_id;
            if (sourceFileId) {
              await downloadMutation.mutateAsync({
                fileId: sourceFileId,
                filename: document.name,
              });
            } else {
              toast.error(t('datasets.messages.actionFailed'));
            }
            break;
          case 'reprocess':
            toast(t('datasets.messages.reprocessSuccess'));
            // Refetch to refresh list after reprocess
            await refetch();
            break;
        }
      } catch (error) {
        console.error(`Action ${action} failed:`, error);
        toast.error(t('datasets.messages.actionFailed'));
      }
    },
    [t, refetch, downloadMutation]
  );

  // Refresh entire documents list
  const handleRefresh = useCallback(async () => {
    await Promise.all([refetch(), refetchFileRefs()]);
    if (canEdit) {
      await refetchErrorDocs();
    }
  }, [refetch, refetchFileRefs, refetchErrorDocs, canEdit]);

  const handleRetryFileRef = useCallback(
    async (ref: DatasetFileRef) => {
      await retryFileRefMutation.mutateAsync(ref.id);
      await Promise.all([refetch(), refetchFileRefs()]);
    },
    [retryFileRefMutation, refetch, refetchFileRefs]
  );

  const handleRemoveFileRef = useCallback((ref: DatasetFileRef) => {
    setRefToRemove(ref);
  }, []);

  const confirmRemoveFileRef = useCallback(async () => {
    if (!refToRemove) return;
    await deleteFileRefMutation.mutateAsync(refToRemove.id);
    setRefToRemove(null);
    await Promise.all([refetch(), refetchFileRefs()]);
  }, [deleteFileRefMutation, refToRemove, refetch, refetchFileRefs]);

  // Toggle selection
  const handleToggleSelect = useCallback((id: string) => {
    setSelectedIds(prev => (prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]));
  }, []);

  const handleToggleSelectAll = useCallback((checked: boolean, idsOnPage: string[]) => {
    setSelectedIds(prev =>
      checked
        ? Array.from(new Set([...prev, ...idsOnPage]))
        : prev.filter(id => !idsOnPage.includes(id))
    );
  }, []);

  // Sorting change from table header
  const handleSortChange = useCallback(
    (field: 'created_at' | 'updated_at' | '-created_at' | '-updated_at') => {
      setSortField(field);
    },
    []
  );

  // Status filter change
  const handleStatusFilterChange = useCallback(
    (value: DocumentIndexingStatus[keyof DocumentIndexingStatus] | 'all') => {
      setStatusFilter(value);
    },
    []
  );

  // Enabled toggle (single) using bulk hooks for optimistic updates
  const [isMutatingEnabled, setIsMutatingEnabled] = useState<string[]>([]);
  const handleToggleEnabled = useCallback(
    async (documentId: string, enabled: boolean) => {
      if (!datasetId) return;
      try {
        setIsMutatingEnabled(prev => [...prev, documentId]);
        if (enabled) {
          await bulkEnableMutation.mutateAsync({ documentIds: [documentId] });
        } else {
          await bulkDisableMutation.mutateAsync({ documentIds: [documentId] });
        }
      } catch (error) {
        toast.error(t('datasets.messages.actionFailed'));
        console.error('toggle enabled failed', error);
      } finally {
        setIsMutatingEnabled(prev => prev.filter(id => id !== documentId));
      }
    },
    [datasetId, t, bulkEnableMutation, bulkDisableMutation]
  );

  // Batch enable/disable
  const handleBatchEnable = useCallback(async () => {
    if (!datasetId || selectedIds.length === 0) return;
    try {
      setIsMutatingEnabled(prev => [...prev, ...selectedIds]);
      await bulkEnableMutation.mutateAsync({ documentIds: selectedIds });
      setSelectedIds([]);
      await refetch();
    } catch (error) {
      toast.error(t('datasets.messages.actionFailed'));
      console.error('batch enable failed', error);
    } finally {
      setIsMutatingEnabled(prev => prev.filter(id => !selectedIds.includes(id)));
    }
  }, [datasetId, selectedIds, refetch, t, bulkEnableMutation]);

  const handleBatchDisable = useCallback(async () => {
    if (!datasetId || selectedIds.length === 0) return;
    try {
      setIsMutatingEnabled(prev => [...prev, ...selectedIds]);
      await bulkDisableMutation.mutateAsync({ documentIds: selectedIds });
      setSelectedIds([]);
      await refetch();
    } catch (error) {
      toast.error(t('datasets.messages.actionFailed'));
      console.error('batch disable failed', error);
    } finally {
      setIsMutatingEnabled(prev => prev.filter(id => !selectedIds.includes(id)));
    }
  }, [datasetId, selectedIds, refetch, t, bulkDisableMutation]);

  // Add Document directly opens file asset selector
  const openAddDialog = useCallback(() => setFileSelectorOpen(true), []);

  // File asset selector dialog state
  const [fileSelectorOpen, setFileSelectorOpen] = useState(false);

  /* ---------------------------------------------------------------------- */
  return (
    <div className="space-y-6 p-6">
      {/* ------------------------------ header ------------------------------ */}
      <div className="flex flex-col gap-4">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            <p className="text-muted-foreground line-clamp-1 shrink-0">
              {total > 0
                ? t('datasets.messages.documentsCount', { count: total })
                : t('datasets.messages.noDocumentsYet')}
            </p>
            <Button
              isIcon
              variant="ghost"
              className="h-7 w-7 transition-all duration-200 hover:scale-105 active:scale-95"
              onClick={handleRefresh}
              disabled={isFetching || isFetchingFileRefs}
            >
              <RefreshCcw
                className={`h-4 w-4 transition-transform duration-500 ${
                  isFetching || isFetchingFileRefs ? 'animate-spin' : ''
                }`}
              />
            </Button>
            {/* Only show error banner when user has edit permission */}
            {canEdit && !isErrorDocsLoading && failedTotal > 0 && (
              <IndexFailedBanner
                count={failedTotal}
                onRetry={handleRetryFailedDocs}
                retrying={retryFailedDocsMutation.isPending}
              />
            )}
          </div>

          <div className="flex items-center gap-2">
            <div className="flex items-center gap-2">
              <div className="relative flex-1 max-w-sm">
                <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-muted-foreground h-4 w-4" />
                <Input
                  placeholder={t('datasets.search.placeholder')}
                  value={searchKeyword}
                  onChange={e => setSearchKeyword(e.target.value)}
                  className="pl-10 h-9"
                />
              </div>
            </div>

            {/* Only show Add Document button when user has edit permission and not external data source */}
            {!isExternalDataSource && canEdit && (
              <Button onClick={openAddDialog}>
                <Plus className="h-4 w-4 mr-1" />
                {t('datasets.documents.addDocument')}
              </Button>
            )}
          </div>
        </div>
      </div>

      <DatasetFileRefPanel
        refs={fileRefs}
        documents={visibleDocuments}
        canEdit={canEdit}
        retryingRefId={
          retryFileRefMutation.isPending ? retryFileRefMutation.variables : undefined
        }
        removingRefId={
          deleteFileRefMutation.isPending ? deleteFileRefMutation.variables : undefined
        }
        onRetry={handleRetryFileRef}
        onRemove={handleRemoveFileRef}
      />

      {isLoading && <DocumentListSkeleton />}

      {!isLoading && visibleDocuments.length > 0 && (
        <DocumentTable
          documents={visibleDocuments}
          datasetId={datasetId}
          onDownload={doc => handleDocumentAction(doc, 'download')}
          onReprocess={doc => handleDocumentAction(doc, 'reprocess')}
          selectedIds={selectedIds}
          onToggleSelect={handleToggleSelect}
          onToggleSelectAll={handleToggleSelectAll}
          sortField={sortField}
          onSortChange={handleSortChange}
          statusFilter={statusFilter}
          onStatusFilterChange={handleStatusFilterChange}
          onToggleEnabled={handleToggleEnabled}
          isMutatingEnabled={isMutatingEnabled}
          canEdit={canEdit}
        />
      )}

      {!isLoading && visibleDocuments.length === 0 && (
        <DocumentEmptyState
          onCreateDocument={canEdit ? openAddDialog : undefined}
          canEdit={canEdit}
        />
      )}

      {/* ---------------------- infinite scroll sentinel -------------------- */}
      <div ref={sentinelRef} className="h-10" />

      {isFetchingNextPage && <DocumentListSkeleton rows={3} />}

      <ConfirmDialog
        variant="warning"
        open={!!refToRemove}
        onOpenChange={open => {
          if (!open) setRefToRemove(null);
        }}
        title={t('datasets.documents.fileRefs.confirmRemoveTitle', {
          name: refToRemove?.file_name || '',
        })}
        description={t('datasets.documents.fileRefs.confirmRemoveDescription')}
        confirmText={t('datasets.actions.delete')}
        cancelText={t('datasets.actions.cancel')}
        onConfirm={confirmRemoveFileRef}
        loading={deleteFileRefMutation.isPending}
      />

      {/* Bottom floating batch actions toolbar - only show when user has edit permission */}
      {selectedIds.length > 0 && canEdit && (
        <div className="absolute bottom-[100px] left-1/2 -translate-x-1/2 z-50">
          <div className="rounded-lg border bg-background shadow-lg px-4 py-2 flex items-center gap-2">
            <div>
              <span className="text-sm text-muted-foreground">
                {t('datasets.documents.selectedCount', { count: selectedIds.length, total })}
              </span>
            </div>
            <Button
              variant="secondary"
              onClick={handleBatchEnable}
              disabled={isMutatingEnabled.length > 0}
            >
              {t('datasets.actions.enable')}
            </Button>
            <Button
              variant="secondary"
              onClick={handleBatchDisable}
              disabled={isMutatingEnabled.length > 0}
            >
              {t('datasets.actions.disable')}
            </Button>
          </div>
        </div>
      )}

      {/* Add file assets to dataset */}
      <DatasetFileAssetDialog
        datasetId={datasetId}
        open={fileSelectorOpen}
        onOpenChange={setFileSelectorOpen}
        onSubmitted={() => refetch()}
      />
    </div>
  );
}
