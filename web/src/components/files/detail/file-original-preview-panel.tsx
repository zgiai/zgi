'use client';

import type { FileItem } from '@/services/types/file';
import { Button } from '@/components/ui/button';
import { Download, Loader2 } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { useFileOriginalPreviewUrl } from '@/hooks/file/use-file-original-preview-url';
import { isOriginalPreviewSupported } from '@/utils/file-helpers';
import { UniversalFilePreviewContent } from '@/components/files/universal-file-preview-dialog';

interface FileOriginalPreviewPanelProps {
  file: FileItem;
  onDownload?: () => void;
  isDownloading?: boolean;
  className?: string;
  hideHeader?: boolean;
}

export function FileOriginalPreviewPanel({
  file,
  onDownload,
  isDownloading = false,
  className,
  hideHeader = false,
}: FileOriginalPreviewPanelProps) {
  const t = useT('files');
  const isSupported = isOriginalPreviewSupported(file.extension, file.mime_type);
  const { previewUrl, isLoading, error } = useFileOriginalPreviewUrl(file.id, {
    enabled: isSupported,
  });

  return (
    <div className={cn('flex h-full min-h-0 flex-col overflow-hidden bg-muted/20', className)}>
      {!hideHeader ? (
        <div className="flex min-h-16 flex-wrap items-center justify-between gap-3 border-b bg-background px-4 py-3">
          <div className="min-w-0">
            <span className="inline-flex rounded-full bg-muted px-4 py-2 text-sm font-semibold text-foreground">
              {t('detail.tabs.originalPreview')}
            </span>
          </div>
          {onDownload ? (
            <Button
              variant="outline"
              size="sm"
              className="gap-2"
              onClick={onDownload}
              disabled={isDownloading}
            >
              {isDownloading ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Download className="h-4 w-4" />
              )}
              {t('actions.downloadFile')}
            </Button>
          ) : null}
        </div>
      ) : null}
      <div className="min-h-0 flex-1 overflow-hidden">
        <UniversalFilePreviewContent
          file={{
            id: file.id,
            name: file.name,
            extension: file.extension,
            mimeType: file.mime_type,
            size: file.size,
          }}
          previewUrl={previewUrl}
          isLoading={isLoading}
          error={error}
        />
      </div>
    </div>
  );
}
