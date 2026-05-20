'use client';

import type { ReactNode } from 'react';
import {
  AlertCircle,
  Download,
  ExternalLink,
  FileText,
  Image as ImageIcon,
  Loader2,
} from 'lucide-react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import type { FileItem } from '@/services/types/file';
import { useFileOriginalPreviewUrl } from '@/hooks/file/use-file-original-preview-url';
import {
  isOriginalPreviewImage,
  isOriginalPreviewPdf,
  isOriginalPreviewSupported,
  isOriginalPreviewText,
} from '@/utils/file-helpers';

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
 * @description Original file preview dialog for signed PDF and image file URLs.
 * @usage Use in file management lists to preview supported original files.
 * @example
 * <FilePreviewDialog open={open} onOpenChange={setOpen} file={file} />
 */
export function FilePreviewDialog({
  open,
  onOpenChange,
  file,
  onDownload,
  isDownloading = false,
}: FilePreviewDialogProps) {
  const t = useT('files');
  const isSupported = isOriginalPreviewSupported(file?.extension, file?.mime_type);
  const shouldLoadPreview = open && Boolean(file?.id) && isSupported;
  const { previewUrl, isLoading, error } = useFileOriginalPreviewUrl(file?.id, {
    enabled: shouldLoadPreview,
  });

  const extension = file?.extension?.toUpperCase() || '';
  const title = file?.name || t('preview.title');
  const isImage = isOriginalPreviewImage(file?.extension, file?.mime_type);
  const isPdf = isOriginalPreviewPdf(file?.extension, file?.mime_type);
  const isText = isOriginalPreviewText(file?.extension);

  const renderPreview = () => {
    if (!file) {
      return (
        <PreviewMessage
          icon={<AlertCircle className="h-5 w-5" />}
          title={t('preview.noFileSelected')}
        />
      );
    }

    if (!isSupported) {
      return (
        <PreviewMessage
          icon={<AlertCircle className="h-5 w-5" />}
          title={t('preview.unsupportedTitle')}
          description={t('preview.unsupportedDescription')}
        />
      );
    }

    if (isLoading) {
      return (
        <PreviewMessage
          icon={<Loader2 className="h-5 w-5 animate-spin" />}
          title={t('preview.loading')}
        />
      );
    }

    if (error || !previewUrl) {
      return (
        <PreviewMessage
          icon={<AlertCircle className="h-5 w-5" />}
          title={error || t('preview.loadError')}
        />
      );
    }

    if (isImage) {
      return (
        <div className="flex h-full min-h-0 items-center justify-center overflow-auto bg-muted/30 p-4">
          <img src={previewUrl} alt={file.name} className="max-h-full max-w-full object-contain" />
        </div>
      );
    }

    if (isPdf || isText) {
      return (
        <iframe
          src={previewUrl}
          title={file.name}
          className="h-full min-h-[60vh] w-full border-0 bg-background"
        />
      );
    }

    return (
      <PreviewMessage
        icon={<AlertCircle className="h-5 w-5" />}
        title={t('preview.unsupportedTitle')}
        description={t('preview.unsupportedDescription')}
      />
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="full" className="gap-0 overflow-hidden p-0">
        <DialogHeader className="border-b px-5 py-4">
          <div className="flex min-w-0 items-start gap-3 pr-8">
            <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              {isPdf || isText ? (
                <FileText className="h-4 w-4" />
              ) : (
                <ImageIcon className="h-4 w-4" />
              )}
            </div>
            <div className="min-w-0">
              <DialogTitle className="truncate text-base leading-6">{title}</DialogTitle>
              <DialogDescription className="mt-1">
                {extension ? t('preview.fileMeta', { extension }) : t('preview.description')}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <DialogBody className="min-h-0 overflow-hidden p-0">{renderPreview()}</DialogBody>

        <DialogFooter className="border-t px-5 py-3">
          {previewUrl ? (
            <Button variant="outline" asChild>
              <a href={previewUrl} target="_blank" rel="noreferrer">
                <ExternalLink className="mr-2 h-4 w-4" />
                {t('preview.openInNewTab')}
              </a>
            </Button>
          ) : null}
          {file && onDownload ? (
            <Button variant="outline" onClick={() => onDownload(file)} disabled={isDownloading}>
              <Download className="mr-2 h-4 w-4" />
              {t('actions.downloadFile')}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

interface PreviewMessageProps {
  icon: ReactNode;
  title: string;
  description?: string;
}

function PreviewMessage({ icon, title, description }: PreviewMessageProps) {
  return (
    <div className="flex h-full min-h-[360px] items-center justify-center p-6 text-center">
      <div className="max-w-sm">
        <div className="mx-auto mb-3 flex h-10 w-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
          {icon}
        </div>
        <div className="text-sm font-medium text-foreground">{title}</div>
        {description ? (
          <div className="mt-2 text-sm leading-6 text-muted-foreground">{description}</div>
        ) : null}
      </div>
    </div>
  );
}
