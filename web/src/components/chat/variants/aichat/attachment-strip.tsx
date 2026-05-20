'use client';

import { useState } from 'react';
import { AlertCircle, FileImage, FileText, Loader2, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { useFileOriginalPreviewUrl } from '@/hooks/file/use-file-original-preview-url';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { AIChatInputAttachment } from '@/components/chat/variants/aichat/input-area-types';
import {
  formatFileSize,
  type ScopedTranslatorWithHas,
  tWithFallback,
} from '@/components/chat/variants/aichat/input-area-utils';

interface AIChatDragUploadOverlayProps {
  isSending: boolean;
  isUploading: boolean;
  remainingSlots: number;
  attachmentLimit: number;
  acceptedTypesLabel: string;
}

/**
 * @component AIChatDragUploadOverlay
 * @category Feature
 * @status Stable
 * @description Full-screen drop target feedback for AIChat file uploads.
 * @usage Render while the user is dragging files over the AIChat window
 * @example
 * <AIChatDragUploadOverlay remainingSlots={3} />
 */
export function AIChatDragUploadOverlay({
  isSending,
  isUploading,
  remainingSlots,
  attachmentLimit,
  acceptedTypesLabel,
}: AIChatDragUploadOverlayProps) {
  const t = useT('webapp');
  const isUnavailable = isSending || isUploading || remainingSlots <= 0;

  return (
    <div className="pointer-events-none fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-6 backdrop-blur-sm">
      <div
        className={cn(
          'flex max-w-sm flex-col items-center gap-2 rounded-lg border bg-background px-5 py-4 text-center shadow-lg',
          isUnavailable ? 'border-destructive/40' : 'border-primary/40'
        )}
      >
        <FileText className={cn('size-6', isUnavailable ? 'text-destructive' : 'text-primary')} />
        <div className="text-sm font-medium text-foreground">
          {isSending || isUploading
            ? t('consoleChat.attachments.uploadUnavailable')
            : remainingSlots <= 0
              ? t('consoleChat.attachments.exceedCount', { max: attachmentLimit })
              : t('consoleChat.attachments.dropToUpload')}
        </div>
        <div className="text-xs text-muted-foreground">
          {t('consoleChat.attachments.dropHint', {
            count: remainingSlots,
            types: acceptedTypesLabel,
          })}
        </div>
      </div>
    </div>
  );
}

interface AIChatAttachmentStripProps {
  attachments: AIChatInputAttachment[];
  onRemove: (id: string) => void;
}

/**
 * @component AIChatAttachmentStrip
 * @category Feature
 * @status Stable
 * @description Displays pending AIChat image and document attachments above the composer.
 * @usage Render inside AIChatInputArea when attachments are selected
 * @example
 * <AIChatAttachmentStrip attachments={attachments} onRemove={removeAttachment} />
 */
export function AIChatAttachmentStrip({ attachments, onRemove }: AIChatAttachmentStripProps) {
  if (attachments.length === 0) {
    return null;
  }

  const imageAttachments = attachments.filter(attachment => attachment.kind === 'image');
  const documentAttachments = attachments.filter(attachment => attachment.kind !== 'image');

  return (
    <div className="space-y-2 px-1 pb-2">
      {imageAttachments.length > 0 ? (
        <div className="flex flex-wrap gap-2">
          {imageAttachments.map(attachment => (
            <AIChatImageAttachmentPreview
              key={attachment.id}
              attachment={attachment}
              onRemove={onRemove}
            />
          ))}
        </div>
      ) : null}
      {documentAttachments.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {documentAttachments.map(attachment => (
            <AIChatDocumentAttachmentChip
              key={attachment.id}
              attachment={attachment}
              onRemove={onRemove}
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}

interface AIChatDocumentAttachmentChipProps {
  attachment: AIChatInputAttachment;
  onRemove: (id: string) => void;
}

function AIChatDocumentAttachmentChip({
  attachment,
  onRemove,
}: AIChatDocumentAttachmentChipProps) {
  const t = useT('webapp');

  return (
    <div
      className={cn(
        'group relative flex min-w-0 max-w-56 items-center gap-1.5 overflow-hidden rounded-md border px-2 py-1 text-xs',
        attachment.status === 'error'
          ? 'border-destructive/40 bg-destructive/10 text-destructive'
          : 'border-border bg-muted/40 text-muted-foreground'
      )}
      title={attachment.error || attachment.name}
    >
      {attachment.status === 'uploading' ? (
        <Loader2 className="size-3.5 shrink-0 animate-spin" />
      ) : attachment.status === 'error' ? (
        <AlertCircle className="size-3.5 shrink-0" />
      ) : (
        <FileText className="size-3.5 shrink-0" />
      )}
      <span className="truncate text-foreground">{attachment.name}</span>
      <span className="shrink-0">
        {attachment.status === 'uploading'
          ? `${Math.round(attachment.progress)}%`
          : formatFileSize(attachment.size)}
      </span>
      <Button
        isIcon
        variant="ghost"
        className="size-5 shrink-0 hover:bg-destructive/10 hover:text-destructive"
        onClick={() => onRemove(attachment.id)}
        disabled={attachment.status === 'uploading'}
        title={t('consoleChat.attachments.remove')}
      >
        <Trash2 className="size-3" />
      </Button>
    </div>
  );
}

interface AIChatImageAttachmentPreviewProps {
  attachment: AIChatInputAttachment;
  onRemove: (id: string) => void;
}

/**
 * @component AIChatImageAttachmentPreview
 * @category Feature
 * @status Stable
 * @description Renders an uploaded AIChat image attachment using the signed preview URL endpoint.
 * @usage Used inside AIChatInputArea image attachment strip
 * @example
 * <AIChatImageAttachmentPreview attachment={attachment} onRemove={remove} />
 */
function AIChatImageAttachmentPreview({
  attachment,
  onRemove,
}: AIChatImageAttachmentPreviewProps) {
  const t = useT('webapp');
  const [isPreviewOpen, setIsPreviewOpen] = useState(false);
  const { previewUrl, isLoading, error } = useFileOriginalPreviewUrl(attachment.file?.id, {
    enabled: attachment.status === 'uploaded' && Boolean(attachment.file?.id),
  });
  const isError = attachment.status === 'error' || Boolean(error);
  const canOpenPreview = Boolean(previewUrl || isError);
  const errorMessage =
    attachment.error ||
    error ||
    tWithFallback(
      t as unknown as ScopedTranslatorWithHas,
      'consoleChat.attachments.previewLoadError',
      'consoleChat.streamError'
    );

  return (
    <>
      <div
        role={canOpenPreview ? 'button' : undefined}
        tabIndex={canOpenPreview ? 0 : -1}
        className={cn(
          'group relative size-24 overflow-hidden rounded-lg border bg-muted text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          isError ? 'border-destructive/40' : 'border-border',
          canOpenPreview ? 'cursor-pointer' : 'cursor-default'
        )}
        title={isError ? errorMessage : attachment.name}
        onClick={() => {
          if (canOpenPreview) {
            setIsPreviewOpen(true);
          }
        }}
        onKeyDown={event => {
          if (!canOpenPreview) return;
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            setIsPreviewOpen(true);
          }
        }}
      >
        {previewUrl ? (
          <img src={previewUrl} alt={attachment.name} className="h-full w-full object-cover" />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
            {attachment.status === 'uploading' || isLoading ? (
              <Loader2 className="size-5 animate-spin" />
            ) : isError ? (
              <AlertCircle className="size-5 text-destructive" />
            ) : (
              <FileImage className="size-5" />
            )}
          </div>
        )}
        <Button
          isIcon
          variant="secondary"
          className="absolute right-1 top-1 size-5 rounded-full bg-background/90 p-0 shadow-sm hover:bg-destructive hover:text-destructive-foreground"
          onClick={event => {
            event.stopPropagation();
            onRemove(attachment.id);
          }}
          disabled={attachment.status === 'uploading'}
          title={t('consoleChat.attachments.remove')}
        >
          <Trash2 className="size-3" />
        </Button>
      </div>
      <Dialog open={isPreviewOpen} onOpenChange={setIsPreviewOpen}>
        <DialogContent className="max-h-[90vh] max-w-[90vw] overflow-hidden p-0">
          <DialogHeader className="border-b px-4 py-3">
            <DialogTitle className="truncate text-sm">{attachment.name}</DialogTitle>
          </DialogHeader>
          <div className="flex max-h-[calc(90vh-56px)] min-h-64 items-center justify-center overflow-auto bg-muted/30 p-4">
            {previewUrl ? (
              <img
                src={previewUrl}
                alt={attachment.name}
                className="max-h-[calc(90vh-96px)] max-w-full object-contain"
              />
            ) : (
              <div className="flex max-w-sm flex-col items-center gap-2 text-center text-sm text-muted-foreground">
                <AlertCircle className="size-6 text-destructive" />
                <span>{errorMessage}</span>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
