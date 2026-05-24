'use client';

import type { FileItem } from '@/services/types/file';
import { useFileOriginalPreviewUrl } from '@/hooks/file/use-file-original-preview-url';
import { isOriginalPreviewSupported } from '@/utils/file-helpers';
import { UniversalFilePreviewDialog } from './universal-file-preview-dialog';

export interface FilePreviewDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  file: FileItem | null;
  onDownload?: (file: FileItem) => void;
  isDownloading?: boolean;
}

/**
 * @component FilePreviewDialog
 * @category Feature
 * @status Stable
 * @description File-management adapter for the shared original-file preview dialog.
 */
export function FilePreviewDialog({
  open,
  onOpenChange,
  file,
  onDownload,
  isDownloading = false,
}: FilePreviewDialogProps) {
  const isSupported = isOriginalPreviewSupported(file?.extension, file?.mime_type);
  const shouldLoadPreview = open && Boolean(file?.id) && isSupported;
  const { previewUrl, isLoading, error } = useFileOriginalPreviewUrl(file?.id, {
    enabled: shouldLoadPreview,
  });

  return (
    <UniversalFilePreviewDialog
      open={open}
      onOpenChange={onOpenChange}
      file={
        file
          ? {
              id: file.id,
              name: file.name,
              extension: file.extension,
              mimeType: file.mime_type,
              size: file.size,
            }
          : null
      }
      previewUrl={previewUrl}
      isLoading={isLoading}
      error={error}
      onDownload={file && onDownload ? () => onDownload(file) : undefined}
      isDownloading={isDownloading}
    />
  );
}
