'use client';

import { useEffect, useState } from 'react';
import { AlertCircle, FileImage, FileText, Loader2, RotateCcw, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Progress } from '@/components/ui/progress';
import { useFileOriginalPreviewUrl } from '@/hooks/file/use-file-original-preview-url';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { AIChatInputAttachment } from '@/components/chat/variants/aichat/input-area-types';
import {
  formatFileSize,
  type ScopedTranslatorWithHas,
  tWithFallback,
} from '@/components/chat/variants/aichat/input-area-utils';

/**
 * @function clampUploadPercent
 * @category Utility
 * @description Normalizes a tracked upload progress value into a 0-100 integer so the
 * attachment UI never renders an out-of-range percentage.
 * @usage Use when rendering attachment.progress
 * @example
 * const percent = clampUploadPercent(attachment.progress);
 */
function clampUploadPercent(progress: number | undefined): number {
  if (typeof progress !== 'number' || !Number.isFinite(progress)) return 0;
  return Math.min(100, Math.max(0, Math.round(progress)));
}

// The browser can only observe the transfer to our API, which the progress callbacks
// cap here. After the body is sent the request waits on server-side storage, so the
// indicator would otherwise sit at 98% for the whole storage write.
const UPLOAD_TRANSFER_CAP = 98;
// Never claim completion before the server confirms; stop one point short.
const UPLOAD_SETTLE_CAP = 99;
// Hold the transfer cap for 5s before creeping, so an ordinary server-side store
// finishes without the number ever moving past 98%.
const UPLOAD_SETTLE_STEP_MS = 5000;

/**
 * @function useUploadPercent
 * @category Hook
 * @description Percentage to render for an attachment. Real transfer progress wins
 * while the body is uploading; once it parks at the transfer cap the value creeps
 * towards the settle cap so a slow server-side store never looks frozen.
 * @usage Use in place of reading attachment.progress directly
 * @example
 * const percent = useUploadPercent(attachment.progress, isUploading);
 */
function useUploadPercent(progress: number | undefined, isUploading: boolean): number {
  const transferred = clampUploadPercent(progress);
  const isSettling = isUploading && transferred >= UPLOAD_TRANSFER_CAP;
  const [settleSteps, setSettleSteps] = useState(0);

  useEffect(() => {
    // Reset when the attempt ends or restarts, so a retry starts the creep over.
    if (!isSettling) {
      setSettleSteps(0);
      return;
    }

    // Creeping from the transfer cap to the settle cap is a single step, so schedule
    // it once instead of polling. The callback checks the wall clock rather than
    // trusting the timer: browsers throttle timers in hidden tabs (down to roughly
    // once a minute), and visibilitychange catches up as soon as the tab is visible
    // again, so a returning user never sees a frozen number.
    const startedAt = Date.now();
    const settleStep = UPLOAD_SETTLE_CAP - UPLOAD_TRANSFER_CAP;
    let timer = 0;

    const sync = () => {
      if (Date.now() - startedAt < UPLOAD_SETTLE_STEP_MS) return;
      setSettleSteps(settleStep);
      window.clearTimeout(timer);
      timer = 0;
    };

    timer = window.setTimeout(sync, UPLOAD_SETTLE_STEP_MS);
    document.addEventListener('visibilitychange', sync);
    return () => {
      window.clearTimeout(timer);
      document.removeEventListener('visibilitychange', sync);
    };
  }, [isSettling]);

  if (!isSettling) return transferred;
  // Never report less than the real value, so a late progress event cannot move
  // the indicator backwards.
  return Math.max(transferred, UPLOAD_TRANSFER_CAP + settleSteps);
}

interface AttachmentUploadBarProps {
  percent: number;
  className?: string;
}

/**
 * @component AttachmentUploadBar
 * @category Feature
 * @description Thin progress bar pinned to the bottom edge of an uploading attachment.
 * Wraps the shared Progress primitive, so the bar carries a progressbar role and an
 * accessible value instead of being a decorative element.
 * @usage Render while attachment.status === 'uploading'
 * @example
 * <AttachmentUploadBar percent={percent} className="h-1" />
 */
function AttachmentUploadBar({ percent, className }: AttachmentUploadBarProps) {
  const t = useT('webapp');

  return (
    <Progress
      value={percent}
      aria-label={t('consoleChat.attachments.uploading')}
      className={cn(
        'pointer-events-none absolute inset-x-0 bottom-0 h-1 rounded-none bg-muted-foreground/20',
        className
      )}
    />
  );
}

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
  onRetry: (id: string) => void;
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
export function AIChatAttachmentStrip({
  attachments,
  onRemove,
  onRetry,
}: AIChatAttachmentStripProps) {
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
              onRetry={onRetry}
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
              onRetry={onRetry}
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
  onRetry: (id: string) => void;
}

function AIChatDocumentAttachmentChip({
  attachment,
  onRemove,
  onRetry,
}: AIChatDocumentAttachmentChipProps) {
  const t = useT('webapp');
  const isUploading = attachment.status === 'uploading';
  const percent = useUploadPercent(attachment.progress, isUploading);

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
      {isUploading ? (
        <Loader2 className="size-3.5 shrink-0 animate-spin" />
      ) : attachment.status === 'error' ? (
        <AlertCircle className="size-3.5 shrink-0" />
      ) : (
        <FileText className="size-3.5 shrink-0" />
      )}
      <span className="truncate text-foreground">{attachment.name}</span>
      <span className="shrink-0 tabular-nums">
        {isUploading ? `${percent}%` : formatFileSize(attachment.size)}
      </span>
      {attachment.status === 'error' && attachment.sourceFile ? (
        <Button
          isIcon
          variant="ghost"
          className="size-5 shrink-0 hover:bg-primary/10 hover:text-primary"
          onClick={() => onRetry(attachment.id)}
          title={t('consoleChat.attachments.retry')}
        >
          <RotateCcw className="size-3" />
        </Button>
      ) : null}
      <Button
        isIcon
        variant="ghost"
        className="size-5 shrink-0 hover:bg-destructive/10 hover:text-destructive"
        onClick={() => onRemove(attachment.id)}
        title={t('consoleChat.attachments.remove')}
      >
        <Trash2 className="size-3" />
      </Button>
      {isUploading ? <AttachmentUploadBar percent={percent} className="h-0.5" /> : null}
    </div>
  );
}

interface AIChatImageAttachmentPreviewProps {
  attachment: AIChatInputAttachment;
  onRemove: (id: string) => void;
  onRetry: (id: string) => void;
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
  onRetry,
}: AIChatImageAttachmentPreviewProps) {
  const t = useT('webapp');
  const [isPreviewOpen, setIsPreviewOpen] = useState(false);
  const { previewUrl, isLoading, error } = useFileOriginalPreviewUrl(attachment.file?.id, {
    enabled: attachment.status === 'uploaded' && Boolean(attachment.file?.id),
  });
  const isError = attachment.status === 'error' || Boolean(error);
  const isUploading = attachment.status === 'uploading';
  const percent = useUploadPercent(attachment.progress, isUploading);
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
          <div className="flex h-full w-full flex-col items-center justify-center gap-1 text-muted-foreground">
            {isUploading ? (
              <>
                <Loader2 className="size-5 animate-spin" />
                <span className="text-[11px] font-medium tabular-nums text-foreground/80">
                  {percent}%
                </span>
              </>
            ) : isLoading ? (
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
          title={t('consoleChat.attachments.remove')}
        >
          <Trash2 className="size-3" />
        </Button>
        {attachment.status === 'error' && attachment.sourceFile ? (
          <Button
            isIcon
            variant="secondary"
            className="absolute bottom-1 right-1 size-5 rounded-full bg-background/90 p-0 shadow-sm hover:bg-primary hover:text-primary-foreground"
            onClick={event => {
              event.stopPropagation();
              onRetry(attachment.id);
            }}
            title={t('consoleChat.attachments.retry')}
          >
            <RotateCcw className="size-3" />
          </Button>
        ) : null}
        {isUploading ? <AttachmentUploadBar percent={percent} className="h-1" /> : null}
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
