'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  AlertCircle,
  Check,
  Edit3,
  FileText,
  Loader2,
  SearchX,
  ShieldCheck,
  Sparkles,
  TriangleAlert,
  X,
} from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Textarea } from '@/components/ui/textarea';
import {
  DocumentPagePreview,
  documentPreviewElementKey,
  type DocumentPreviewPage,
} from '@/components/document-preview/document-page-preview';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  FileItem,
  FileParseConfirmationAction,
  FileParsePreviewConfirmation,
  FileParsePreviewElement,
} from '@/services/types/file';
import {
  useFileParseConfirmationActions,
  useFileParsePreview,
} from '@/hooks/file/use-file-parse-preview';
import { useFileSourcePreviewPages } from '@/hooks/file/use-file-source-preview-pages';
import { useFileOriginalPreviewUrl } from '@/hooks/file/use-file-original-preview-url';
import { UniversalFilePreviewContent } from '@/components/files/universal-file-preview-dialog';

interface FileVisualParseReviewPanelProps {
  file: FileItem;
  enabled: boolean;
  canReparse?: boolean;
  onReparse?: () => void;
  isReparsing?: boolean;
}

type FilesTranslator = ((key: string, values?: Record<string, unknown>) => string) & {
  has?: (key: string) => boolean;
};

const reviewReasonTranslationKeys = {
  low_confidence_text: 'detail.parseReview.reviewReasons.lowConfidenceText',
  low_confidence_table: 'detail.parseReview.reviewReasons.lowConfidenceTable',
  low_confidence_image_ocr: 'detail.parseReview.reviewReasons.lowConfidenceImageOcr',
  review_required: 'detail.parseReview.reviewReasons.reviewRequired',
  ocr_fallback: 'detail.parseReview.reviewReasons.ocrFallback',
  local_vlm_fallback: 'detail.parseReview.reviewReasons.vlmFallback',
  table_structure_risk: 'detail.parseReview.reviewReasons.tableStructureRisk',
} as const;

const reviewReasonFallbacks: Record<keyof typeof reviewReasonTranslationKeys, string> = {
  low_confidence_text: '文本识别置信度较低',
  low_confidence_table: '表格识别置信度较低',
  low_confidence_image_ocr: '图片文字识别置信度较低',
  review_required: '需要人工确认',
  ocr_fallback: '已使用 OCR 兜底解析',
  local_vlm_fallback: '已使用视觉模型兜底解析',
  table_structure_risk: '表格结构可能需要确认',
};

function confidenceLabel(value?: number) {
  if (value === undefined || value === null) return '-';
  const normalized = value <= 1 ? value * 100 : value;
  return `${Math.round(normalized)}%`;
}

function confirmationContent(confirmation: FileParsePreviewConfirmation) {
  return (
    confirmation.final_content ||
    confirmation.suggested_content ||
    confirmation.original_content ||
    ''
  );
}

function confirmationStatusVariant(status?: string) {
  switch (status) {
    case 'pending':
      return 'warning' as const;
    case 'edited':
    case 'kept':
      return 'success' as const;
    case 'ignored':
      return 'subtle' as const;
    default:
      return 'outline' as const;
  }
}

function pageBaseForElements(elements: FileParsePreviewElement[], renderedPageCount: number) {
  if (elements.length === 0) return 0;
  const pages = elements.map(element => element.page).filter(page => Number.isFinite(page));
  if (pages.length === 0) return 0;
  const minPage = Math.min(...pages);
  const maxPage = Math.max(...pages);
  return minPage >= 1 && renderedPageCount > 0 && maxPage <= renderedPageCount ? 1 : 0;
}

function pageIndexForElement(elementPage: number, pageBase: number) {
  return Math.max(elementPage - pageBase, 0);
}

function displayPageNumberForElement(
  element: FileParsePreviewElement,
  pageBase: number,
  hasVisualSourcePreview: boolean
) {
  if (hasVisualSourcePreview) {
    return pageIndexForElement(element.page, pageBase) + 1;
  }
  if (!Number.isFinite(element.page)) {
    return 1;
  }
  return Math.max(element.page, 1);
}

function buildPages(
  previewPages: DocumentPreviewPage[],
  elements: FileParsePreviewElement[],
  pageBase: number
): DocumentPreviewPage[] {
  const pages = [...previewPages];
  const maxElementPage = elements.reduce(
    (max, element) => Math.max(max, pageIndexForElement(element.page, pageBase)),
    -1
  );
  const neededCount = Math.max(maxElementPage + 1, previewPages.length, 1);
  for (let index = pages.length; index < neededCount; index += 1) {
    pages.push({
      pageIndex: index,
      aspectRatio: 0.72,
    });
  }
  return pages;
}

function statusLabel(status: string | undefined, t: FilesTranslator) {
  switch (status) {
    case 'pending':
      return t('detail.parseReview.status.pending');
    case 'kept':
      return t('detail.parseReview.status.kept');
    case 'edited':
      return t('detail.parseReview.status.edited');
    case 'ignored':
      return t('detail.parseReview.status.ignored');
    default:
      return status || '-';
  }
}

function elementTypeLabel(type: string | undefined, t: FilesTranslator) {
  const normalized = (type || '').replace(/[_-]/g, '').toLowerCase();
  switch (normalized) {
    case 'title':
      return t('detail.parseReview.types.title');
    case 'heading':
      return t('detail.parseReview.types.heading');
    case 'text':
      return t('detail.parseReview.types.text');
    case 'paragraph':
      return t('detail.parseReview.types.paragraph');
    case 'table':
      return t('detail.parseReview.types.table');
    case 'figure':
      return t('detail.parseReview.types.figure');
    case 'image':
      return t('detail.parseReview.types.image');
    case 'formula':
      return t('detail.parseReview.types.formula');
    case 'list':
      return t('detail.parseReview.types.list');
    case 'listitem':
      return t('detail.parseReview.types.listItem');
    case 'code':
      return t('detail.parseReview.types.code');
    default:
      return type || t('detail.parseReview.types.element');
  }
}

function reviewReasonLabel(reason: string, t: FilesTranslator) {
  const normalized = reason.trim();
  const translationKey = reviewReasonTranslationKeys[
    normalized as keyof typeof reviewReasonTranslationKeys
  ];
  if (!translationKey) return normalized;
  if (!t.has || t.has(translationKey)) {
    return t(translationKey);
  }
  return reviewReasonFallbacks[normalized as keyof typeof reviewReasonFallbacks];
}

function reviewReasonText(reason: string | undefined, t: FilesTranslator) {
  if (!reason) return '';
  return reason
    .split(',')
    .map(item => reviewReasonLabel(item, t))
    .filter(Boolean)
    .join('、');
}

export function FileVisualParseReviewPanel({
  file,
  enabled,
  canReparse = false,
  onReparse,
  isReparsing = false,
}: FileVisualParseReviewPanelProps) {
  const t = useT('files');
  const [selectedElementId, setSelectedElementId] = useState<string | undefined>();
  const [editedContentByItem, setEditedContentByItem] = useState<Record<string, string>>({});
  const elementCardRefs = useRef(new Map<string, HTMLDivElement>());
  const pageRefs = useRef(new Map<number, HTMLDivElement>());
  const { data, isLoading, error } = useFileParsePreview(file.id, { enabled });
  const sourcePreview = useFileSourcePreviewPages(file.id, { enabled, maxPages: 20 });
  const {
    resolveConfirmation,
    batchIgnoreConfirmations,
    isResolving,
    isBatchIgnoring,
  } = useFileParseConfirmationActions(file.id);

  const preview = data?.data;
  const renderedSourcePages = sourcePreview.data?.data.preview_pages;
  const renderedSourcePageCount = renderedSourcePages?.length ?? 0;
  const isSourcePreviewLoading = sourcePreview.isLoading && renderedSourcePageCount === 0;
  const hasVisualSourcePreview = !sourcePreview.error && renderedSourcePageCount > 0;
  const shouldUseOriginalPreviewFallback = !isSourcePreviewLoading && !hasVisualSourcePreview;
  const originalPreview = useFileOriginalPreviewUrl(file.id, {
    enabled: enabled && shouldUseOriginalPreviewFallback,
  });
  const elements = useMemo(
    () => (preview?.elements ?? []).slice().sort((a, b) => a.ordinal - b.ordinal),
    [preview?.elements]
  );
  const pendingElements = useMemo(
    () => elements.filter(element => element.confirmation?.status === 'pending'),
    [elements]
  );
  const pageBase = useMemo(
    () => pageBaseForElements(elements, sourcePreview.data?.data.preview_pages.length ?? 0),
    [elements, sourcePreview.data?.data.preview_pages.length]
  );
  const pages = useMemo(
    () =>
      hasVisualSourcePreview
        ? buildPages(renderedSourcePages ?? [], elements, pageBase)
        : [],
    [elements, hasVisualSourcePreview, pageBase, renderedSourcePages]
  );
  const selectedElement = useMemo(
    () => elements.find(element => documentPreviewElementKey(element) === selectedElementId),
    [elements, selectedElementId]
  );
  const pendingCount = preview?.pending_confirmation_count ?? pendingElements.length;
  const isMutating = isResolving || isBatchIgnoring;

  useEffect(() => {
    if (elements.length === 0) {
      setSelectedElementId(undefined);
      return;
    }
    if (selectedElementId && elements.some(element => documentPreviewElementKey(element) === selectedElementId)) {
      return;
    }
    const firstPendingWithBox = pendingElements.find(element => element.bbox);
    const firstWithBox = elements.find(element => element.bbox);
    const nextElement = firstPendingWithBox || firstWithBox || elements[0];
    setSelectedElementId(documentPreviewElementKey(nextElement));
  }, [elements, pendingElements, selectedElementId]);

  const setElementCardRef = useCallback(
    (key: string) => (node: HTMLDivElement | null) => {
      if (node) {
        elementCardRefs.current.set(key, node);
        return;
      }
      elementCardRefs.current.delete(key);
    },
    []
  );

  const setPageRef = useCallback(
    (pageIndex: number) => (node: HTMLDivElement | null) => {
      if (node) {
        pageRefs.current.set(pageIndex, node);
        return;
      }
      pageRefs.current.delete(pageIndex);
    },
    []
  );

  const scrollToElementCard = useCallback((key: string) => {
    window.setTimeout(() => {
      elementCardRefs.current.get(key)?.scrollIntoView({
        block: 'center',
        behavior: 'smooth',
      });
    }, 60);
  }, []);

  const scrollToPreviewPage = useCallback((element: FileParsePreviewElement) => {
    window.setTimeout(() => {
      pageRefs.current.get(pageIndexForElement(element.page, pageBase))?.scrollIntoView({
        block: 'center',
        behavior: 'smooth',
      });
    }, 40);
  }, [pageBase]);

  const selectElement = useCallback(
    (element: FileParsePreviewElement, source: 'preview' | 'list') => {
      const key = documentPreviewElementKey(element);
      setSelectedElementId(key);
      if (source === 'preview') {
        scrollToElementCard(key);
        return;
      }
      scrollToPreviewPage(element);
    },
    [scrollToElementCard, scrollToPreviewPage]
  );

  const jumpToNextPending = () => {
    if (pendingElements.length === 0) return;
    const currentIndex = selectedElement
      ? pendingElements.findIndex(
          element => documentPreviewElementKey(element) === documentPreviewElementKey(selectedElement)
        )
      : -1;
    const next = pendingElements[(currentIndex + 1 + pendingElements.length) % pendingElements.length];
    selectElement(next, 'list');
  };

  const updateEditedContent = (itemId: string, value: string) => {
    setEditedContentByItem(current => ({ ...current, [itemId]: value }));
  };

  const resolveItem = async (
    confirmation: FileParsePreviewConfirmation,
    action: FileParseConfirmationAction
  ) => {
    const finalContent =
      action === 'edit'
        ? editedContentByItem[confirmation.id] ?? confirmationContent(confirmation)
        : undefined;
    await resolveConfirmation({
      itemId: confirmation.id,
      action,
      finalContent,
    });
  };

  if (!enabled) {
    return (
      <Alert className="m-4">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>{t('detail.parseReview.notReadyTitle')}</AlertTitle>
        <AlertDescription>{t('detail.parseReview.notReadyDescription')}</AlertDescription>
      </Alert>
    );
  }

  if (isLoading) {
    return <FileVisualParseReviewSkeleton />;
  }

  if (error || !preview) {
    return (
      <Alert variant="destructive" className="m-4">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>{t('detail.parseReview.loadErrorTitle')}</AlertTitle>
        <AlertDescription>{t('detail.parseReview.loadErrorDescription')}</AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="grid min-h-[620px] xl:grid-cols-[minmax(0,1fr)_minmax(420px,0.95fr)]">
      <section className="min-w-0 border-r bg-muted/20">
        <div className="flex min-h-12 items-center justify-between gap-3 border-b bg-background px-4 py-2">
          <Badge variant="subtle" className="px-3 py-1.5 text-sm font-semibold text-foreground">
            {t('detail.tabs.originalPreview')}
          </Badge>
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            {isSourcePreviewLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            <span>
              {hasVisualSourcePreview
                ? t('detail.parseReview.sourcePageCount', {
                    count: pages.length,
                  })
                : t('detail.parseReview.sourcePreviewFallback')}
            </span>
          </div>
        </div>
        <div
          className={cn(
            'h-[calc(100vh-430px)] min-h-[560px]',
            hasVisualSourcePreview ? 'overflow-y-auto p-3' : 'overflow-hidden'
          )}
        >
          {isSourcePreviewLoading ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              {t('preview.loading')}
            </div>
          ) : shouldUseOriginalPreviewFallback ? (
            <div className="flex h-full min-h-0 flex-col">
              <div className="min-h-0 flex-1">
                <UniversalFilePreviewContent
                  file={{
                    id: file.id,
                    name: file.name,
                    extension: file.extension,
                    mimeType: file.mime_type,
                    size: file.size,
                  }}
                  previewUrl={originalPreview.previewUrl}
                  isLoading={originalPreview.isLoading}
                  error={originalPreview.error}
                />
              </div>
            </div>
          ) : elements.length === 0 ? (
            <div className="p-4">
              <EmptyPreviewState />
            </div>
          ) : (
            <div className="space-y-5">
              {pages.map(page => {
                const pageElements = elements.filter(
                  element => pageIndexForElement(element.page, pageBase) === page.pageIndex
                );
                return (
                  <div key={page.pageIndex} ref={setPageRef(page.pageIndex)}>
                    <DocumentPagePreview
                      page={page}
                      elements={pageElements}
                      selectedElementId={selectedElementId}
                      onSelectElement={element => selectElement(element, 'preview')}
                      pageLabel={t('detail.parseReview.page', { page: page.pageIndex + 1 })}
                      boxesLabel={t('detail.parseReview.boxes', {
                        count: pageElements.filter(element => element.bbox).length,
                      })}
                      formatElementType={type => elementTypeLabel(type, t as FilesTranslator)}
                    />
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </section>

      <section className="min-w-0 bg-background">
        <div className="flex min-h-12 flex-wrap items-center justify-between gap-3 border-b px-4 py-2">
          <Badge variant="subtle" className="px-3 py-1.5 text-sm font-semibold text-foreground">
            {t('detail.workbench.steps.parsed')}
          </Badge>
          {canReparse ? (
            <Button
              variant="outline"
              className="h-8 gap-1.5 rounded-md px-3 text-sm"
              onClick={onReparse}
              disabled={isReparsing}
            >
              {isReparsing ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Sparkles className="h-4 w-4" />
              )}
              {t('detail.reparse.action')}
            </Button>
          ) : null}
        </div>

        <div className="max-h-[calc(100vh-430px)] min-h-[560px] overflow-y-auto p-3">
          {pendingCount > 0 ? (
            <div className="mb-3 rounded-md border border-warning/30 bg-warning/5 p-3">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex min-w-0 gap-2.5">
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-warning/10 text-warning">
                    <TriangleAlert className="h-4 w-4" />
                  </span>
                  <div className="min-w-0">
                    <div className="text-sm font-semibold text-foreground">
                      {t('detail.parseReview.pendingReviewTitle', { count: pendingCount })}
                    </div>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {t('detail.parseReview.pendingReviewDescription')}
                    </p>
                  </div>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={jumpToNextPending}
                    disabled={pendingElements.length === 0}
                  >
                    {t('detail.parseReview.jumpNext')}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => void batchIgnoreConfirmations({})}
                    disabled={pendingElements.length === 0 || isMutating}
                  >
                    {isBatchIgnoring ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <ShieldCheck className="h-4 w-4" />
                    )}
                    {t('detail.parseReview.batchIgnore')}
                  </Button>
                </div>
              </div>
            </div>
          ) : null}

          {elements.length === 0 ? (
            <EmptyReviewState />
          ) : (
            <div className="space-y-2.5">
              {elements.map(element => {
                const key = documentPreviewElementKey(element);
                return (
                  <div key={key} ref={setElementCardRef(key)}>
                    <VisualReviewCard
                      element={element}
                      pageNumber={displayPageNumberForElement(
                        element,
                        pageBase,
                        hasVisualSourcePreview
                      )}
                      selected={key === selectedElementId}
                      editedContent={editedContentByItem[element.confirmation?.id ?? '']}
                      onEditContent={updateEditedContent}
                      onResolve={resolveItem}
                      onSelect={() => selectElement(element, 'list')}
                      disabled={isMutating}
                    />
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </section>
    </div>
  );
}

function VisualReviewCard({
  element,
  pageNumber,
  selected,
  editedContent,
  onEditContent,
  onResolve,
  onSelect,
  disabled,
}: {
  element: FileParsePreviewElement;
  pageNumber: number;
  selected: boolean;
  editedContent?: string;
  onEditContent: (itemId: string, value: string) => void;
  onResolve: (
    confirmation: FileParsePreviewConfirmation,
    action: FileParseConfirmationAction
  ) => Promise<void>;
  onSelect: () => void;
  disabled: boolean;
}) {
  const t = useT('files');
  const confirmation = element.confirmation;
  const content = element.content || confirmation?.original_content || '';
  const isPending = confirmation?.status === 'pending';
  const editValue = confirmation
    ? editedContent ?? confirmationContent(confirmation)
    : '';
  const reasonText = reviewReasonText(confirmation?.review_reason, t as FilesTranslator);

  return (
    <article
      className={cn(
        'rounded-md border bg-background p-3 transition-all',
        selected ? 'border-primary shadow-sm ring-2 ring-primary/15' : 'border-border',
        isPending && !selected && 'border-warning/40'
      )}
    >
      <button type="button" className="block w-full text-left" onClick={onSelect}>
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant="outline" className="px-2 py-0.5 text-xs">
            {elementTypeLabel(element.type, t as FilesTranslator)}
          </Badge>
          <span className="text-xs text-muted-foreground">
            {t('detail.parseReview.page', { page: pageNumber })}
          </span>
          <span className="min-w-0 truncate text-sm font-semibold text-foreground">
            {element.subtype || element.content?.split('\n')[0] || element.type}
          </span>
          <span className="ml-auto" />
          {confirmation ? (
            <Badge variant={confirmationStatusVariant(confirmation.status)}>
              {statusLabel(confirmation.status, t as FilesTranslator)}
            </Badge>
          ) : null}
        </div>
      </button>

      <div
        className={cn(
          'mt-2.5 whitespace-pre-wrap rounded-md bg-muted/35 p-2.5 text-[13px] leading-5 text-foreground',
          selected && isPending && 'border border-primary bg-background'
        )}
      >
        {content || t('detail.parseReview.emptyContent')}
      </div>

      {isPending && reasonText ? (
        <div className="mt-2.5 flex gap-2 rounded-md border border-warning/20 bg-warning/5 p-2.5 text-xs text-warning">
          <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>{reasonText}</span>
        </div>
      ) : null}

      <div className="mt-2.5 flex flex-wrap gap-1.5 text-xs text-muted-foreground">
        <Badge variant="subtle" className="px-2 py-0.5 text-xs">
          {t('detail.parseReview.confidence', { value: confidenceLabel(element.confidence) })}
        </Badge>
        {element.bbox ? (
          <Badge variant="subtle" className="px-2 py-0.5 text-xs">
            {t('detail.parseReview.hasLocation')}
          </Badge>
        ) : null}
      </div>

      {confirmation && isPending && selected ? (
        <div className="mt-2.5 rounded-md border border-border bg-bg-canvas/40 p-2.5">
          {confirmation.suggested_content ? (
            <div className="mb-2.5">
              <div className="text-xs font-medium text-muted-foreground">
                {t('detail.parseReview.suggestedContent')}
              </div>
              <div className="mt-1 whitespace-pre-wrap text-[13px] leading-5 text-foreground">
                {confirmation.suggested_content}
              </div>
            </div>
          ) : null}
          <Textarea
            value={editValue}
            onChange={event => onEditContent(confirmation.id, event.target.value)}
            className="min-h-28 bg-background text-sm leading-6"
          />
          <div className="mt-3 flex flex-wrap justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              className="gap-2"
              onClick={() => void onResolve(confirmation, 'keep')}
              disabled={disabled}
            >
              <Check className="h-4 w-4" />
              {t('detail.parseReview.keep')}
            </Button>
            <Button
              size="sm"
              className="gap-2"
              onClick={() => void onResolve(confirmation, 'edit')}
              disabled={disabled || editValue.trim() === ''}
            >
              <Edit3 className="h-4 w-4" />
              {t('detail.parseReview.saveEdit')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="gap-2 text-muted-foreground"
              onClick={() => void onResolve(confirmation, 'ignore')}
              disabled={disabled}
            >
              <X className="h-4 w-4" />
              {t('detail.parseReview.ignore')}
            </Button>
          </div>
        </div>
      ) : null}
    </article>
  );
}

function FileVisualParseReviewSkeleton() {
  return (
    <div className="grid min-h-[620px] xl:grid-cols-[minmax(0,1fr)_minmax(420px,0.95fr)]">
      <div className="space-y-4 border-r bg-muted/20 p-4">
        <Skeleton className="h-10 w-28" />
        <Skeleton className="h-[560px] w-full" />
      </div>
      <div className="space-y-4 p-4">
        <Skeleton className="h-28 w-full" />
        {Array.from({ length: 4 }).map((_, index) => (
          <Skeleton key={index} className="h-32 w-full" />
        ))}
      </div>
    </div>
  );
}

function EmptyPreviewState() {
  const t = useT('files');
  return (
    <div className="flex min-h-[360px] items-center justify-center rounded-md border border-dashed bg-background p-6 text-center">
      <div>
        <FileText className="mx-auto h-8 w-8 text-muted-foreground" />
        <div className="mt-3 text-sm font-medium text-foreground">
          {t('detail.parseReview.emptyTitle')}
        </div>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('detail.parseReview.emptyDescription')}
        </p>
      </div>
    </div>
  );
}

function EmptyReviewState() {
  const t = useT('files');
  return (
    <div className="flex min-h-[280px] items-center justify-center rounded-md border border-dashed border-border bg-background p-6 text-center">
      <div>
        <SearchX className="mx-auto h-8 w-8 text-muted-foreground" />
        <div className="mt-3 text-sm font-medium text-foreground">
          {t('detail.parseReview.emptyTitle')}
        </div>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('detail.parseReview.emptyDescription')}
        </p>
      </div>
    </div>
  );
}
