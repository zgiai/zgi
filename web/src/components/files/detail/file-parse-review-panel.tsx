'use client';

import { useMemo, useState } from 'react';
import { AlertCircle, Check, Edit3, Loader2, SearchX, ShieldCheck, X } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import type {
  FileParsePreviewConfirmation,
  FileParsePreviewElement,
} from '@/services/types/file';
import {
  useFileParseConfirmationActions,
  useFileParsePreview,
} from '@/hooks/file/use-file-parse-preview';
import { cn } from '@/lib/utils';

interface FileParseReviewPanelProps {
  fileId: string;
  enabled: boolean;
  compact?: boolean;
}

function getConfirmationStatusVariant(status?: string) {
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

function confidenceLabel(value?: number) {
  if (value === undefined || value === null) return '-';
  const normalized = value <= 1 ? value * 100 : value;
  return `${Math.round(normalized)}%`;
}

function elementTitle(element: FileParsePreviewElement) {
  const type = element.subtype ? `${element.type}/${element.subtype}` : element.type;
  return `${type} #${element.ordinal + 1}`;
}

function confirmationContent(confirmation: FileParsePreviewConfirmation) {
  return (
    confirmation.final_content ||
    confirmation.suggested_content ||
    confirmation.original_content ||
    ''
  );
}

function ParseReviewSkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className="rounded-md border border-border bg-background p-4">
          <Skeleton className="h-5 w-36" />
          <Skeleton className="mt-3 h-20 w-full" />
          <div className="mt-3 flex gap-2">
            <Skeleton className="h-8 w-20" />
            <Skeleton className="h-8 w-20" />
          </div>
        </div>
      ))}
    </div>
  );
}

export function FileParseReviewPanel({ fileId, enabled, compact = false }: FileParseReviewPanelProps) {
  const t = useT('files');
  const [editedContentByItem, setEditedContentByItem] = useState<Record<string, string>>({});
  const { data, isLoading, error } = useFileParsePreview(fileId, { enabled });
  const {
    resolveConfirmation,
    batchIgnoreConfirmations,
    isResolving,
    isBatchIgnoring,
  } = useFileParseConfirmationActions(fileId);

  const preview = data?.data;
  const elements = preview?.elements ?? [];
  const pendingItems = useMemo(
    () => (preview?.confirmation_items ?? []).filter(item => item.status === 'pending'),
    [preview?.confirmation_items]
  );
  const isMutating = isResolving || isBatchIgnoring;

  const updateEditedContent = (itemId: string, value: string) => {
    setEditedContentByItem(current => ({ ...current, [itemId]: value }));
  };

  const resolveItem = async (
    confirmation: FileParsePreviewConfirmation,
    action: 'keep' | 'edit' | 'ignore'
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
      <Alert>
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>{t('detail.parseReview.notReadyTitle')}</AlertTitle>
        <AlertDescription>{t('detail.parseReview.notReadyDescription')}</AlertDescription>
      </Alert>
    );
  }

  if (isLoading) return <ParseReviewSkeleton />;

  if (error || !preview) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>{t('detail.parseReview.loadErrorTitle')}</AlertTitle>
        <AlertDescription>{t('detail.parseReview.loadErrorDescription')}</AlertDescription>
      </Alert>
    );
  }

  const content = (
    <div className="space-y-4">
      {!compact ? (
        <div className="rounded-md border border-border bg-background p-4">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div className="min-w-0">
              <h2 className="text-base font-semibold text-foreground">
                {t('detail.parseReview.title')}
              </h2>
              <div className="mt-2 flex flex-wrap gap-2 text-sm text-muted-foreground">
                <Badge variant="outline">
                  {t('detail.parseReview.elementCount', { count: elements.length })}
                </Badge>
                <Badge variant={pendingItems.length > 0 ? 'warning' : 'success'}>
                  {t('detail.parseReview.pendingCount', {
                    count: preview.pending_confirmation_count,
                  })}
                </Badge>
                {preview.engine_used ? <Badge variant="subtle">{preview.engine_used}</Badge> : null}
              </div>
            </div>
            <Button
              variant="outline"
              className="gap-2"
              onClick={() => batchIgnoreConfirmations({})}
              disabled={pendingItems.length === 0 || isMutating}
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
      ) : (
        <div className="flex flex-wrap gap-2 text-sm text-muted-foreground">
          <Badge variant="outline">
            {t('detail.parseReview.elementCount', { count: elements.length })}
          </Badge>
          <Badge variant={pendingItems.length > 0 ? 'warning' : 'success'}>
            {t('detail.parseReview.pendingCount', {
              count: preview.pending_confirmation_count,
            })}
          </Badge>
          {preview.engine_used ? <Badge variant="subtle">{preview.engine_used}</Badge> : null}
        </div>
      )}

      {elements.length === 0 ? (
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
      ) : (
        <div className="space-y-3">
          {elements.map(element => (
            <ParseReviewElement
              key={`${element.id || element.ordinal}`}
              element={element}
              editedContent={editedContentByItem[element.confirmation?.id ?? '']}
              onEditContent={updateEditedContent}
              onResolve={resolveItem}
              disabled={isMutating}
            />
          ))}
        </div>
      )}
    </div>
  );

  return content;
}

function ParseReviewElement({
  element,
  editedContent,
  onEditContent,
  onResolve,
  disabled,
}: {
  element: FileParsePreviewElement;
  editedContent?: string;
  onEditContent: (itemId: string, value: string) => void;
  onResolve: (
    confirmation: FileParsePreviewConfirmation,
    action: 'keep' | 'edit' | 'ignore'
  ) => Promise<void>;
  disabled: boolean;
}) {
  const t = useT('files');
  const confirmation = element.confirmation;
  const content = element.content || confirmation?.original_content || '';
  const isPending = confirmation?.status === 'pending';
  const editValue = confirmation
    ? editedContent ?? confirmationContent(confirmation)
    : '';
  const statusLabel = (() => {
    switch (confirmation?.status) {
      case 'pending':
        return t('detail.parseReview.status.pending');
      case 'kept':
        return t('detail.parseReview.status.kept');
      case 'edited':
        return t('detail.parseReview.status.edited');
      case 'ignored':
        return t('detail.parseReview.status.ignored');
      default:
        return confirmation?.status || '-';
    }
  })();

  return (
    <article
      className={cn(
        'rounded-md border bg-background p-4',
        isPending ? 'border-warning/60 shadow-sm' : 'border-border'
      )}
    >
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-sm font-semibold text-foreground">{elementTitle(element)}</h3>
            <Badge variant="outline">{t('detail.parseReview.page', { page: element.page })}</Badge>
            <Badge variant="subtle">
              {t('detail.parseReview.confidence', {
                value: confidenceLabel(element.confidence),
              })}
            </Badge>
            {confirmation ? (
              <Badge variant={getConfirmationStatusVariant(confirmation.status)}>
                {statusLabel}
              </Badge>
            ) : null}
          </div>
          {confirmation?.review_reason ? (
            <p className="mt-2 text-sm text-warning">{confirmation.review_reason}</p>
          ) : null}
        </div>
      </div>

      <div className="mt-3 whitespace-pre-wrap rounded-md bg-muted/40 p-3 text-sm leading-6 text-foreground">
        {content || t('detail.parseReview.emptyContent')}
      </div>

      {confirmation ? (
        <div className="mt-3 rounded-md border border-border bg-bg-canvas/40 p-3">
          {confirmation.suggested_content ? (
            <div className="mb-3">
              <div className="text-xs font-medium text-muted-foreground">
                {t('detail.parseReview.suggestedContent')}
              </div>
              <div className="mt-1 whitespace-pre-wrap text-sm leading-6 text-foreground">
                {confirmation.suggested_content}
              </div>
            </div>
          ) : null}

          {isPending ? (
            <>
              <Textarea
                value={editValue}
                onChange={event => onEditContent(confirmation.id, event.target.value)}
                className="min-h-28 bg-background"
              />
              <div className="mt-3 flex flex-wrap gap-2">
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
            </>
          ) : (
            <div className="text-sm text-muted-foreground">
              {t('detail.parseReview.resolvedHint')}
            </div>
          )}
        </div>
      ) : null}
    </article>
  );
}
