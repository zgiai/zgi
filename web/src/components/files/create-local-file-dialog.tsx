'use client';

import { useState, useCallback, useRef, useEffect } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { FileUpload, type FileUploadRef } from '@/components/common/file-upload';
import { toast } from 'sonner';
import { FolderOpen, RefreshCw } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { FILES_QUERY_KEY, useFileFolders } from '@/hooks/use-files';
import { useUploadConfig } from '@/hooks/use-upload';
import { Label } from '@/components/ui/label';
import { RadioCard, RadioCardGroup } from '@/components/ui/radio-card';
import { Skeleton } from '@/components/ui/skeleton';
import {
  WorkspaceSelector,
  type WorkspaceSelectorValue,
} from '@/components/common/workspace-selector';
import { useCurrentWorkspace, useIsOrganizationMode } from '@/store';
import { FolderTreeNode } from './folder-tree-node';
import { cn } from '@/lib/utils';
import type { FileParseProviderKey, FileUploadProcessingMode } from '@/services/types/file';
import {
  MAX_FILE_FOLDER_TREE_LEVEL,
  getFileFolderAncestorIds,
  getFileFolderAncestorIdsByRequest,
} from './file-folder-levels';

export interface CreateLocalFileDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  folderId?: string;
  /** Allowed extensions like ['.jpg', '.png'] (case-insensitive) */
  acceptExt?: string[];
  workspaceId?: string;
  processingMode?: FileUploadProcessingMode;
  parseProvider?: FileParseProviderKey;
  /** Callback after successful upload */
  onUploadComplete?: () => void;
}

/**
 * File Upload Inline Dialog Component
 * A dialog for uploading files directly without navigation
 */
const CreateLocalFileDialog = ({
  open,
  onOpenChange,
  folderId,
  acceptExt = [],
  workspaceId,
  processingMode = 'process_now',
  onUploadComplete,
}: CreateLocalFileDialogProps) => {
  const t = useT();
  const queryClient = useQueryClient();
  const currentWorkspace = useCurrentWorkspace();
  const isOrganizationMode = useIsOrganizationMode();
  const fileUploadRef = useRef<FileUploadRef>(null);
  const cancelUploadRequestedRef = useRef(false);
  const [uploadFilesCount, setUploadFilesCount] = useState(0);
  const [failedUploadFilesCount, setFailedUploadFilesCount] = useState(0);
  const [isUploading, setIsUploading] = useState(false);
  const [closeConfirmOpen, setCloseConfirmOpen] = useState(false);
  const [selectedWorkspace, setSelectedWorkspace] = useState<WorkspaceSelectorValue | undefined>();
  const effectiveWorkspaceId = isOrganizationMode
    ? selectedWorkspace?.id
    : workspaceId || currentWorkspace?.id;
  const { folders, isLoading: isFoldersLoading } = useFileFolders(effectiveWorkspaceId, {
    enabled: !isOrganizationMode || !!effectiveWorkspaceId,
  });
  const [selectedFolderId, setSelectedFolderId] = useState(folderId || '');
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(new Set());
  const [selectedProcessingMode, setSelectedProcessingMode] =
    useState<FileUploadProcessingMode>(processingMode);
  const { data: uploadConfig } = useUploadConfig({ enabled: open });
  const maxSizeMB = uploadConfig?.file_size_limit ?? 15;
  const maxCount = uploadConfig?.batch_count_limit ?? 100;

  useEffect(() => {
    if (!open) return;
    setSelectedWorkspace(undefined);
    setSelectedFolderId(folderId || '');
    setExpandedFolders(new Set());
    setUploadFilesCount(0);
    setFailedUploadFilesCount(0);
    setSelectedProcessingMode(processingMode);
  }, [folderId, open, processingMode]);

  useEffect(() => {
    if (!open || !folderId) return;
    let ignore = false;

    const expandAncestors = async () => {
      const knownAncestorIds = getFileFolderAncestorIds(folders, folderId);
      const ancestorIds =
        knownAncestorIds.length > 0
          ? knownAncestorIds
          : await getFileFolderAncestorIdsByRequest(folderId);

      if (ignore || ancestorIds.length === 0) return;

      setExpandedFolders(prev => {
        const next = new Set(prev);
        let changed = false;
        ancestorIds.forEach(id => {
          if (!next.has(id)) {
            next.add(id);
            changed = true;
          }
        });
        return changed ? next : prev;
      });
    };

    void expandAncestors();

    return () => {
      ignore = true;
    };
  }, [folderId, folders, open]);

  const handleWorkspaceChange = useCallback((workspace: WorkspaceSelectorValue) => {
    setSelectedWorkspace(workspace);
    setSelectedFolderId('');
    setExpandedFolders(new Set());
  }, []);

  const handleToggleFolderExpand = useCallback((targetFolderId: string) => {
    setExpandedFolders(prev => {
      const next = new Set(prev);
      if (next.has(targetFolderId)) {
        next.delete(targetFolderId);
      } else {
        next.add(targetFolderId);
      }
      return next;
    });
  }, []);

  // Handle file selection change
  const handleFilesChange = useCallback((files: File[]) => {
    setUploadFilesCount(files.length);
  }, []);

  // Handle upload save
  const handleUploadSave = useCallback(async () => {
    const failedFiles = fileUploadRef.current?.getFailedFiles() ?? [];
    const pendingFiles = fileUploadRef.current?.getPendingFiles() ?? [];

    if (failedFiles.length > 0) {
      toast.error(t('files.upload.removeInvalidBeforeUpload'));
      return;
    }

    if (pendingFiles.length === 0) {
      toast.error(t('files.messages.noFiles'));
      return;
    }

    if (!fileUploadRef.current) {
      return;
    }

    try {
      setIsUploading(true);
      cancelUploadRequestedRef.current = false;
      // Use uploadAll method which handles progress tracking and folder assignment
      const uploadedFiles = await fileUploadRef.current.uploadAll();
      const wasCancelled = cancelUploadRequestedRef.current;

      if (uploadedFiles.length > 0) {
        // Invalidate files queries to refresh the list
        queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] });
      }

      if (wasCancelled) {
        return;
      }

      if (uploadedFiles.length > 0) {
        toast.success(t('files.messages.uploadSuccess'));
      }

      fileUploadRef.current.clearAll();
      setUploadFilesCount(0);
      setFailedUploadFilesCount(0);
      onOpenChange(false);
      // Call callback after successful upload
      onUploadComplete?.();
    } catch (error) {
      const message = (error as { message?: string }).message ?? 'Failed to upload files';
      toast.error(message);
    } finally {
      cancelUploadRequestedRef.current = false;
      setIsUploading(false);
    }
  }, [onOpenChange, onUploadComplete, t, queryClient]);

  // Handle cancel
  const handleCancel = useCallback(() => {
    fileUploadRef.current?.clearAll();
    setUploadFilesCount(0);
    setFailedUploadFilesCount(0);
    setSelectedWorkspace(undefined);
    setSelectedFolderId('');
    setExpandedFolders(new Set());
    onOpenChange(false);
  }, [onOpenChange]);

  const handleCancelUpload = useCallback(() => {
    setCloseConfirmOpen(false);
    cancelUploadRequestedRef.current = true;
    fileUploadRef.current?.cancelUploading();
    fileUploadRef.current?.clearAll();
    setUploadFilesCount(0);
    setFailedUploadFilesCount(0);
    setIsUploading(false);
    queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] });
    onOpenChange(false);
  }, [onOpenChange, queryClient]);

  const handleDialogOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen && isUploading) {
        setCloseConfirmOpen(true);
        return;
      }

      onOpenChange(nextOpen);
    },
    [isUploading, onOpenChange]
  );

  const canUpload = !isOrganizationMode || !!effectiveWorkspaceId;

  return (
    <Dialog open={open} onOpenChange={handleDialogOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-[760px]">
        <DialogHeader>
          <DialogTitle>{t('files.upload.uploadFiles')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-4 py-4">
          <FileUpload
            ref={fileUploadRef}
            autoUpload={false}
            maxCount={maxCount}
            maxSizeMB={maxSizeMB}
            acceptExt={acceptExt}
            folderId={selectedFolderId}
            workspaceId={effectiveWorkspaceId}
            processingMode={selectedProcessingMode}
            parseProvider="auto"
            showAllowedTypesHint={false}
            useNativeAccept={false}
            onFilesChange={handleFilesChange}
            onQueueStateChange={state => setFailedUploadFilesCount(state.failedCount)}
            queueSummaryNamespace="files"
          />

          {isOrganizationMode ? (
            <div className="space-y-2">
              <Label className="text-sm font-semibold">{t('files.upload.workspaceLabel')}</Label>
              <WorkspaceSelector
                value={selectedWorkspace}
                placeholder={t('files.upload.workspacePlaceholder')}
                autoSelectFirst
                onChange={handleWorkspaceChange}
              />
            </div>
          ) : null}

          <div className="grid gap-2">
            <Label className="text-sm font-semibold">{t('files.upload.storageLocation')}</Label>
            <div className="max-h-[240px] overflow-y-auto rounded-xl border border-border bg-muted/20 p-2">
              {isOrganizationMode && !effectiveWorkspaceId ? (
                <div className="px-3 py-6 text-center text-sm text-muted-foreground">
                  {t('files.upload.workspaceRequired')}
                </div>
              ) : isFoldersLoading ? (
                <div className="space-y-1">
                  {[1, 2, 3].map(index => (
                    <div
                      key={`upload-folder-skeleton-${index}`}
                      className="flex items-center gap-3 p-3"
                    >
                      <Skeleton className="h-5 w-5 rounded" />
                      <Skeleton className="h-4 flex-1" />
                    </div>
                  ))}
                </div>
              ) : (
                <div className="space-y-1">
                  <button
                    type="button"
                    onClick={() => setSelectedFolderId('')}
                    className={cn(
                      'flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm transition-colors',
                      selectedFolderId === ''
                        ? 'bg-background text-primary shadow-sm ring-1 ring-border'
                        : 'text-muted-foreground hover:bg-background/80 hover:text-foreground'
                    )}
                  >
                    <FolderOpen className="size-5 shrink-0" />
                    <span className="flex-1 truncate font-semibold">
                      {t('files.upload.defaultFolder')}
                    </span>
                  </button>
                  <div className="pl-4">
                    {folders.map(folder => (
                      <FolderTreeNode
                        key={folder.id}
                        folder={folder}
                        level={0}
                        activeItemId={selectedFolderId}
                        onItemClick={setSelectedFolderId}
                        expandedFolders={expandedFolders}
                        onToggleExpand={handleToggleFolderExpand}
                        maxLevel={MAX_FILE_FOLDER_TREE_LEVEL}
                        variant="dialog"
                        workspaceId={effectiveWorkspaceId}
                      />
                    ))}
                  </div>
                </div>
              )}
            </div>
            <p className="text-xs leading-5 text-muted-foreground">
              {t('files.upload.uploadFolderRootHelp')}
            </p>
          </div>

          <div className="rounded-xl border border-border bg-muted/20 p-4">
            <div className="space-y-3">
              <Label className="text-sm font-semibold">{t('files.upload.processingMode')}</Label>
              <RadioCardGroup
                value={selectedProcessingMode}
                onValueChange={value =>
                  setSelectedProcessingMode(value as FileUploadProcessingMode)
                }
                className="grid grid-cols-1 gap-3 sm:grid-cols-2"
              >
                <RadioCard
                  value="process_now"
                  title={t('files.upload.processingModes.processNow.title')}
                  description={t('files.upload.processingModes.processNow.desc')}
                  className="h-full"
                />
                <RadioCard
                  value="store_only"
                  title={t('files.upload.processingModes.storeOnly.title')}
                  description={t('files.upload.processingModes.storeOnly.desc')}
                  className="h-full"
                />
              </RadioCardGroup>
            </div>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button
            variant={isUploading ? 'destructive' : 'outline'}
            onClick={isUploading ? () => setCloseConfirmOpen(true) : handleCancel}
          >
            {isUploading ? t('files.upload.cancelUpload') : t('common.cancel')}
          </Button>
          <Button
            onClick={handleUploadSave}
            disabled={!canUpload || uploadFilesCount === 0 || failedUploadFilesCount > 0 || isUploading}
          >
            {isUploading && <RefreshCw className="mr-2 h-4 w-4 animate-spin" />}
            {t('files.upload.confirmUpload')}
          </Button>
        </DialogFooter>
      </DialogContent>
      <ConfirmDialog
        variant="danger"
        open={closeConfirmOpen}
        onOpenChange={setCloseConfirmOpen}
        title={t('files.upload.cancelUploadConfirmTitle')}
        description={t('files.upload.cancelUploadConfirmDescription')}
        confirmText={t('files.upload.cancelUploadConfirmAction')}
        cancelText={t('common.cancel')}
        onConfirm={handleCancelUpload}
      />
    </Dialog>
  );
};

export default CreateLocalFileDialog;
