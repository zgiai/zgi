'use client';

import { useEffect, useMemo, useRef } from 'react';
import type { FileItem } from '@/services/types/file';
import { Button } from '@/components/ui/button';
import { Download, Loader2 } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { useFileOriginalPreviewUrl } from '@/hooks/file/use-file-original-preview-url';
import { useFileSourcePreviewPages } from '@/hooks/file/use-file-source-preview-pages';
import { isOriginalPreviewSupported } from '@/utils/file-helpers';
import { UniversalFilePreviewContent } from '@/components/files/universal-file-preview-dialog';
import {
  DocumentPagePreview,
  type DocumentPreviewBoundingBox,
  type DocumentPreviewElement,
} from '@/components/document-preview/document-page-preview';

export interface FilePreviewLocator {
  id?: string;
  page?: number;
  bbox?: DocumentPreviewBoundingBox;
  label?: string;
}

interface FileOriginalPreviewPanelProps {
  file: FileItem;
  onDownload?: () => void;
  isDownloading?: boolean;
  className?: string;
  hideHeader?: boolean;
  activeLocator?: FilePreviewLocator | null;
}

export function FileOriginalPreviewPanel({
  file,
  onDownload,
  isDownloading = false,
  className,
  hideHeader = false,
  activeLocator,
}: FileOriginalPreviewPanelProps) {
  const t = useT('files');
  const isSupported = isOriginalPreviewSupported(file.extension, file.mime_type);
  const isPDF = isPDFFile(file);
  const pageRefs = useRef(new Map<number, HTMLDivElement>());
  const sourcePreview = useFileSourcePreviewPages(file.id, {
    enabled: isPDF,
    maxPages: 50,
  });
  const { previewUrl, isLoading, error } = useFileOriginalPreviewUrl(file.id, {
    enabled: isSupported && (!isPDF || Boolean(sourcePreview.error)),
  });
  const previewPages = sourcePreview.data?.data.preview_pages ?? [];
  const activeElement = useMemo(
    () => locatorToPreviewElement(activeLocator),
    [activeLocator]
  );
  const selectedElementId = activeElement ? activeElement.id : undefined;
  const canUseVisualPDFPreview = isPDF && !sourcePreview.error && previewPages.length > 0;
  const isVisualPDFLoading = isPDF && sourcePreview.isLoading && previewPages.length === 0;

  useEffect(() => {
    if (!activeElement || !canUseVisualPDFPreview) {
      return;
    }
    window.setTimeout(() => {
      pageRefs.current.get(pageIndexForLocator(activeElement.page, previewPages.length))?.scrollIntoView({
        block: 'center',
        behavior: 'smooth',
      });
    }, 40);
  }, [activeElement, canUseVisualPDFPreview, previewPages.length]);

  const setPageRef = (pageIndex: number) => (node: HTMLDivElement | null) => {
    if (node) {
      pageRefs.current.set(pageIndex, node);
      return;
    }
    pageRefs.current.delete(pageIndex);
  };

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
        {isVisualPDFLoading ? (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            {t('preview.loading')}
          </div>
        ) : canUseVisualPDFPreview ? (
          <div className="h-full overflow-y-auto overscroll-contain p-3">
            <div className="space-y-5">
              {previewPages.map(page => {
                const pageElement =
                  activeElement && pageIndexForLocator(activeElement.page, previewPages.length) === page.pageIndex
                    ? [activeElement]
                    : [];
                return (
                  <div key={page.pageIndex} ref={setPageRef(page.pageIndex)}>
                    <DocumentPagePreview
                      page={page}
                      elements={pageElement}
                      selectedElementId={selectedElementId}
                      onSelectElement={() => undefined}
                      pageLabel={t('detail.parseReview.page', { page: page.pageIndex + 1 })}
                      boxesLabel={
                        activeElement
                          ? t('detail.parseReview.boxes', { count: pageElement.length })
                          : undefined
                      }
                      formatElementType={() => activeElement?.content || t('detail.chunks.issues.fallback')}
                    />
                  </div>
                );
              })}
            </div>
          </div>
        ) : (
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
        )}
      </div>
    </div>
  );
}

function isPDFFile(file: FileItem) {
  const extension = file.extension.replace(/^\./, '').toLowerCase();
  const mimeType = file.mime_type.toLowerCase();
  return extension === 'pdf' || mimeType === 'application/pdf';
}

function locatorToPreviewElement(locator: FilePreviewLocator | null | undefined): DocumentPreviewElement | null {
  if (!locator?.bbox || !Number.isFinite(locator.page)) {
    return null;
  }
  return {
    id: locator.id || `issue-${locator.page}`,
    type: 'text',
    page: Number(locator.page),
    bbox: locator.bbox,
    ordinal: 0,
    content: locator.label,
  };
}

function pageIndexForLocator(page: number | undefined, renderedPageCount: number) {
  if (!Number.isFinite(page)) {
    return 0;
  }
  const pageNumber = Number(page);
  if (pageNumber >= 1 && pageNumber <= renderedPageCount) {
    return pageNumber - 1;
  }
  return Math.max(pageNumber, 0);
}
