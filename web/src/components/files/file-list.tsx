import { memo, useState, type ComponentType, type KeyboardEvent } from 'react';
import Link from 'next/link';
import {
  MoreVertical,
  FileIcon,
  FileText,
  FileSpreadsheet,
  Image as ImageIcon,
  Video,
  FileArchive,
  FileCode,
  FileMusic,
  FileAudio,
  Download,
  Eye,
  Trash2,
  CalendarDays,
  HardDrive,
  Link2,
  MoveRight,
  Activity,
  Info,
  FileSearch,
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
import { Progress } from '@/components/ui/progress';
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
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { WorkspaceAssetMoveDialog } from '@/components/common/workspace-asset-move-dialog';
import { fileManageService } from '@/services/file-manage.service';

function getProcessingStatus(file: FileItem): string {
  return file.processing_status || 'stored_only';
}

function getProcessingBadgeVariant(status: string) {
  switch (status) {
    case 'ready':
      return 'success' as const;
    case 'parsing':
    case 'confirming':
    case 'generating':
      return 'info' as const;
    case 'parse_failed':
      return 'destructive' as const;
    case 'stored_only':
    default:
      return 'subtle' as const;
  }
}

function isProcessingActive(status: string) {
  return status === 'parsing' || status === 'confirming' || status === 'generating';
}

function FileProcessingStatus({ file, compact = false }: { file: FileItem; compact?: boolean }) {
  const { files: t } = useT();
  const status = getProcessingStatus(file);
  const progress = file.processing_progress ?? 0;
  const pendingCount = file.pending_confirmation_count ?? 0;
  const chunkCount = file.chunk_count ?? 0;
  const embeddingCount = file.embedding_count ?? 0;
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
    <div className="min-w-0 space-y-1.5">
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <Badge
          variant={getProcessingBadgeVariant(status)}
          className={cn(compact ? 'px-2 py-0 text-[10px]' : 'px-2 py-0.5 text-[11px]')}
        >
          {statusLabel}
        </Badge>
        {isProcessingActive(status) ? (
          <span className="text-[11px] font-medium text-muted-foreground">{progress}%</span>
        ) : null}
      </div>
      {isProcessingActive(status) ? (
        <Progress className="h-1.5 w-20 max-w-full" value={progress} />
      ) : null}
      {!compact ? (
        <div className="flex min-w-0 flex-wrap gap-x-2 gap-y-0.5 text-[11px] text-muted-foreground">
          {pendingCount > 0 ? (
            <span>{t('fileList.pendingCount', { count: pendingCount })}</span>
          ) : null}
          {chunkCount > 0 ? <span>{t('fileList.chunkCount', { count: chunkCount })}</span> : null}
          {embeddingCount > 0 ? (
            <span>{t('fileList.embeddingCount', { count: embeddingCount })}</span>
          ) : null}
          {file.last_error_message ? (
            <span className="max-w-[220px] truncate text-destructive">
              {file.last_error_message}
            </span>
          ) : null}
        </div>
      ) : null}
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

/**
 * Get file type icon component and color
 */
function getFileTypeConfig(extension: string): {
  Icon: ComponentType<{ className?: string }>;
  color: string;
} {
  const ext = extension.toLowerCase();

  const configs: Record<string, { Icon: ComponentType<{ className?: string }>; color: string }> = {
    // Documents
    pdf: { Icon: FileText, color: 'text-red-600' },
    doc: { Icon: FileText, color: 'text-blue-600' },
    docx: { Icon: FileText, color: 'text-blue-600' },
    txt: { Icon: FileText, color: 'text-gray-600' },
    // Spreadsheets
    xls: { Icon: FileSpreadsheet, color: 'text-green-600' },
    xlsx: { Icon: FileSpreadsheet, color: 'text-green-600' },
    csv: { Icon: FileSpreadsheet, color: 'text-orange-600' },
    // Images
    jpg: { Icon: ImageIcon, color: 'text-yellow-600' },
    jpeg: { Icon: ImageIcon, color: 'text-yellow-600' },
    png: { Icon: ImageIcon, color: 'text-purple-600' },
    gif: { Icon: ImageIcon, color: 'text-pink-600' },
    webp: { Icon: ImageIcon, color: 'text-pink-600' },
    svg: { Icon: ImageIcon, color: 'text-indigo-600' },
    // Video
    mp4: { Icon: Video, color: 'text-pink-600' },
    avi: { Icon: Video, color: 'text-pink-600' },
    mov: { Icon: Video, color: 'text-pink-600' },
    wmv: { Icon: Video, color: 'text-pink-600' },
    // Audio
    mp3: { Icon: FileMusic, color: 'text-indigo-600' },
    wav: { Icon: FileAudio, color: 'text-indigo-600' },
    // Archives
    zip: { Icon: FileArchive, color: 'text-orange-600' },
    rar: { Icon: FileArchive, color: 'text-orange-600' },
    '7z': { Icon: FileArchive, color: 'text-orange-600' },
    // Code
    js: { Icon: FileCode, color: 'text-yellow-600' },
    ts: { Icon: FileCode, color: 'text-blue-600' },
    jsx: { Icon: FileCode, color: 'text-blue-600' },
    tsx: { Icon: FileCode, color: 'text-blue-600' },
    json: { Icon: FileCode, color: 'text-gray-600' },
  };

  return configs[ext] || { Icon: FileIcon, color: 'text-gray-600' };
}

function FileListBase({
  files,
  maxCount,
  total,
  selectedFiles = [],
  onSelectionChange,
  isLoading = false,
  selectionMode = false,
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
  const [workspaceMoveFile, setWorkspaceMoveFile] = useState<FileItem | null>(null);
  const [reparsingFileId, setReparsingFileId] = useState<string | null>(null);
  const { currentOrganization } = useOrganizations();

  // Permission checks
  const { hasPermission } = useAccountPermissions();
  const canDownload = hasPermission('file.download');
  const canManage = hasPermission('file.manage');
  const canUpload = hasPermission('file.upload_create');
  const canMoveAssets = ['owner', 'admin'].includes(currentOrganization?.organization_role ?? '');
  const canViewDetail = !selectionMode;
  const hasAnyAction = canViewDetail || canDownload || canManage || canMoveAssets;
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
    if (selectedFiles.length === 0 || isDeleting) return;
    setShowBulkDeleteConfirm(true);
  };

  const handleBulkDeleteConfirm = async () => {
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
        <div className="flex items-center justify-between border-b px-3 py-2.5">
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
                const { Icon, color } = getFileTypeConfig(file.extension);

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
                          <Icon className={cn('h-5 w-5', color)} />
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
                            {file.related_count > 0 ? (
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
                        {canViewDetail ? (
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
                        {canDownload &&
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
                          {file.related_count > 0
                            ? t('fileList.relatedCount', { count: file.related_count })
                            : t('fileList.notRelated')}
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

        <FilePreviewDialog
          open={Boolean(previewFile)}
          onOpenChange={open => {
            if (!open) setPreviewFile(null);
          }}
          file={previewFile}
          onDownload={file => {
            void handleDownload(file);
          }}
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
            <Button
              type="button"
              size="sm"
              className="h-8 rounded-md px-3 text-xs"
              disabled
              title={t('actions.batchUnavailable')}
            >
              <FileSearch className="h-4 w-4" />
              {t('actions.batchParse')}
              {selectedStoredOnlyCount > 0 ? (
                <span className="ml-1 text-[11px] opacity-80">{selectedStoredOnlyCount}</span>
              ) : null}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-8 rounded-md px-3 text-xs shadow-none"
              disabled
              title={t('actions.batchUnavailable')}
            >
              <MoveRight className="h-4 w-4" />
              {t('actions.batchMove')}
            </Button>
            {canManage ? (
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
      <Table className="table-fixed" containerClassName="overflow-y-auto flex-1 relative">
        <colgroup>
          <col style={{ width: 44 }} />
          <col style={{ width: 260 }} />
          <col style={{ width: 90 }} />
          <col style={{ width: 100 }} />
          <col style={{ width: 240 }} />
          <col style={{ width: 140 }} />
          <col style={{ width: 160 }} />
          {hasAnyAction ? <col style={{ width: 170 }} /> : null}
        </colgroup>
        <TableHeader className="sticky top-0 z-10 bg-muted/30 backdrop-blur">
          <TableRow className="h-11 hover:bg-muted/30">
            <TableHead className="px-3">
              <Checkbox
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
            <TableHead className="text-sm">{t('fileList.fileName')}</TableHead>
            <TableHead className="text-sm">{t('fileList.fileType')}</TableHead>
            <TableHead className="text-sm">{t('fileList.fileSize')}</TableHead>
            <TableHead className="text-sm">{t('fileList.processingStatus')}</TableHead>
            <TableHead className="text-sm">{t('fileList.relatedStatus')}</TableHead>
            <TableHead className="text-sm">{t('fileList.uploadDate')}</TableHead>
            {hasAnyAction && (
              <TableHead className="text-right text-sm">{t('fileList.actions')}</TableHead>
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
                {/* Related status badge */}
                <TableCell className="text-sm">
                  <div className="h-6 w-16 bg-muted animate-pulse rounded-full" />
                </TableCell>
                {/* Processing status */}
                <TableCell className="text-sm">
                  <div className="h-6 w-24 bg-muted animate-pulse rounded-full" />
                </TableCell>
                {/* Upload date */}
                <TableCell className="text-sm text-muted-foreground">
                  <div className="h-4 w-24 bg-muted animate-pulse rounded" />
                </TableCell>
                {/* Actions button */}
                {hasAnyAction && (
                  <TableCell className="text-right">
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
              const processingStatus = getProcessingStatus(file);

              return (
                <TableRow
                  key={file.id}
                  className={cn(
                    'h-14 cursor-pointer transition-colors hover:bg-muted/30',
                    processingStatus === 'confirming' &&
                      'bg-amber-50/40 shadow-[inset_2px_0_0_rgb(245,158,11)] hover:bg-amber-50/60',
                    processingStatus === 'parse_failed' &&
                      'bg-red-50/40 shadow-[inset_2px_0_0_rgb(239,68,68)] hover:bg-red-50/60',
                    selectedFiles.includes(file.id) &&
                      'bg-primary/5 shadow-[inset_2px_0_0_hsl(var(--primary))]'
                  )}
                  onClick={() => handleRowClick(file)}
                >
                  <TableCell className="px-3">
                    <Checkbox
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
                      {(() => {
                        const { Icon, color } = getFileTypeConfig(file.extension);
                        return <Icon className={cn('h-5 w-5 flex-shrink-0', color)} />;
                      })()}
                      {canViewDetail ? (
                        <Link
                          href={`/console/files/${file.id}`}
                          className="truncate text-sm font-medium text-foreground underline-offset-4 hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30"
                          onClick={event => event.stopPropagation()}
                        >
                          {file.name}
                        </Link>
                      ) : (
                        <span className="truncate text-sm font-medium text-foreground">
                          {file.name}
                        </span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-sm text-foreground">{file.extension}</TableCell>
                  <TableCell className="text-sm text-foreground">
                    {formatFileSize(file.size)}
                  </TableCell>
                  <TableCell className="text-sm">
                    <FileProcessingStatus file={file} />
                  </TableCell>
                  <TableCell className="text-sm">
                    {file.related_count > 0 ? (
                      <RelatedResourcesPopover fileId={file.id} relatedCount={file.related_count}>
                        <span className="inline-flex cursor-pointer items-center rounded-full bg-primary/10 px-2 py-1 text-xs font-medium text-primary transition-colors hover:bg-primary/15">
                          {t('fileList.relatedCount', { count: file.related_count })}
                        </span>
                      </RelatedResourcesPopover>
                    ) : (
                      <span className="inline-flex items-center rounded-full px-2 py-1 text-xs font-medium bg-muted">
                        {t('fileList.notRelated')}
                      </span>
                    )}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(new Date(file.created_at).getTime() - 8 * 60 * 60 * 1000)}
                  </TableCell>
                  {hasAnyAction && (
                    <TableCell className="text-right">
                      <div className="flex min-w-0 items-center justify-end gap-1.5">
                        {canViewDetail && processingStatus === 'confirming' ? (
                          <Button
                            asChild
                            variant="outline"
                            size="sm"
                            className="h-8 rounded-md px-3 text-xs"
                            onClick={event => event.stopPropagation()}
                          >
                            <Link href={`/console/files/${file.id}`}>
                              {t('actions.confirmParse')}
                            </Link>
                          </Button>
                        ) : null}
                        {processingStatus === 'parse_failed' ? (
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            className="h-8 rounded-md px-3 text-xs"
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
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-8 w-8 p-0"
                              onClick={e => e.stopPropagation()}
                            >
                              <MoreVertical className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            {canViewDetail ? (
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
                            {canDownload && (
                              <>
                                {isOriginalPreviewSupported(file.extension, file.mime_type) ? (
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
                              </>
                            )}

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

                            {canManage && !selectionMode && (
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
                            )}
                            {canMoveAssets && !selectionMode && (
                              <DropdownMenuItem
                                onClick={e => {
                                  e.stopPropagation();
                                  setWorkspaceMoveFile(file);
                                }}
                              >
                                <MoveRight className="h-4 w-4 mr-2" />
                                {common('assetMove.title')}
                              </DropdownMenuItem>
                            )}
                          </DropdownMenuContent>
                        </DropdownMenu>
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
        onDownload={file => {
          void handleDownload(file);
        }}
        isDownloading={isDownloading}
      />
      <WorkspaceAssetMoveDialog
        open={Boolean(workspaceMoveFile)}
        onOpenChange={open => {
          if (!open) setWorkspaceMoveFile(null);
        }}
        assetType="file"
        assetId={workspaceMoveFile?.id ?? ''}
        assetName={workspaceMoveFile?.name}
      />
    </div>
  );
}

export const FileList = memo(FileListBase);
