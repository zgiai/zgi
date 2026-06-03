'use client';

import { useState, useCallback, useRef } from 'react';
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
import { FileUpload, type FileUploadRef } from '@/components/common/file-upload';
import { toast } from 'sonner';
import { RefreshCw } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { FILES_QUERY_KEY } from '@/hooks/use-files';
import { useUploadConfig } from '@/hooks/use-upload';
import type { FileParseProviderKey, FileUploadProcessingMode } from '@/services/types/file';

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
  parseProvider = 'auto',
  onUploadComplete,
}: CreateLocalFileDialogProps) => {
  const t = useT();
  const queryClient = useQueryClient();
  const fileUploadRef = useRef<FileUploadRef>(null);
  const [uploadFilesCount, setUploadFilesCount] = useState(0);
  const [isUploading, setIsUploading] = useState(false);
  const { data: uploadConfig } = useUploadConfig({ enabled: open });
  const maxSizeMB = uploadConfig?.file_size_limit ?? 15;
  const maxCount = uploadConfig?.batch_count_limit ?? 100;

  // Handle file selection change
  const handleFilesChange = useCallback((files: File[]) => {
    setUploadFilesCount(files.length);
  }, []);

  // Handle upload save
  const handleUploadSave = useCallback(async () => {
    const pendingFiles = fileUploadRef.current?.getPendingFiles() ?? [];

    if (pendingFiles.length === 0) {
      toast.error(t('files.messages.noFiles'));
      return;
    }

    if (!fileUploadRef.current) {
      return;
    }

    try {
      setIsUploading(true);
      // Use uploadAll method which handles progress tracking and folder assignment
      const uploadedFiles = await fileUploadRef.current.uploadAll();

      if (uploadedFiles.length > 0) {
        toast.success(t('files.messages.uploadSuccess'));
        // Invalidate files queries to refresh the list
        queryClient.invalidateQueries({ queryKey: [FILES_QUERY_KEY] });
      }

      fileUploadRef.current.clearAll();
      setUploadFilesCount(0);
      setIsUploading(false);
      onOpenChange(false);
      // Call callback after successful upload
      onUploadComplete?.();
    } catch (error) {
      setIsUploading(false);
      const message = (error as { message?: string }).message ?? 'Failed to upload files';
      toast.error(message);
    }
  }, [onOpenChange, onUploadComplete, t, queryClient]);

  // Handle cancel
  const handleCancel = useCallback(() => {
    fileUploadRef.current?.clearAll();
    setUploadFilesCount(0);
    onOpenChange(false);
  }, [onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[600px] max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t('files.upload.uploadFiles')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="py-4">
          <FileUpload
            ref={fileUploadRef}
            autoUpload={false}
            maxCount={maxCount}
            maxSizeMB={maxSizeMB}
            acceptExt={acceptExt}
            folderId={folderId}
            workspaceId={workspaceId}
            processingMode={processingMode}
            parseProvider={parseProvider}
            onFilesChange={handleFilesChange}
          />
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={handleCancel} disabled={isUploading}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleUploadSave} disabled={uploadFilesCount === 0 || isUploading}>
            {isUploading && <RefreshCw className="mr-2 h-4 w-4 animate-spin" />}
            {t('files.upload.confirmUpload')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default CreateLocalFileDialog;
