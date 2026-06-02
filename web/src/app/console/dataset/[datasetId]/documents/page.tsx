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
  useDeleteDocument,
  useBulkDeleteDocuments,
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
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';

// New: data source union shared with child components
export type DataSourceType = 'file' | 'notion' | 'web' | 'api';

const TERMINAL_STATUSES = new Set(['completed', 'error', 'paused']);

export default function DatasetDocumentsPage() {
  const t = useT();
  const params = useParams();
  const datasetId = params.datasetId as string;

  /* -------------------------------- state -------------------------------- */
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Delete confirmation state
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [documentToDelete, setDocumentToDelete] = useState<Document | null>(null);

  // Delete document mutation
  const deleteDocumentMutation = useDeleteDocument();
  // Batch delete confirmation state
  const [batchDeleteDialogOpen, setBatchDeleteDialogOpen] = useState(false);
  const [isBatchDeleting, setIsBatchDeleting] = useState(false);
  const bulkDeleteMutation = useBulkDeleteDocuments(datasetId);
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

  // Detect in-progress documents and toggle polling
  useEffect(() => {
    const hasInProgress = (documents ?? []).some(
      doc => doc.indexing_status && !TERMINAL_STATUSES.has(doc.indexing_status)
    );
    setPollingEnabled(hasInProgress);
  }, [documents]);

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
  const handleDeleteDocument = useCallback(async (document: Document) => {
    setDocumentToDelete(document);
    setDeleteDialogOpen(true);
  }, []);

  const confirmDeleteDocument = useCallback(async () => {
    if (!documentToDelete) return;

    try {
      await deleteDocumentMutation.mutateAsync({ datasetId, documentId: documentToDelete.id });
      // React Query invalidation will refetch; ensure UI sync
      await refetch();
      setDeleteDialogOpen(false);
      setDocumentToDelete(null);
    } catch (error) {
      // Error handling is already done in the mutation hook
      console.error('Delete document failed:', error);
    }
  }, [documentToDelete, datasetId, deleteDocumentMutation, refetch]);

  const downloadMutation = useDownloadDocument();

  const handleDocumentAction = useCallback(
    async (document: Document, action: 'delete' | 'download' | 'reprocess') => {
      try {
        switch (action) {
          case 'delete':
            handleDeleteDocument(document);
            break;
          case 'download':
            if (document.data_source_info?.upload_file_id) {
              await downloadMutation.mutateAsync({
                fileId: document.data_source_info.upload_file_id,
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
    [t, handleDeleteDocument, refetch, downloadMutation]
  );

  // Refresh entire documents list
  const handleRefresh = useCallback(async () => {
    await refetch();
    if (canEdit) {
      await refetchErrorDocs();
    }
  }, [refetch, refetchErrorDocs, canEdit]);

  // Confirm batch delete
  const confirmBatchDelete = useCallback(async () => {
    if (!datasetId || selectedIds.length === 0) return;
    try {
      setIsBatchDeleting(true);
      await bulkDeleteMutation.mutateAsync({ documentIds: selectedIds });
      setSelectedIds([]);
      await refetch();
      setBatchDeleteDialogOpen(false);
    } catch (error) {
      toast.error(t('datasets.messages.actionFailed'));
      console.error('batch delete failed', error);
    } finally {
      setIsBatchDeleting(false);
    }
  }, [datasetId, selectedIds, refetch, t, bulkDeleteMutation]);

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
              disabled={isFetching}
            >
              <RefreshCcw
                className={`h-4 w-4 transition-transform duration-500 ${
                  isFetching ? 'animate-spin' : ''
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

      {isLoading && <DocumentListSkeleton />}

      {!isLoading && visibleDocuments.length > 0 && (
        <DocumentTable
          documents={visibleDocuments}
          datasetId={datasetId}
          onDelete={doc => handleDocumentAction(doc, 'delete')}
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

      {/* Delete confirmation dialog */}
      <ConfirmDialog
        variant="danger"
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        title={t('datasets.messages.confirmDelete', { name: documentToDelete?.name || '' })}
        description={t('datasets.messages.confirmDeleteDescription')}
        confirmText={t('datasets.actions.delete')}
        cancelText={t('datasets.actions.cancel')}
        onConfirm={confirmDeleteDocument}
        loading={deleteDocumentMutation.isPending}
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
            <Button
              variant="destructive"
              onClick={() => setBatchDeleteDialogOpen(true)}
              disabled={isMutatingEnabled.length > 0 || isBatchDeleting}
            >
              {t('datasets.actions.delete')}
            </Button>
          </div>
        </div>
      )}

      {/* Batch delete confirmation dialog */}
      <ConfirmDialog
        variant="danger"
        open={batchDeleteDialogOpen}
        onOpenChange={setBatchDeleteDialogOpen}
        title={t('datasets.messages.batchDeleteDocuments', { count: selectedIds.length })}
        description={t('datasets.messages.batchDeleteDescription')}
        confirmText={t('datasets.actions.delete')}
        cancelText={t('datasets.actions.cancel')}
        onConfirm={confirmBatchDelete}
        loading={isBatchDeleting}
      />

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
