import { memo, useState, type KeyboardEvent } from 'react';
import Link from 'next/link';
import {
  MoreHorizontal,
  FileIcon,
  Download,
  Eye,
  Trash2,
  CalendarDays,
  HardDrive,
  Link2,
  Activity,
  Info,
  FileSearch,
  FileUp,
} from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { cn } from '@/lib/utils';
import type { FileItem } from '@/services/types/file';
import { formatDate } from '@/utils/format';
import { RelatedResourcesPopover } from './related-resources-popover';
import { DeleteWarningDialog } from './delete-warning-dialog';
import {
  useDownloadFile,
  useDeleteFiles,
  FileAssociationError,
  FILES_QUERY_KEY,
} from '@/hooks/use-files';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { Badge } from '@/components/ui/badge';
import { useIsMobile } from '@/hooks/use-mobile';
import { FilePreviewDialog } from './file-preview-dialog';
import { isOriginalPreviewSupported } from '@/utils/file-helpers';
import { FILE_PERMISSION_ACTIONS } from '@/constants/permissions';
import { fileManageService } from '@/services/file-manage.service';
import { StartFileParseDialog } from './start-file-parse-dialog';
import { ReplaceDocumentDialog } from './replace-document-dialog';
import type { FileUploadProcessingMode } from '@/services/types/file';
import { getFileDetailKey } from '@/hooks/file/use-file-detail';
import { FILE_PARSE_PREVIEW_QUERY_KEY } from '@/hooks/file/use-file-parse-preview';
import { FILE_CHUNKS_QUERY_KEY } from '@/hooks/file/use-file-chunks';
import { FileTypeIcon } from './file-type-icon';

function getProcessingStatus(file: FileItem): string {
  return file.processing_status || 'stored_only';
}

function getEffectiveProcessingStatus(file: FileItem): string {
  return getProcessingStatus(file);
}

function getProcessingStatusTone(status: string) {
  switch (status) {
    case 'ready':
      return {
        badge: 'border-success/25 bg-success/10 text-success',
        bar: 'bg-success',
      };
    case 'confirming':
      return {
        badge: 'border-warning/25 bg-warning/10 text-warning',
        bar: 'bg-warning',
      };
    case 'parsing':
    case 'generating':
      return {
        badge: 'border-primary/25 bg-primary/10 text-primary',
        bar: 'bg-primary',
      };
    case 'parse_failed':
      return {
        badge: 'border-destructive/25 bg-destructive/10 text-destructive',
        bar: 'bg-destructive',
      };
    case 'stored_only':
    default:
      return {
        badge: 'border-border bg-muted/60 text-muted-foreground',
        bar: 'bg-muted-foreground',
      };
  }
}

function isProcessingActive(status: string) {
  return status === 'parsing' || status === 'confirming' || status === 'generating';
}

function getFileProcessingProgress(file: FileItem, status: string) {
  if (typeof file.processing_progress === 'number') {
    return Math.min(100, Math.max(0, file.processing_progress));
  }

  if (status === 'confirming') {
    return 82;
  }

  if (status === 'parsing' || status === 'generating') {
    return 96;
  }

  return 0;
}

function FileProcessingStatus({ file, compact = false }: { file: FileItem; compact?: boolean }) {
  const { files: t } = useT();
  const status = getEffectiveProcessingStatus(file);
  const progress = getFileProcessingProgress(file, status);
  const tone = getProcessingStatusTone(status);
  const showProgress = isProcessingActive(status);
  const statusLabel = (() => {
    switch (status) {
      case 'parsing':
        return t('processingStatus.parsing');
      case 'confirming':
        return t('processingStatus.confirming');
      case 'generating':
        return t('processingStatus.generating');
      case 'parse_failed':
        return t('processingStatus.parse_failed');
      case 'ready':
        return t('processingStatus.ready');
      case 'stored_only':
      default:
        return t('processingStatus.stored_only');
    }
  })();

  return (
    <div className="min-w-0">
      <div className={cn('flex min-w-0 items-center', compact ? 'gap-1.5' : 'gap-2')}>
        <span
          className={cn(
            'inline-flex h-5 shrink-0 items-center justify-center rounded-full border px-2 text-[12px] font-medium leading-none',
            tone.badge,
            compact && 'h-5 px-2 text-[11px]'
          )}
        >
          {statusLabel}
        </span>
        {showProgress ? (
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <div
              className={cn(
                'h-1.5 overflow-hidden rounded-full bg-border',
                compact ? 'w-12' : 'w-14'
              )}
              role="progressbar"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={progress}
            >
              <div
                className={cn('h-full rounded-full transition-[width]', tone.bar)}
                style={{ width: `${progress}%` }}
              />
            </div>
            <span
              className={cn(
                'w-8 shrink-0 text-right text-[12px] tabular-nums text-muted-foreground',
                compact && 'w-7 text-[11px]'
              )}
            >
              {progress}%
            </span>
          </div>
        ) : null}
      </div>
    </div>
  );
}

export interface FileListProps {
  files: FileItem[];
  maxCount?: number;
  total: number;
  onDelete?: (fileId: string) => void;
  selectedFiles?: string[];
  onSelectionChange?: (selectedIds: string[]) => void;
  isLoading?: boolean;
  /** When true, hides delete functionality (for dialog mode) */
  selectionMode?: boolean;
  /** Current active category (e.g., 'favorites') */
  activeCategory?: string;
  /** Folder name to show when browsing inside a specific folder. */
  folderNoticeName?: string;
  mobileEmptyActionLabel?: string;
  onMobileEmptyAction?: () => void;
  mobileEmptyDescription?: string;
}

function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return Math.round((bytes / Math.pow(k, i)) * 10) / 10 + ' ' + sizes[i];
}

function FileListBase({
  files,
  maxCount,
  total,
  selectedFiles = [],
  onSelectionChange,
  isLoading = false,
  selectionMode = false,
  folderNoticeName,
  mobileEmptyActionLabel,
  onMobileEmptyAction,
  mobileEmptyDescription,
}: FileListProps) {
  const { files: t, common } = useT();
  const router = useRouter();
  const queryClient = useQueryClient();
  const isMobile = useIsMobile();
  const { downloadFile, isDownloading } = useDownloadFile();
  const { deleteFiles, isDeleting } = useDeleteFiles();
  const [previewFile, setPreviewFile] = useState<FileItem | null>(null);
  const [startParseFile, setStartParseFile] = useState<FileItem | null>(null);
  const [replaceDocumentFile, setReplaceDocumentFile] = useState<FileItem | null>(null);
  const [startingParseFileId, setStartingParseFileId] = useState<string | null>(null);
  const [isBatchParsing, setIsBatchParsing] = useState(false);
  const [replacingFileId, setReplacingFileId] = useState<string | null>(null);
  const [reparsingFileId, setReparsingFileId] = useState<string | null>(null);

  // Permission checks
  const { hasAnyPermission } = useAccountPermissions();
  const canDownload = hasAnyPermission(FILE_PERMISSION_ACTIONS.download);
  const canPreview = hasAnyPermission(FILE_PERMISSION_ACTIONS.preview);
  const canUpdateFile = hasAnyPermission(FILE_PERMISSION_ACTIONS.update);
  const canDeleteFilePermission = hasAnyPermission(FILE_PERMISSION_ACTIONS.delete);
  const canUpload = hasAnyPermission(FILE_PERMISSION_ACTIONS.upload);
  const canViewRelatedResources = hasAnyPermission(FILE_PERMISSION_ACTIONS.relatedView);
  const canOpenFileDetailByPermission = hasAnyPermission([
    ...FILE_PERMISSION_ACTIONS.metadataView,
    ...FILE_PERMISSION_ACTIONS.preview,
    ...FILE_PERMISSION_ACTIONS.relatedView,
    ...FILE_PERMISSION_ACTIONS.download,
    ...FILE_PERMISSION_ACTIONS.update,
    ...FILE_PERMISSION_ACTIONS.delete,
    ...FILE_PERMISSION_ACTIONS.move,
    ...FILE_PERMISSION_ACTIONS.archive,
    ...FILE_PERMISSION_ACTIONS.shareManage,
    ...FILE_PERMISSION_ACTIONS.favoriteManage,
  ]);
  const canViewDetail = !selectionMode && canOpenFileDetailByPermission;
  const canRequestProcessing = !selectionMode && canUpdateFile;
  const hasAnyAction =
    canViewDetail || canRequestProcessing || canDownload || canPreview || canDeleteFilePermission;
  const emptyDescription = mobileEmptyDescription
    ? mobileEmptyDescription
    : canUpload
      ? selectionMode
        ? t('messages.noFilesDescWithUploadInSelector')
        : t('messages.noFilesDescWithUpload')
      : t('messages.noFilesDescWithoutUploadPermission');

  // Delete warning dialog state
  const [showDeleteWarning, setShowDeleteWarning] = useState(false);
  const [deleteWarningFileNames, setDeleteWarningFileNames] = useState<string[]>([]);

  // Bulk delete confirm dialog state
  const [showBulkDeleteConfirm, setShowBulkDeleteConfirm] = useState(false);

  const allSelected = files.length > 0 && files.every(file => selectedFiles.includes(file.id));
  const someSelected = files.some(file => selectedFiles.includes(file.id));
  const selectedStoredOnlyCount = files.filter(
    file => selectedFiles.includes(file.id) && getProcessingStatus(file) === 'stored_only'
  ).length;

  const handleSelectAll = (checked: boolean) => {
    if (!onSelectionChange) return;

    if (checked) {
      const currentSelectedCount = selectedFiles.length;
      const currentPageIds = files.map(file => file.id);
      const newFilesToSelect = currentPageIds.filter(id => !selectedFiles.includes(id));

      if (maxCount !== undefined) {
        const availableSlots = maxCount - currentSelectedCount;

        if (availableSlots <= 0) {
          toast.error(t('maxCountExceeded', { max: maxCount }));
          return;
        }

        if (newFilesToSelect.length > availableSlots) {
          const filesToSelect = newFilesToSelect.slice(0, availableSlots);
          const newSelection = [...new Set([...selectedFiles, ...filesToSelect])];
          onSelectionChange(newSelection);
          toast.error(t('maxCountExceeded', { max: maxCount }));
          return;
        }
      }

      const newSelection = [...new Set([...selectedFiles, ...newFilesToSelect])];
      onSelectionChange(newSelection);
    } else {
      const currentPageIds = files.map(file => file.id);
      const newSelection = selectedFiles.filter(id => !currentPageIds.includes(id));
      onSelectionChange(newSelection);
    }
  };

  const handleDownload = async (file: FileItem) => {
    try {
      await downloadFile(file.id, file.name);
    } catch (_error) {
      // Error is handled by the hook
    }
  };

  const handlePreview = (file: FileItem) => {
    setPreviewFile(file);
  };

  const handleOpenDetail = (file: FileItem) => {
    router.push(`/console/files/${file.id}`);
  };

  const handleOpenStartParse = (file: FileItem) => {
    setStartParseFile(file);
  };

  const handleOpenReplaceDocument = (file: FileItem) => {
    setReplaceDocumentFile(file);
  };

  const handleStartParse = async (file: FileItem) => {
    if (!canRequestProcessing) return;
    try {
      setStartingParseFileId(file.id);
      await fileManageService.createProcessingRequest(file.id, {
        mode: 'parse_now',
        target_level: 'vectorize',
        force: false,
        parse_provider: 'auto',
      });
      toast.success(t('fileList.startParseDialog.toasts.started'));
      setStartParseFile(null);
      await queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] });
    } catch (error) {
      toast.error(
        (error as { message?: string }).message || t('fileList.startParseDialog.toasts.failed')
      );
    } finally {
      setStartingParseFileId(null);
    }
  };

  const handleBatchParse = async () => {
    if (!canRequestProcessing) return;
    if (selectedStoredOnlyCount === 0 || isBatchParsing) return;
    const targets = files.filter(
      file => selectedFiles.includes(file.id) && getProcessingStatus(file) === 'stored_only'
    );
    if (targets.length === 0) return;

    setIsBatchParsing(true);
    try {
      const results = await Promise.allSettled(
        targets.map(file =>
          fileManageService.createProcessingRequest(file.id, {
            mode: 'parse_now',
            target_level: 'vectorize',
            force: false,
            parse_provider: 'auto',
          })
        )
      );
      const failedCount = results.filter(result => result.status === 'rejected').length;
      const successCount = targets.length - failedCount;
      if (successCount > 0) {
        toast.success(t('fileList.startParseDialog.toasts.batchStarted', { count: successCount }));
      }
      if (failedCount > 0) {
        toast.error(t('fileList.startParseDialog.toasts.batchFailed', { count: failedCount }));
      }
      if (successCount > 0) {
        onSelectionChange?.(selectedFiles.filter(id => !targets.some(file => file.id === id)));
      }
      await queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] });
    } finally {
      setIsBatchParsing(false);
    }
  };

  const handleReparse = async (file: FileItem) => {
    try {
      setReparsingFileId(file.id);
      await fileManageService.createProcessingRequest(file.id, {
        mode: 'reparse',
        target_level: 'vectorize',
        force: false,
      });
      toast.success(t('detail.reparse.toasts.started'));
      await queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] });
    } catch (error) {
      toast.error((error as { message?: string }).message || t('detail.reparse.toasts.failed'));
    } finally {
      setReparsingFileId(null);
    }
  };

  const handleReplaceDocument = async (
    file: FileItem,
    replacementFile: File,
    processingMode: FileUploadProcessingMode
  ) => {
    try {
      setReplacingFileId(file.id);
      await fileManageService.replaceDocument(file.id, {
        file: replacementFile,
        processing_mode: processingMode,
        parse_provider: 'auto',
      });
      toast.success(t('replaceDocument.toasts.started'));
      setReplaceDocumentFile(null);
      queryClient.removeQueries({ queryKey: getFileDetailKey(file.id) });
      queryClient.removeQueries({ queryKey: [FILE_PARSE_PREVIEW_QUERY_KEY, file.id] });
      queryClient.removeQueries({ queryKey: [FILE_CHUNKS_QUERY_KEY, file.id] });
      await queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] });
    } catch (error) {
      toast.error((error as { message?: string }).message || t('replaceDocument.toasts.failed'));
    } finally {
      setReplacingFileId(null);
    }
  };

  const handleCardKeyDown = (event: KeyboardEvent<HTMLDivElement>, file: FileItem) => {
    if (event.key !== 'Enter' && event.key !== ' ') return;

    event.preventDefault();
    handleRowClick(file);
  };

  const handleDelete = async (file: FileItem) => {
    try {
      await deleteFiles([file.id], files);
      if (selectedFiles.includes(file.id)) {
        onSelectionChange?.(selectedFiles.filter(id => id !== file.id));
      }
    } catch (error) {
      if (error instanceof FileAssociationError) {
        setDeleteWarningFileNames(error.fileNames);
        setShowDeleteWarning(true);
      }
    }
  };

  const handleBulkDeleteClick = () => {
    if (!canDeleteFilePermission) return;
    if (selectedFiles.length === 0 || isDeleting) return;
    setShowBulkDeleteConfirm(true);
  };

  const handleBulkDeleteConfirm = async () => {
    if (!canDeleteFilePermission) return;
    try {
      await deleteFiles(selectedFiles, files);
      onSelectionChange?.([]);
      setShowBulkDeleteConfirm(false);
    } catch (error) {
      if (error instanceof FileAssociationError) {
        setDeleteWarningFileNames(error.fileNames);
        setShowDeleteWarning(true);
      }
      setShowBulkDeleteConfirm(false);
    }
  };

  const handleFileSelect = (fileId: string, checked: boolean) => {
    if (!onSelectionChange) return;

    if (checked) {
      // Check maxCount limit before adding
      if (maxCount !== undefined && selectedFiles.length >= maxCount) {
        toast.error(t('maxCountExceeded', { max: maxCount }));
        return;
      }

      onSelectionChange([...selectedFiles, fileId]);
    } else {
      onSelectionChange(selectedFiles.filter(id => id !== fileId));
    }
  };

  const handleRowClick = (file: FileItem) => {
    const willSelect = !selectedFiles.includes(file.id);

    if (willSelect && maxCount !== undefined && selectedFiles.length >= maxCount) {
      toast.error(t('maxCountExceeded', { max: maxCount }));
      return;
    }

    handleFileSelect(file.id, willSelect);
  };

  if (isMobile && selectionMode) {
    return (
      <div className="flex h-full flex-col overflow-hidden bg-background">
        <div className="border-b px-3 py-2.5">
          {folderNoticeName ? (
            <div className="mb-2 rounded-xl border border-primary/15 bg-primary/5 px-3 py-2 text-xs leading-5 text-muted-foreground">
              {t('fileList.folderNotice', { name: folderNoticeName })}
            </div>
          ) : null}
          <div className="flex items-center justify-between">
            <div className="min-w-0">
              <div className="text-sm font-medium text-foreground">
                {t('fileList.totalItems', { total })}
              </div>
              {selectedFiles.length > 0 ? (
                <div className="mt-0.5 text-xs text-muted-foreground">
                  {maxCount !== undefined
                    ? t('selectedCountWithMax', { count: selectedFiles.length, max: maxCount })
                    : t('selectedCount', { count: selectedFiles.length })}
                </div>
              ) : null}
            </div>

            {files.length > 0 ? (
              <Button
                variant="outline"
                size="sm"
                className="h-8 rounded-lg px-3"
                onClick={() => handleSelectAll(!allSelected)}
                disabled={
                  maxCount !== undefined &&
                  !allSelected &&
                  selectedFiles.length >= maxCount &&
                  files.some(file => !selectedFiles.includes(file.id))
                }
              >
                {allSelected ? common('clear') : t('fileList.selectAll')}
              </Button>
            ) : null}
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
          {isLoading ? (
            <div className="space-y-3">
              {Array.from({ length: 6 }).map((_, index) => (
                <div
                  key={`mobile-skeleton-${index}`}
                  className="rounded-2xl border border-border/70 bg-background p-4 shadow-sm"
                >
                  <div className="mb-4 flex items-start justify-between gap-3">
                    <div className="flex min-w-0 items-center gap-3">
                      <div className="h-10 w-10 rounded-2xl bg-muted animate-pulse" />
                      <div className="min-w-0 space-y-2">
                        <div className="h-4 w-32 rounded bg-muted animate-pulse" />
                        <div className="h-3 w-16 rounded bg-muted animate-pulse" />
                      </div>
                    </div>
                    <div className="h-4 w-4 rounded-sm bg-muted animate-pulse" />
                  </div>
                  <div className="grid grid-cols-2 gap-2">
                    <div className="h-12 rounded-xl bg-muted/70 animate-pulse" />
                    <div className="h-12 rounded-xl bg-muted/70 animate-pulse" />
                    <div className="h-12 rounded-xl bg-muted/70 animate-pulse" />
                    <div className="h-12 rounded-xl bg-muted/70 animate-pulse" />
                  </div>
                </div>
              ))}
            </div>
          ) : files.length === 0 ? (
            <div className="flex min-h-full items-center justify-center py-6">
              <div className="mx-auto flex w-full max-w-[360px] flex-col items-center rounded-3xl border border-dashed border-border/70 bg-background px-6 py-8 text-center shadow-sm">
                <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-primary/10 text-primary">
                  <FileIcon className="h-7 w-7" />
                </div>
                <h3 className="mb-2 text-xl font-semibold text-foreground">
                  {t('messages.noFiles')}
                </h3>
                <p className="max-w-[280px] text-sm leading-6 text-muted-foreground">
                  {emptyDescription}
                </p>
                {mobileEmptyActionLabel && onMobileEmptyAction ? (
                  <Button className="mt-5 h-10 rounded-xl px-4" onClick={onMobileEmptyAction}>
                    {mobileEmptyActionLabel}
                  </Button>
                ) : null}
              </div>
            </div>
          ) : (
            <div className="space-y-3">
              {files.map(file => {
                const isSelected = selectedFiles.includes(file.id);
                const processingStatus = getEffectiveProcessingStatus(file);
                const canStartParse = processingStatus === 'stored_only' && canRequestProcessing;
                const canOpenFileDetail = canViewDetail && processingStatus !== 'stored_only';
                const canReplaceDocument =
                  !selectionMode &&
                  canUpdateFile &&
                  Boolean(file.asset_id) &&
                  !isProcessingActive(processingStatus);

                return (
                  <div
                    key={file.id}
                    role="button"
                    tabIndex={0}
                    className={cn(
                      'w-full rounded-2xl border p-4 text-left shadow-sm transition-colors',
                      isSelected
                        ? 'border-primary/40 bg-primary/5'
                        : 'border-border/70 bg-background active:bg-muted/50'
                    )}
                    onClick={() => handleRowClick(file)}
                    onKeyDown={event => handleCardKeyDown(event, file)}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex min-w-0 items-start gap-3">
                        <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-muted">
                          <FileTypeIcon
                            extension={file.extension}
                            filename={file.name}
                            className="h-5 w-5"
                          />
                        </div>
                        <div className="min-w-0">
                          <div className="truncate text-sm font-semibold text-foreground">
                            {file.name}
                          </div>
                          <div className="mt-1 flex flex-wrap items-center gap-2">
                            <Badge
                              variant="outline"
                              className="rounded-full px-2 py-0.5 text-[11px]"
                            >
                              {file.extension}
                            </Badge>
                            {canViewRelatedResources && file.related_count > 0 ? (
                              <Badge
                                variant="secondary"
                                className="rounded-full px-2 py-0.5 text-[11px]"
                              >
                                {t('fileList.relatedCount', { count: file.related_count })}
                              </Badge>
                            ) : null}
                            <FileProcessingStatus file={file} compact />
                          </div>
                        </div>
                      </div>

                      <div className="flex shrink-0 items-center gap-2">
                        {canStartParse ? (
                          <Button
                            isIcon
                            type="button"
                            variant="ghost"
                            className="h-8 w-8 rounded-lg"
                            onClick={e => {
                              e.stopPropagation();
                              handleOpenStartParse(file);
                            }}
                            aria-label={t('actions.startParse')}
                          >
                            <FileSearch className="h-4 w-4" />
                          </Button>
                        ) : canOpenFileDetail ? (
                          <Button
                            isIcon
                            type="button"
                            variant="ghost"
                            className="h-8 w-8 rounded-lg"
                            onClick={e => {
                              e.stopPropagation();
                              handleOpenDetail(file);
                            }}
                            aria-label={t('actions.viewDetails')}
                          >
                            <Info className="h-4 w-4" />
                          </Button>
                        ) : null}
                        {canPreview &&
                        isOriginalPreviewSupported(file.extension, file.mime_type) ? (
                          <Button
                            isIcon
                            type="button"
                            variant="ghost"
                            className="h-8 w-8 rounded-lg"
                            onClick={e => {
                              e.stopPropagation();
                              handlePreview(file);
                            }}
                            aria-label={t('actions.preview')}
                          >
                            <Eye className="h-4 w-4" />
                          </Button>
                        ) : null}
                        {canReplaceDocument ? (
                          <Button
                            isIcon
                            type="button"
                            variant="ghost"
                            className="h-8 w-8 rounded-lg"
                            onClick={e => {
                              e.stopPropagation();
                              handleOpenReplaceDocument(file);
                            }}
                            disabled={replacingFileId === file.id}
                            aria-label={t('actions.replaceDocument')}
                          >
                            <FileUp className="h-4 w-4" />
                          </Button>
                        ) : null}
                        <Checkbox
                          checked={isSelected}
                          onCheckedChange={checked => handleFileSelect(file.id, checked as boolean)}
                          onClick={e => e.stopPropagation()}
                          disabled={
                            maxCount !== undefined &&
                            selectedFiles.length >= maxCount &&
                            !isSelected
                          }
                        />
                      </div>
                    </div>

                    <div className="mt-4 grid grid-cols-2 gap-2">
                      <div className="rounded-xl bg-muted/50 px-3 py-2">
                        <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                          <HardDrive className="h-3 w-3" />
                          <span>{t('fileList.fileSize')}</span>
                        </div>
                        <div className="mt-1 text-sm font-medium text-foreground">
                          {formatFileSize(file.size)}
                        </div>
                      </div>
                      <div className="rounded-xl bg-muted/50 px-3 py-2">
                        <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                          <CalendarDays className="h-3 w-3" />
                          <span>{t('fileList.uploadDate')}</span>
                        </div>
                        <div className="mt-1 text-sm font-medium text-foreground">
                          {formatDate(new Date(file.created_at).getTime() - 8 * 60 * 60 * 1000)}
                        </div>
                      </div>
                      <div className="col-span-2 rounded-xl bg-muted/50 px-3 py-2">
                        <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                          <Activity className="h-3 w-3" />
                          <span>{t('fileList.processingStatus')}</span>
                        </div>
                        <div className="mt-1">
                          <FileProcessingStatus file={file} />
                        </div>
                      </div>
                      <div className="col-span-2 rounded-xl bg-muted/50 px-3 py-2">
                        <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                          <Link2 className="h-3 w-3" />
                          <span>{t('fileList.relatedStatus')}</span>
                        </div>
                        <div className="mt-1 text-sm font-medium text-foreground">
                          {canViewRelatedResources
                            ? file.related_count > 0
                              ? t('fileList.relatedCount', { count: file.related_count })
                              : t('fileList.notRelated')
                            : '-'}
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>

        <DeleteWarningDialog
          open={showDeleteWarning}
          onOpenChange={setShowDeleteWarning}
          fileNames={deleteWarningFileNames}
        />

        <ConfirmDialog
          open={showBulkDeleteConfirm}
          onOpenChange={setShowBulkDeleteConfirm}
          title={t('delete.bulkConfirmTitle', { count: selectedFiles.length })}
          description={t('delete.bulkConfirmDescription')}
          confirmText={t('actions.delete')}
          cancelText={common('cancel')}
          onConfirm={handleBulkDeleteConfirm}
          loading={isDeleting}
          variant="danger"
        />

        <StartFileParseDialog
          open={Boolean(startParseFile)}
          onOpenChange={open => {
            if (!open) setStartParseFile(null);
          }}
          file={startParseFile}
          loading={Boolean(startParseFile && startingParseFileId === startParseFile.id)}
          onConfirm={file => {
            void handleStartParse(file);
          }}
        />

        <FilePreviewDialog
          open={Boolean(previewFile)}
          onOpenChange={open => {
            if (!open) setPreviewFile(null);
          }}
          file={previewFile}
          onDownload={
            canDownload
              ? file => {
                  void handleDownload(file);
                }
              : undefined
          }
          isDownloading={isDownloading}
        />
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden bg-background">
      <div className="flex min-h-12 items-center justify-between border-b px-4 py-2.5">
        <div className="flex min-w-0 items-center gap-3">
          <div className="text-base font-medium text-foreground">
            {t('fileList.totalItems', { total })}
          </div>
          {selectedFiles.length > 0 ? (
            <span className="rounded-full border bg-muted/40 px-2.5 py-1 text-xs font-medium text-muted-foreground">
              {maxCount !== undefined
                ? t('selectedCountWithMax', { count: selectedFiles.length, max: maxCount })
                : t('selectedCount', { count: selectedFiles.length })}
            </span>
          ) : null}
        </div>

        {selectedFiles.length > 0 && !selectionMode ? (
          <div className="flex shrink-0 items-center gap-1.5 rounded-lg border bg-muted/30 p-1">
            {canRequestProcessing ? (
              <Button
                type="button"
                size="sm"
                className="h-8 rounded-md px-3 text-xs"
                onClick={handleBatchParse}
                disabled={selectedStoredOnlyCount === 0 || isBatchParsing}
                title={
                  selectedStoredOnlyCount === 0 ? t('actions.batchParseNoStoredOnly') : undefined
                }
              >
                <FileSearch className="h-4 w-4" />
                {isBatchParsing ? t('actions.batchParsing') : t('actions.batchParse')}
                {selectedStoredOnlyCount > 0 ? (
                  <span className="ml-1 text-[11px] opacity-80">{selectedStoredOnlyCount}</span>
                ) : null}
              </Button>
            ) : null}
            {canDeleteFilePermission ? (
              <Button
                variant="ghost"
                size="sm"
                className="h-8 rounded-md px-3 text-xs text-muted-foreground shadow-none hover:bg-destructive/5 hover:text-destructive"
                onClick={handleBulkDeleteClick}
                disabled={isDeleting}
              >
                {isDeleting ? t('actions.deleting') : t('actions.bulkDelete')}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
      {folderNoticeName ? (
        <div className="shrink-0 border-b bg-primary/5 px-4 py-2 text-sm leading-6 text-muted-foreground">
          {t('fileList.folderNotice', { name: folderNoticeName })}
        </div>
      ) : null}
      <Table className="min-w-[1024px] table-fixed" containerClassName="overflow-auto flex-1">
        <colgroup>
          <col style={{ width: 44 }} />
          <col style={{ width: 220 }} />
          <col style={{ width: 66 }} />
          <col style={{ width: 80 }} />
          <col style={{ width: 188 }} />
          <col style={{ width: 116 }} />
          <col style={{ width: 150 }} />
          {hasAnyAction ? <col style={{ width: 160 }} /> : null}
        </colgroup>
        <TableHeader className="sticky top-0 z-10 bg-secondary/40 backdrop-blur">
          <TableRow className="h-11 hover:bg-muted/30">
            <TableHead className="px-3">
              <Checkbox
                className="size-4 rounded-[3px] [&_svg]:size-3 [&_svg]:stroke-[2]"
                checked={allSelected}
                onCheckedChange={handleSelectAll}
                disabled={
                  maxCount !== undefined &&
                  selectedFiles.length >= maxCount &&
                  files.some(file => !selectedFiles.includes(file.id))
                }
                ref={el => {
                  if (el && 'indeterminate' in el) {
                    (el as HTMLInputElement).indeterminate = someSelected && !allSelected;
                  }
                }}
              />
            </TableHead>
            <TableHead className="text-[13px]">{t('fileList.fileName')}</TableHead>
            <TableHead className="text-[13px]">{t('fileList.fileType')}</TableHead>
            <TableHead className="text-[13px]">{t('fileList.fileSize')}</TableHead>
            <TableHead className="text-[13px]">{t('fileList.processingStatus')}</TableHead>
            <TableHead className="text-[13px]">{t('fileList.relatedStatus')}</TableHead>
            <TableHead className="text-[13px]">{t('fileList.uploadDate')}</TableHead>
            {hasAnyAction && (
              <TableHead className="pr-8 text-right text-[13px]">{t('fileList.actions')}</TableHead>
            )}
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading ? (
            // Loading skeleton rows matching actual layout
            Array.from({ length: 8 }).map((_, index) => (
              <TableRow key={`skeleton-${index}`}>
                {/* Checkbox */}
                <TableCell>
                  <div className="h-4 w-4 bg-muted animate-pulse rounded-sm border border-muted-foreground/20" />
                </TableCell>
                {/* File name with icon */}
                <TableCell className="font-medium max-w-[200px]">
                  <div className="flex items-center gap-2">
                    <div className="h-5 w-5 bg-muted animate-pulse rounded flex-shrink-0" />
                    <div
                      className="h-4 bg-muted animate-pulse rounded"
                      style={{ width: `${100 + (index % 4) * 30}px` }}
                    />
                  </div>
                </TableCell>
                {/* Extension */}
                <TableCell className="text-sm">
                  <div className="h-4 w-10 bg-muted animate-pulse rounded" />
                </TableCell>
                {/* File size */}
                <TableCell className="text-sm">
                  <div className="h-4 w-14 bg-muted animate-pulse rounded" />
                </TableCell>
                {/* Processing status */}
                <TableCell className="text-sm">
                  <div className="flex items-center gap-2">
                    <div className="h-5 w-16 bg-muted animate-pulse rounded-full" />
                    <div className="h-1.5 w-14 bg-muted animate-pulse rounded-full" />
                    <div className="h-3 w-8 bg-muted animate-pulse rounded" />
                  </div>
                </TableCell>
                {/* Related status badge */}
                <TableCell className="text-sm">
                  <div className="h-6 w-16 bg-muted animate-pulse rounded-full" />
                </TableCell>
                {/* Upload date */}
                <TableCell className="text-sm text-muted-foreground">
                  <div className="h-4 w-24 bg-muted animate-pulse rounded" />
                </TableCell>
                {/* Actions button */}
                {hasAnyAction && (
                  <TableCell className="pr-8 text-right">
                    <div className="h-8 w-8 bg-muted animate-pulse rounded-md ml-auto" />
                  </TableCell>
                )}
              </TableRow>
            ))
          ) : files.length === 0 ? (
            <TableRow className="hover:bg-transparent">
              <TableCell colSpan={hasAnyAction ? 8 : 7} className="border-0 p-0 whitespace-normal">
                <div className="flex min-h-[360px] items-center justify-center px-6 py-10">
                  <div className="mx-auto flex w-full max-w-[560px] flex-col items-center rounded-lg border border-dashed border-border/80 bg-bg-canvas/40 px-8 py-8 text-center">
                    <div className="mb-5 flex h-14 w-14 items-center justify-center rounded-md bg-background text-muted-foreground ring-1 ring-border/80">
                      <FileIcon className="h-8 w-8" />
                    </div>
                    <h3 className="mb-3 text-xl font-semibold text-foreground">
                      {t('messages.noFiles')}
                    </h3>
                    <p className="max-w-[460px] text-sm leading-7 text-muted-foreground">
                      {emptyDescription}
                    </p>
                  </div>
                </div>
              </TableCell>
            </TableRow>
          ) : (
            files.map(file => {
              const processingStatus = getEffectiveProcessingStatus(file);
              const canStartParse = processingStatus === 'stored_only' && canRequestProcessing;
              const canOpenFileDetail = canViewDetail && processingStatus !== 'stored_only';
              const canPreviewOriginal =
                canPreview && isOriginalPreviewSupported(file.extension, file.mime_type);
              const canDeleteFile = canDeleteFilePermission && !selectionMode;
              const canReplaceDocument =
                !selectionMode &&
                canUpdateFile &&
                Boolean(file.asset_id) &&
                !isProcessingActive(processingStatus);
              const canShowActionsMenu =
                canStartParse ||
                canOpenFileDetail ||
                canDownload ||
                canReplaceDocument ||
                canDeleteFile;

              return (
                <TableRow
                  key={file.id}
                  className={cn(
                    'h-14 cursor-pointer transition-colors hover:bg-muted/30',
                    processingStatus === 'confirming' &&
                      'bg-warning/5 shadow-[inset_2px_0_0_var(--warning)] hover:bg-warning/10',
                    processingStatus === 'parse_failed' &&
                      'bg-destructive/5 shadow-[inset_2px_0_0_var(--destructive)] hover:bg-destructive/10',
                    selectedFiles.includes(file.id) &&
                      'bg-primary/5 shadow-[inset_2px_0_0_var(--primary)]'
                  )}
                  onClick={() => handleRowClick(file)}
                >
                  <TableCell className="px-3">
                    <Checkbox
                      className="size-4 rounded-[3px] [&_svg]:size-3 [&_svg]:stroke-[2]"
                      checked={selectedFiles.includes(file.id)}
                      onCheckedChange={checked => handleFileSelect(file.id, checked as boolean)}
                      onClick={e => e.stopPropagation()}
                      disabled={
                        maxCount !== undefined &&
                        selectedFiles.length >= maxCount &&
                        !selectedFiles.includes(file.id)
                      }
                    />
                  </TableCell>
                  <TableCell className="max-w-0 font-medium">
                    <div className="flex min-w-0 items-center gap-3">
                      <FileTypeIcon
                        extension={file.extension}
                        filename={file.name}
                        className="h-4 w-4 flex-shrink-0"
                      />
                      {canOpenFileDetail ? (
                        <Link
                          href={`/console/files/${file.id}`}
                          className="truncate text-[13px] font-medium text-foreground underline-offset-4 hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30"
                          onClick={event => event.stopPropagation()}
                        >
                          {file.name}
                        </Link>
                      ) : (
                        <span className="truncate text-[13px] font-medium text-foreground">
                          {file.name}
                        </span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-[13px] text-foreground">{file.extension}</TableCell>
                  <TableCell className="text-[13px] text-foreground">
                    {formatFileSize(file.size)}
                  </TableCell>
                  <TableCell>
                    <FileProcessingStatus file={file} />
                  </TableCell>
                  <TableCell>
                    {canViewRelatedResources && file.related_count > 0 ? (
                      <RelatedResourcesPopover fileId={file.id} relatedCount={file.related_count}>
                        <span className="inline-flex max-w-full cursor-pointer items-center rounded-full bg-primary/10 px-2 py-0.5 text-[12px] font-medium text-primary transition-colors hover:bg-primary/15">
                          {t('fileList.relatedCount', { count: file.related_count })}
                        </span>
                      </RelatedResourcesPopover>
                    ) : canViewRelatedResources ? (
                      <span className="inline-flex max-w-full items-center rounded-full bg-muted px-2 py-0.5 text-[12px] font-medium text-muted-foreground">
                        {t('fileList.notRelated')}
                      </span>
                    ) : (
                      <span className="inline-flex max-w-full items-center rounded-full bg-muted px-2 py-0.5 text-[12px] font-medium text-muted-foreground">
                        -
                      </span>
                    )}
                  </TableCell>
                  <TableCell className="max-w-0 truncate text-[13px] text-muted-foreground">
                    {formatDate(new Date(file.created_at).getTime() - 8 * 60 * 60 * 1000)}
                  </TableCell>
                  {hasAnyAction && (
                    <TableCell className="pr-8 text-right">
                      <div className="flex min-w-0 items-center justify-end gap-1.5">
                        {canOpenFileDetail && processingStatus === 'confirming' ? (
                          <Button
                            asChild
                            variant="outline"
                            size="sm"
                            className="h-8 rounded-lg px-3 text-[12px]"
                            onClick={event => event.stopPropagation()}
                          >
                            <Link href={`/console/files/${file.id}`}>
                              {t('actions.confirmParse')}
                            </Link>
                          </Button>
                        ) : null}
                        {canStartParse ? (
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            className="h-8 rounded-lg px-3 text-[12px]"
                            disabled={startingParseFileId === file.id}
                            onClick={event => {
                              event.stopPropagation();
                              handleOpenStartParse(file);
                            }}
                          >
                            {startingParseFileId === file.id
                              ? t('actions.startParsing')
                              : t('actions.startParse')}
                          </Button>
                        ) : null}
                        {processingStatus === 'parse_failed' ? (
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            className="h-8 rounded-lg px-3 text-[12px]"
                            disabled={reparsingFileId === file.id}
                            onClick={event => {
                              event.stopPropagation();
                              void handleReparse(file);
                            }}
                          >
                            {reparsingFileId === file.id
                              ? t('detail.reparse.reparsing')
                              : t('detail.reparse.action')}
                          </Button>
                        ) : null}
                        {canShowActionsMenu ? (
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-8 w-8 rounded-md p-0 text-muted-foreground hover:bg-muted hover:text-foreground"
                                onClick={e => e.stopPropagation()}
                              >
                                <MoreHorizontal className="h-4 w-4" />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end" className="w-40 rounded-lg">
                              {canStartParse ? (
                                <DropdownMenuItem
                                  onClick={e => {
                                    e.stopPropagation();
                                    handleOpenStartParse(file);
                                  }}
                                >
                                  <FileSearch className="h-4 w-4 mr-2" />
                                  {t('actions.startParse')}
                                </DropdownMenuItem>
                              ) : canOpenFileDetail ? (
                                <DropdownMenuItem
                                  onClick={e => {
                                    e.stopPropagation();
                                    handleOpenDetail(file);
                                  }}
                                >
                                  <Info className="h-4 w-4 mr-2" />
                                  {t('actions.viewDetails')}
                                </DropdownMenuItem>
                              ) : null}
                              {canPreviewOriginal ? (
                                <DropdownMenuItem
                                  onClick={e => {
                                    e.stopPropagation();
                                    handlePreview(file);
                                  }}
                                >
                                  <Eye className="h-4 w-4 mr-2" />
                                  {t('actions.preview')}
                                </DropdownMenuItem>
                              ) : null}

                              {canDownload ? (
                                <DropdownMenuItem
                                  onClick={e => {
                                    e.stopPropagation();
                                    handleDownload(file);
                                  }}
                                  disabled={isDownloading}
                                >
                                  <Download className="h-4 w-4 mr-2" />
                                  {t('actions.downloadFile')}
                                </DropdownMenuItem>
                              ) : null}

                              {canReplaceDocument ? (
                                <DropdownMenuItem
                                  onClick={e => {
                                    e.stopPropagation();
                                    handleOpenReplaceDocument(file);
                                  }}
                                  disabled={replacingFileId === file.id}
                                >
                                  <FileUp className="h-4 w-4 mr-2" />
                                  {t('actions.replaceDocument')}
                                </DropdownMenuItem>
                              ) : null}

                              {/* TODO: Favorites feature temporarily disabled, may restore later
                      {file.is_favorite ? (
                        <DropdownMenuItem
                          onClick={e => {
                            e.stopPropagation();
                            removeFavorite(file.id);
                          }}
                          disabled={isRemoving}
                        >
                          <Star className="h-4 w-4 mr-2 text-yellow-500 fill-yellow-500" />
                          {t('actions.removeFromFavorites')}
                        </DropdownMenuItem>
                      ) : (
                        <DropdownMenuItem
                          onClick={e => {
                            e.stopPropagation();
                            addFavorite(file.id);
                          }}
                          disabled={isAdding}
                        >
                          <Star className="h-4 w-4 mr-2 text-yellow-500" />
                          {t('actions.addToFavorites')}
                        </DropdownMenuItem>
                      )}
                      */}

                              {canDeleteFile ? (
                                <DropdownMenuItem
                                  className="text-destructive"
                                  onClick={e => {
                                    e.stopPropagation();
                                    handleDelete(file);
                                  }}
                                  disabled={isDeleting}
                                >
                                  <Trash2 className="h-4 w-4 mr-2 text-destructive" />
                                  {t('actions.delete')}
                                </DropdownMenuItem>
                              ) : null}
                            </DropdownMenuContent>
                          </DropdownMenu>
                        ) : null}
                      </div>
                    </TableCell>
                  )}
                </TableRow>
              );
            })
          )}
        </TableBody>
      </Table>

      {/* Delete Warning Dialog */}
      <DeleteWarningDialog
        open={showDeleteWarning}
        onOpenChange={setShowDeleteWarning}
        fileNames={deleteWarningFileNames}
      />

      {/* Bulk Delete Confirm Dialog */}
      <ConfirmDialog
        open={showBulkDeleteConfirm}
        onOpenChange={setShowBulkDeleteConfirm}
        title={t('delete.bulkConfirmTitle', { count: selectedFiles.length })}
        description={t('delete.bulkConfirmDescription')}
        confirmText={t('actions.delete')}
        cancelText={common('cancel')}
        onConfirm={handleBulkDeleteConfirm}
        loading={isDeleting}
        variant="danger"
      />

      <FilePreviewDialog
        open={Boolean(previewFile)}
        onOpenChange={open => {
          if (!open) setPreviewFile(null);
        }}
        file={previewFile}
        onDownload={
          canDownload
            ? file => {
                void handleDownload(file);
              }
            : undefined
        }
        isDownloading={isDownloading}
      />
      <StartFileParseDialog
        open={Boolean(startParseFile)}
        onOpenChange={open => {
          if (!open) setStartParseFile(null);
        }}
        file={startParseFile}
        loading={Boolean(startParseFile && startingParseFileId === startParseFile.id)}
        onConfirm={file => {
          void handleStartParse(file);
        }}
      />
      <ReplaceDocumentDialog
        open={Boolean(replaceDocumentFile)}
        onOpenChange={open => {
          if (!open) setReplaceDocumentFile(null);
        }}
        file={replaceDocumentFile}
        loading={Boolean(replaceDocumentFile && replacingFileId === replaceDocumentFile.id)}
        onConfirm={(file, replacementFile, processingMode) => {
          void handleReplaceDocument(file, replacementFile, processingMode);
        }}
      />
    </div>
  );
}

export const FileList = memo(FileListBase);
