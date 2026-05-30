'use client';

import type { FileItem } from '@/services/types/file';
import { Button } from '@/components/ui/button';
import { Download, Loader2 } from 'lucide-react';
import { useT } from '@/i18n';
import { useFileOriginalPreviewUrl } from '@/hooks/file/use-file-original-preview-url';
import { isOriginalPreviewSupported } from '@/utils/file-helpers';
import { UniversalFilePreviewContent } from '@/components/files/universal-file-preview-dialog';

interface FileOriginalPreviewPanelProps {
  file: FileItem;
  onDownload?: () => void;
  isDownloading?: boolean;
}

export function FileOriginalPreviewPanel({
  file,
  onDownload,
  isDownloading = false,
}: FileOriginalPreviewPanelProps) {
  const t = useT('files');
  const isSupported = isOriginalPreviewSupported(file.extension, file.mime_type);
  const { previewUrl, isLoading, error } = useFileOriginalPreviewUrl(file.id, {
    enabled: isSupported,
  });

  return (
    <div className="min-h-[560px] overflow-hidden rounded-md border border-border bg-background">
      <div className="flex min-h-12 flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div className="min-w-0">
          <div className="truncate text-sm font-medium text-foreground">{file.name}</div>
          <div className="mt-0.5 text-xs text-muted-foreground">
            {t('preview.fileMeta', { extension: file.extension.toUpperCase() })}
          </div>
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
      <div className="h-[calc(100vh-360px)] min-h-[500px] overflow-hidden">
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
