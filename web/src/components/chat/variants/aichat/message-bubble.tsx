'use client';

import { ModelIcon } from 'modelicons';
import { useMemo, useState } from 'react';
import {
  AlertCircle,
  CheckCircle2,
  CircleStop,
  Download,
  Eye,
  FileImage,
  FileText,
  Loader2,
} from 'lucide-react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Textarea } from '@/components/ui/textarea';
import { useFileOriginalPreviewUrl } from '@/hooks/file/use-file-original-preview-url';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { useWorkspaceStore } from '@/store/workspace-store';
import type {
  AIChatGeneratedFile,
  AIChatMessage,
  AIChatMessageFile,
} from '@/services/types/aichat';
import { isSensitiveOutputBlockedValue } from '@/utils/model-output-filter';
import type { ChatBranchNavigation } from '@/components/chat/utils/message-tree';
import {
  AssistantMessageToolbar,
  UserEditToolbar,
  UserMessageToolbar,
} from '@/components/chat/variants/aichat/message-toolbars';
import { UniversalFilePreviewDialog } from '@/components/files/universal-file-preview-dialog';
import { AIChatAgenticTimeline } from '@/components/chat/variants/aichat/agentic-timeline';
import {
  getAIChatMessageErrorInput,
  resolveAIChatErrorMessage,
} from '@/components/chat/variants/aichat/error-utils';
import type { AIChatSkillDisplayMap } from '@/components/chat/variants/aichat/skill-display';
import type { AIChatAgenticTimelineItem } from '@/components/chat/controllers/aichat';
import { timelineFromAIChatMessage } from '@/components/chat/controllers/aichat/selectors';
import { MAX_AICHAT_BRANCHES } from '@/components/chat/variants/aichat/types';

interface AIChatMessageBubbleProps {
  message: AIChatMessage;
  isSending?: boolean;
  timeline?: AIChatAgenticTimelineItem[];
  skillDisplayById: AIChatSkillDisplayMap;
  isLastMessage?: boolean;
  canReplaceRoot?: boolean;
  onRegenerate?: (message: AIChatMessage) => void;
  branchNavigation?: ChatBranchNavigation;
  onSwitchBranch?: (messageId: string) => void;
  isEditing?: boolean;
  editValue?: string;
  onEditStart?: (message: AIChatMessage) => void;
  onEditChange?: (value: string) => void;
  onEditCancel?: () => void;
  onEditSubmit?: (message: AIChatMessage) => void;
}

function formatAIChatTime(timestamp: number): string {
  if (!timestamp) return '';

  const date = new Date(timestamp * 1000);
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function formatFileSize(size: number): string {
  if (!Number.isFinite(size) || size <= 0) {
    return '0 B';
  }

  const units = ['B', 'KB', 'MB', 'GB'];
  let value = size;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  return `${value >= 10 || unitIndex === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unitIndex]}`;
}

function formatGeneratedFileExtension(file: AIChatGeneratedFile): string {
  const extension = file.extension || file.filename.split('.').pop() || '';
  return extension.replace(/^\./, '').toUpperCase();
}

interface AIChatHistoryImagePreviewProps {
  file: AIChatMessageFile;
}

/**
 * @component AIChatHistoryImagePreview
 * @category Feature
 * @status Stable
 * @description Renders historical AIChat image attachments using the signed preview URL endpoint.
 * @usage Used in AIChatMessageBubble for message metadata files
 * @example
 * <AIChatHistoryImagePreview file={file} />
 */
function AIChatHistoryImagePreview({ file }: AIChatHistoryImagePreviewProps) {
  const t = useT('webapp');
  const [isPreviewOpen, setIsPreviewOpen] = useState(false);
  const { previewUrl, isLoading, error } = useFileOriginalPreviewUrl(file.id, {
    enabled: Boolean(file.id),
  });
  const isFiltered = file.content_status === 'filtered';
  const isError = file.parse_status === 'error' || Boolean(error);
  const title =
    file.error ||
    error ||
    (file.filtered_reason === 'model_without_vision'
      ? t('consoleChat.attachments.filteredModelWithoutVision')
      : file.name);

  return (
    <>
      <button
        type="button"
        className={cn(
          'relative size-24 overflow-hidden rounded-lg border bg-background/70 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          isError || isFiltered ? 'border-destructive/40' : 'border-border',
          previewUrl || isError || isFiltered ? 'cursor-pointer' : 'cursor-default'
        )}
        title={title}
        onClick={() => {
          if (previewUrl || isError || isFiltered) {
            setIsPreviewOpen(true);
          }
        }}
      >
        {previewUrl ? (
          <img src={previewUrl} alt={file.name} className="h-full w-full object-cover" />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
            {isLoading ? (
              <Loader2 className="size-5 animate-spin" />
            ) : isError || isFiltered ? (
              <AlertCircle className="size-5 text-destructive" />
            ) : (
              <FileImage className="size-5" />
            )}
          </div>
        )}
      </button>
      <Dialog open={isPreviewOpen} onOpenChange={setIsPreviewOpen}>
        <DialogContent className="max-h-[90vh] max-w-[90vw] overflow-hidden p-0">
          <DialogHeader className="border-b px-4 py-3">
            <DialogTitle className="truncate text-sm">{file.name}</DialogTitle>
          </DialogHeader>
          <div className="flex max-h-[calc(90vh-56px)] min-h-64 items-center justify-center overflow-auto bg-muted/30 p-4">
            {previewUrl ? (
              <img
                src={previewUrl}
                alt={file.name}
                className="max-h-[calc(90vh-96px)] max-w-full object-contain"
              />
            ) : (
              <div className="flex max-w-sm flex-col items-center gap-2 text-center text-sm text-muted-foreground">
                <AlertCircle className="size-6 text-destructive" />
                <span>{title || t('consoleChat.attachments.previewLoadError')}</span>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

interface AIChatGeneratedFileCardProps {
  file: AIChatGeneratedFile;
}

/**
 * @component AIChatGeneratedFileCard
 * @category Feature
 * @status Stable
 * @description Renders a downloadable file artifact generated by an AIChat skill.
 * @usage Used in AIChatMessageBubble for skill_artifact_created outputs.
 * @example
 * <AIChatGeneratedFileCard file={file} />
 */
function AIChatGeneratedFileCard({ file }: AIChatGeneratedFileCardProps) {
  const t = useT('webapp');
  const [isPreviewOpen, setIsPreviewOpen] = useState(false);
  const extension = formatGeneratedFileExtension(file);
  const downloadUrl = file.download_url || file.url;
  const previewUrl = file.url || downloadUrl;

  return (
    <>
      <div
        className="flex min-w-0 max-w-sm items-center gap-3 rounded-md border bg-background px-3 py-2 text-sm shadow-sm"
        title={file.filename}
      >
        <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <FileText className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="truncate font-medium text-foreground">{file.filename}</div>
          <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            <span>{t('consoleChat.attachments.generatedFile')}</span>
            {extension ? <span>{extension}</span> : null}
            <span>{formatFileSize(file.size)}</span>
          </div>
        </div>
        <Button
          type="button"
          isIcon
          variant="ghost"
          className="size-8 shrink-0 rounded-full text-muted-foreground hover:text-foreground"
          aria-label={t('consoleChat.attachments.previewGeneratedFile')}
          title={t('consoleChat.attachments.previewGeneratedFile')}
          onClick={() => setIsPreviewOpen(true)}
        >
          <Eye className="size-4" />
        </Button>
        <Button
          asChild
          type="button"
          isIcon
          variant="ghost"
          className="size-8 shrink-0 rounded-full text-muted-foreground hover:text-foreground"
          aria-label={t('consoleChat.attachments.downloadGeneratedFile')}
          title={t('consoleChat.attachments.downloadGeneratedFile')}
        >
          <a href={downloadUrl} download={file.filename}>
            <Download className="size-4" />
          </a>
        </Button>
      </div>
      <UniversalFilePreviewDialog
        open={isPreviewOpen}
        onOpenChange={setIsPreviewOpen}
        file={{
          id: file.file_id,
          name: file.filename,
          extension: file.extension,
          mimeType: file.mime_type,
          size: file.size,
          previewUrl,
          downloadUrl,
        }}
      />
    </>
  );
}

/**
 * @component AIChatMessageBubble
 * @category Feature
 * @status Stable
 * @description Renders one AIChat persisted turn as user query plus assistant answer.
 * @usage Used by AIChatShell for standalone console chat messages
 * @example
 * <AIChatMessageBubble message={message} />
 */
export function AIChatMessageBubble({
  message,
  isSending = false,
  timeline = [],
  skillDisplayById,
  isLastMessage = false,
  canReplaceRoot = false,
  onRegenerate,
  branchNavigation,
  onSwitchBranch,
  isEditing = false,
  editValue = '',
  onEditStart,
  onEditChange,
  onEditCancel,
  onEditSubmit,
}: AIChatMessageBubbleProps) {
  const t = useT('webapp');
  const tGlobal = useT();
  const tCommon = useT('common');
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const organizationRole = useWorkspaceStore.use.permissionState().organizationRole;
  const isBillingAdmin = organizationRole === 'owner' || organizationRole === 'admin';
  const isStreaming = message.status === 'pending' || message.status === 'streaming';
  const isError = message.status === 'error';
  const isStopped = message.status === 'stopped';
  const isSensitiveBlocked =
    message.metadata?.sensitiveOutputBlocked === true ||
    isSensitiveOutputBlockedValue(message.answer);
  const displayAnswer = isSensitiveBlocked ? tCommon('sensitiveOutput.blocked') : message.answer;
  const answer = displayAnswer.trim();
  const hasParent = Boolean(message.parent_id);
  const branchCount = branchNavigation?.total ?? 1;
  const canCreateBranch = hasParent && branchCount < MAX_AICHAT_BRANCHES;
  const canEdit =
    Boolean(onEditStart && (canReplaceRoot || canCreateBranch)) && !isSending && !isStreaming;
  const canRegenerateMessage = Boolean(onRegenerate && (canReplaceRoot || canCreateBranch));
  const canSwitchBranch = Boolean(branchNavigation && onSwitchBranch) && !isSending && !isStreaming;
  const shouldHideAssistantToolbar = isLastMessage && isStreaming;
  const toolbarVisibility = isLastMessage
    ? 'opacity-100'
    : 'pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100';
  const files = message.metadata?.files ?? [];
  const generatedFiles = message.metadata?.generated_files ?? [];
  const imageFiles = files.filter(file => file.kind === 'image');
  const documentFiles = files.filter(file => file.kind !== 'image');
  const historicalTimeline = useMemo<AIChatAgenticTimelineItem[]>(
    () => timelineFromAIChatMessage(message),
    [message]
  );
  const displayTimeline = timeline.length > 0 ? timeline : historicalTimeline;
  const hasTimeline = displayTimeline.length > 0;
  const shouldOpenTimelineByDefault =
    isStreaming ||
    displayTimeline.some(
      item =>
        item.type === 'skill_event' &&
        (item.invocation.status === 'error' || item.invocation.status === 'blocked')
    );
  const errorDisplay = useMemo(
    () =>
      isError
        ? resolveAIChatErrorMessage(
            (key, values) => tGlobal(key as never, values),
            getAIChatMessageErrorInput(message),
            {
              isAdmin: isBillingAdmin,
              workspaceId: currentWorkspace?.id,
            }
          )
        : null,
    [currentWorkspace?.id, isBillingAdmin, isError, message, tGlobal]
  );

  return (
    <div className="group space-y-3">
      <div className="flex justify-end">
        <div className={cn('max-w-[82%]', isEditing ? 'w-full max-w-2xl' : '')}>
          {isEditing ? (
            <div className="rounded-2xl border bg-background p-2 shadow-sm">
              <Textarea
                value={editValue}
                onChange={event => onEditChange?.(event.target.value)}
                onKeyDown={event => {
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault();
                    onEditSubmit?.(message);
                  }
                }}
                className="max-h-40 min-h-20 resize-none border-0 bg-transparent px-2 py-1 text-sm shadow-none focus-visible:ring-0"
                autoFocus
              />
              <UserEditToolbar
                canSubmit={Boolean(editValue.trim())}
                isSending={isSending}
                onCancel={onEditCancel}
                onSubmit={() => onEditSubmit?.(message)}
              />
            </div>
          ) : (
            <>
              <div className="rounded-2xl bg-muted px-3 py-2 text-sm text-foreground shadow-sm">
                <div className="whitespace-pre-wrap break-words">{message.query}</div>
                {files.length > 0 ? (
                  <div className="mt-2 space-y-2">
                    {imageFiles.length > 0 ? (
                      <div className="flex flex-wrap gap-2">
                        {imageFiles.map(file => (
                          <AIChatHistoryImagePreview key={file.id} file={file} />
                        ))}
                      </div>
                    ) : null}
                    {documentFiles.length > 0 ? (
                      <div className="flex flex-wrap gap-1.5">
                        {documentFiles.map(file => {
                          const isFileParsing =
                            file.parse_status === 'parsing' ||
                            (file.content_status === 'pending' &&
                              file.parse_status !== 'error');
                          const isFileError = file.parse_status === 'error';
                          const isFileEmpty = file.content_status === 'empty' && !isFileError;
                          const isFileExtracted =
                            file.content_status === 'extracted' && !isFileError;
                          const isVisionReady =
                            file.content_status === 'vision_ready' && !isFileError;
                          const isFiltered = file.content_status === 'filtered' && !isFileError;
                          const label = isFileError
                            ? t('consoleChat.attachments.parseFailed')
                            : isFileEmpty
                              ? t('consoleChat.attachments.empty')
                              : isFiltered
                                ? t('consoleChat.attachments.filtered')
                                : isVisionReady
                                  ? t('consoleChat.attachments.visionReady')
                                  : isFileExtracted
                                    ? t('consoleChat.attachments.parsed')
                                    : t('consoleChat.attachments.parsing');

                          return (
                            <div
                              key={file.id}
                              className={cn(
                                'inline-flex max-w-full items-center gap-1.5 rounded-md border bg-background/70 px-2 py-1 text-xs',
                                isFileError || isFiltered
                                  ? 'border-destructive/40 text-destructive'
                                  : 'border-border text-muted-foreground'
                              )}
                              title={
                                file.error ||
                                (file.filtered_reason === 'model_without_vision'
                                  ? t('consoleChat.attachments.filteredModelWithoutVision')
                                  : file.name)
                              }
                            >
                              {isFileParsing ? (
                                <Loader2 className="size-3.5 shrink-0 animate-spin" />
                              ) : isFileError ? (
                                <AlertCircle className="size-3.5 shrink-0" />
                              ) : isFileExtracted || isVisionReady ? (
                                <CheckCircle2 className="size-3.5 shrink-0 text-emerald-600" />
                              ) : isFiltered ? (
                                <AlertCircle className="size-3.5 shrink-0" />
                              ) : file.kind === 'image' ? (
                                <FileImage className="size-3.5 shrink-0" />
                              ) : (
                                <FileText className="size-3.5 shrink-0" />
                              )}
                              <span className="max-w-40 truncate text-foreground">
                                {file.name}
                              </span>
                              <span className="shrink-0">{formatFileSize(file.size)}</span>
                              <span className="shrink-0">{label}</span>
                            </div>
                          );
                        })}
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </div>
              <UserMessageToolbar
                query={message.query}
                canEdit={canEdit}
                isDisabled={isSending || isStreaming}
                toolbarVisibility={toolbarVisibility}
                onEdit={() => onEditStart?.(message)}
              />
            </>
          )}
        </div>
      </div>

      <div className="flex justify-start gap-3">
        <div className="mt-1 flex size-7 shrink-0 items-center justify-center rounded-full border bg-background">
          <ModelIcon model={message.model_name || 'unknown'} size={28} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="mb-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            {message.model_name ? <span>{message.model_name}</span> : null}
            {message.created_at ? <span>{formatAIChatTime(message.created_at)}</span> : null}
            {isStreaming ? (
              <span className="inline-flex items-center gap-1">
                <Loader2 className="size-3 animate-spin" />
                {t('consoleChat.streaming')}
              </span>
            ) : null}
            {isStopped && answer ? (
              <span
                className="inline-flex items-center"
                title={t('consoleChat.stopped')}
                aria-label={t('consoleChat.stopped')}
              >
                <CircleStop className="size-3" />
              </span>
            ) : null}
          </div>

          {hasTimeline ? (
            <AIChatAgenticTimeline
              key={`${message.id}-${isStreaming ? 'streaming' : 'history'}-${shouldOpenTimelineByDefault ? 'open' : 'closed'}`}
              timeline={displayTimeline}
              skillDisplayById={skillDisplayById}
              defaultOpen={shouldOpenTimelineByDefault}
            />
          ) : null}

          {generatedFiles.length > 0 ? (
            <div className="mb-3 flex flex-wrap gap-2">
              {generatedFiles.map(file => (
                <AIChatGeneratedFileCard key={file.file_id} file={file} />
              ))}
            </div>
          ) : null}

          {answer ? (
            <div className="prose prose-sm min-w-0 max-w-full overflow-x-hidden dark:prose-invert sm:pr-4 md:pr-6 lg:pr-8 xl:pr-9">
              <MarkdownViewer
                className="md-viewer min-w-0 max-w-full overflow-x-hidden break-words"
                content={displayAnswer}
              />
              {shouldHideAssistantToolbar ? null : (
                <AssistantMessageToolbar
                  answer={answer}
                  canRegenerate={canRegenerateMessage}
                  isDisabled={isSending || isStreaming}
                  toolbarVisibility={toolbarVisibility}
                  branchNavigation={branchNavigation}
                  canSwitchBranch={canSwitchBranch}
                  onRegenerate={() => onRegenerate?.(message)}
                  onSwitchBranch={onSwitchBranch}
                />
              )}
            </div>
          ) : isStreaming && !hasTimeline ? (
            <div className="space-y-2 pt-1">
              <Skeleton className="h-4 w-2/3" />
              <Skeleton className="h-4 w-1/2" />
              <Skeleton className="h-4 w-3/4" />
            </div>
          ) : isStopped ? (
            <div className="text-sm text-muted-foreground">{t('consoleChat.stopped')}</div>
          ) : null}

          {isError ? (
            <div
              className={cn(
                'mt-2 flex items-start gap-2 rounded-md border p-3 text-sm',
                errorDisplay?.isBilling
                  ? 'border-amber-200 bg-amber-50 text-amber-950'
                  : 'border-destructive/30 bg-destructive/10 text-destructive'
              )}
            >
              <AlertCircle className="mt-0.5 size-4 shrink-0" />
              <div className="min-w-0 flex-1 space-y-2">
                <div>{errorDisplay?.description || t('consoleChat.streamError')}</div>
                {isBillingAdmin && errorDisplay?.href && errorDisplay.actionLabel ? (
                  <a
                    href={errorDisplay.href}
                    className="inline-flex h-7 items-center rounded-[4px] border border-amber-300 bg-white px-2.5 text-xs font-semibold text-amber-950 transition-colors hover:border-amber-400 hover:bg-amber-100"
                  >
                    {errorDisplay.actionLabel}
                  </a>
                ) : null}
              </div>
            </div>
          ) : null}

          {!answer && (isError || isStopped) && canRegenerateMessage && !shouldHideAssistantToolbar ? (
            <AssistantMessageToolbar
              answer=""
              canRegenerate={canRegenerateMessage}
              isDisabled={isSending || isStreaming}
              toolbarVisibility={toolbarVisibility}
              branchNavigation={branchNavigation}
              canSwitchBranch={canSwitchBranch}
              onRegenerate={() => onRegenerate?.(message)}
              onSwitchBranch={onSwitchBranch}
            />
          ) : null}
        </div>
      </div>
    </div>
  );
}
