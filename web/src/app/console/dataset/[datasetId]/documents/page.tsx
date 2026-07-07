'use client';

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useParams } from 'next/navigation';
import { Plus, RefreshCcw, Search } from 'lucide-react';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import { DatasetFileAssetDialog } from '@/components/datasets/document/dataset-file-asset-dialog';
import { DatasetFileRefPanel } from '@/components/datasets/document/dataset-file-ref-panel';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useDataset } from '@/hooks/dataset/use-datasets';
import { useBulkDisableDocuments, useBulkEnableDocuments } from '@/hooks/dataset/use-documents';
import {
  useDatasetFileRefs,
  useDeleteDatasetFileRef,
  useRetryDatasetFileRefSync,
} from '@/hooks/dataset/use-dataset-file-refs';
import type { DatasetFileRef } from '@/services/types/dataset';
import { KNOWLEDGE_BASE_PERMISSION_ACTIONS } from '@/constants/permissions';
import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';

export default function DatasetDocumentsPage() {
  const t = useT();
  const params = useParams();
  const datasetId = params.datasetId as string;

  // Permission checking - use new permission system
  const {
    hasAnyPermission,
    hasWorkspaceAccess,
    isLoading: isPermissionsLoading,
  } = useAccountPermissions();
  const canViewDocuments = hasAnyPermission([
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentView,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentCreate,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentUpdate,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentDelete,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.indexManage,
  ]);
  const { data: datasetData } = useDataset(datasetId, { enabled: canViewDocuments });
  const isExternalDataSource = !!datasetData?.data?.external_knowledge_info?.external_knowledge_id;
  const canCreateDocument = hasAnyPermission(KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentCreate);
  const canUpdateDocument = hasAnyPermission(KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentUpdate);
  const canDeleteDocument = hasAnyPermission(KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentDelete);
  const canManageIndex = hasAnyPermission(KNOWLEDGE_BASE_PERMISSION_ACTIONS.indexManage);
  const canOpenSourceFile = hasWorkspaceAccess();
  const canEdit = canCreateDocument || canUpdateDocument || canDeleteDocument || canManageIndex;

  const [fileSelectorOpen, setFileSelectorOpen] = useState(false);
  const [fileRefPollingEnabled, setFileRefPollingEnabled] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [refToRemove, setRefToRemove] = useState<DatasetFileRef | null>(null);
  const [togglingRefId, setTogglingRefId] = useState<string>();

  const {
    refs: fileRefs,
    refetch: refetchFileRefs,
    isFetching: isFetchingFileRefs,
  } = useDatasetFileRefs(
    datasetId,
    { limit: 100 },
    {
      refetchInterval: fileRefPollingEnabled ? 5000 : false,
      enabled: canViewDocuments,
    }
  );
  const retryFileRefMutation = useRetryDatasetFileRefSync(datasetId);
  const deleteFileRefMutation = useDeleteDatasetFileRef(datasetId);
  const bulkEnableMutation = useBulkEnableDocuments(datasetId);
  const bulkDisableMutation = useBulkDisableDocuments(datasetId);

  useEffect(() => {
    const hasSyncInProgress = (fileRefs ?? []).some(ref =>
      ['pending', 'syncing'].includes(ref.sync_status)
    );
    setFileRefPollingEnabled(hasSyncInProgress);
  }, [fileRefs]);

  const filteredRefs = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();
    if (!normalizedKeyword) return fileRefs;
    return fileRefs.filter(ref => ref.file_name.toLowerCase().includes(normalizedKeyword));
  }, [fileRefs, keyword]);

  const stats = useMemo(() => {
    const synced = fileRefs.filter(ref => ref.sync_status === 'synced');
    return {
      total: fileRefs.length,
      enabled: synced.filter(ref => ref.dataset_document_enabled).length,
      ready: fileRefs.filter(ref => ref.processing_status === 'ready').length,
    };
  }, [fileRefs]);

  const handleRefresh = useCallback(async () => {
    await refetchFileRefs();
  }, [refetchFileRefs]);

  const handleRetryFileRef = useCallback(
    async (ref: DatasetFileRef) => {
      if (!canManageIndex) return;
      await retryFileRefMutation.mutateAsync(ref.id);
      await refetchFileRefs();
    },
    [canManageIndex, retryFileRefMutation, refetchFileRefs]
  );

  const handleToggleEnabled = useCallback(
    async (ref: DatasetFileRef, enabled: boolean) => {
      if (!canUpdateDocument) return;
      if (!ref.dataset_document_id) return;
      try {
        setTogglingRefId(ref.id);
        if (enabled) {
          await bulkEnableMutation.mutateAsync({ documentIds: [ref.dataset_document_id] });
        } else {
          await bulkDisableMutation.mutateAsync({ documentIds: [ref.dataset_document_id] });
        }
        await refetchFileRefs();
      } catch (error) {
        toast.error(t('datasets.messages.actionFailed'));
        console.error('toggle dataset file enabled failed', error);
      } finally {
        setTogglingRefId(undefined);
      }
    },
    [bulkDisableMutation, bulkEnableMutation, canUpdateDocument, refetchFileRefs, t]
  );

  const confirmRemoveFileRef = useCallback(async () => {
    if (!canDeleteDocument) return;
    if (!refToRemove) return;
    await deleteFileRefMutation.mutateAsync(refToRemove.id);
    setRefToRemove(null);
    await refetchFileRefs();
  }, [canDeleteDocument, deleteFileRefMutation, refToRemove, refetchFileRefs]);

  if (isPermissionsLoading) {
    return <PermissionLoadingState />;
  }

  if (!canViewDocuments) {
    return <PermissionDeniedState />;
  }

  return (
    <div className="min-h-full bg-background">
      <section className="flex min-h-[88px] flex-wrap items-center justify-between gap-4 border-b px-6 py-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-lg font-semibold text-foreground">
            <span>{t('datasets.documents.fileRefs.count', { count: stats.total })}</span>
            <Button
              isIcon
              variant="ghost"
              className="h-8 w-8 text-muted-foreground"
              onClick={handleRefresh}
              disabled={isFetchingFileRefs}
              aria-label={t('datasets.documents.fileRefs.refresh')}
            >
              <RefreshCcw className={`h-4 w-4 ${isFetchingFileRefs ? 'animate-spin' : ''}`} />
            </Button>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('datasets.documents.fileRefs.enabledSummary', {
              enabled: stats.enabled,
              ready: stats.ready,
            })}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <div className="relative w-[320px] max-w-full">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="search"
              value={keyword}
              onChange={event => setKeyword(event.target.value)}
              placeholder={t('datasets.documents.fileRefs.searchPlaceholder')}
              className="h-10 rounded-lg pl-10"
            />
          </div>
          {!isExternalDataSource && canCreateDocument ? (
            <Button className="h-10 rounded-lg px-4" onClick={() => setFileSelectorOpen(true)}>
              <Plus className="h-4 w-4" />
              {t('datasets.documents.fileRefs.addFile')}
            </Button>
          ) : null}
        </div>
      </section>

      <div className="p-6">
        <DatasetFileRefPanel
          refs={filteredRefs}
          canEdit={canEdit}
          canOpenSourceFile={canOpenSourceFile}
          canToggleEnabled={canUpdateDocument}
          canRetry={canManageIndex}
          canRemove={canDeleteDocument}
          retryingRefId={
            retryFileRefMutation.isPending ? retryFileRefMutation.variables : undefined
          }
          removingRefId={
            deleteFileRefMutation.isPending ? deleteFileRefMutation.variables : undefined
          }
          togglingRefId={togglingRefId}
          onRetry={handleRetryFileRef}
          onRemove={setRefToRemove}
          onToggleEnabled={handleToggleEnabled}
        />
      </div>

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
        confirmText={t('datasets.documents.fileRefs.removeConfirm')}
        cancelText={t('datasets.actions.cancel')}
        onConfirm={confirmRemoveFileRef}
        loading={deleteFileRefMutation.isPending}
      />

      <DatasetFileAssetDialog
        datasetId={datasetId}
        open={fileSelectorOpen}
        onOpenChange={setFileSelectorOpen}
        onSubmitted={refetchFileRefs}
      />
    </div>
  );
}
