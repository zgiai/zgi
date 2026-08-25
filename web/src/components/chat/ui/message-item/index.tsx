import React, { useMemo, useDeferredValue, memo } from 'react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { MarkdownImage } from '@/components/common/markdown-image';
import type { Message, NodeRunStatus } from '@/components/chat/types';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import WorkflowRunMonitor from '@/components/chat/ui/workflow-run-monitor';
import { Bot, CirclePause, CircleStop, Copy, Loader } from 'lucide-react';
import { ModelIcon } from 'modelicons';
import { useT } from '@/i18n';
import { isSensitiveOutputBlockedValue } from '@/utils/model-output-filter';
import { cn } from '@/lib/utils';
import {
  normalizeQuestionAnswerTranscript,
  QuestionAnswerTranscript,
} from '@/components/workflow/question-answer/question-answer-transcript';

const IMAGE_GENERATION_CLIENT_MAX_MS = 10 * 60 * 1000;

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function objectValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : undefined;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function formatMessageTime(timestamp: number): string {
  if (!timestamp) return '';
  const date = new Date(timestamp * 1000);
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

interface MessageItemProps {
  message: Message;
  messageAddon?: React.ReactNode;
  /** Show expanded input/output details in workflow node list */
  showWorkflowNodeDetail?: boolean;
  /** Show workflow run monitor (hide entire workflow section if false) */
  showWorkflowDetail?: boolean;
  /** Allow expanding workflow run summary to inspect node details */
  allowWorkflowDetailExpand?: boolean;
  /** Default open state for workflow run summary */
  defaultWorkflowDetailOpen?: boolean;
  /** Show AI avatar */
  showAvatar?: boolean;
  /** Show copy button */
  showCopyButton?: boolean;
}

const MessageItemComponent: React.FC<MessageItemProps> = ({
  message,
  showWorkflowNodeDetail = true,
  showWorkflowDetail = true,
  allowWorkflowDetailExpand = true,
  defaultWorkflowDetailOpen = true,
  showAvatar = true,
  showCopyButton = true,
  messageAddon,
}) => {
  const t = useT('common');
  const tAgents = useT('agents');
  const tWebapp = useT('webapp');
  // Defer heavy markdown parsing under streaming to reduce render pressure
  const deferredAnswer = useDeferredValue(message.answer);

  const isSensitiveBlocked = useMemo(
    () =>
      message.messageData?.sensitiveOutputBlocked === true ||
      isSensitiveOutputBlockedValue(deferredAnswer),
    [deferredAnswer, message.messageData]
  );
  const safeAnswer = isSensitiveBlocked ? t('sensitiveOutput.blocked') : deferredAnswer;
  const questionAnswerTranscript = useMemo(() => {
    const metadata =
      message.messageData?.metadata && typeof message.messageData.metadata === 'object'
        ? (message.messageData.metadata as Record<string, unknown>)
        : {};
    return normalizeQuestionAnswerTranscript(
      message.messageData?.questionAnswerTranscript ?? metadata.questionAnswerTranscript
    );
  }, [message.messageData]);
  const hasQuestionAnswerTranscript = questionAnswerTranscript.length > 0;

  const isUser = useMemo(() => message.query && message.query.trim().length > 0, [message.query]);
  const hasAddon = Boolean(messageAddon);
  const generatedImages = useMemo(() => message.generatedImages || [], [message.generatedImages]);
  const hasImages = generatedImages.length > 0;
  const imageGenerationStatus = useMemo(() => getImageGenerationStatus(message), [message]);
  const imageGenerationErrorText = useMemo(
    () => getImageGenerationErrorText(message, imageGenerationStatus, tWebapp),
    [message, imageGenerationStatus, tWebapp]
  );
  const displayAnswer = imageGenerationErrorText || safeAnswer;
  const hasAi = useMemo(() => displayAnswer && displayAnswer.length > 0, [displayAnswer]);
  const isImageGenerating = isActiveImageGenerationStatus(imageGenerationStatus);
  const isImageCancelled = imageGenerationStatus === 'cancelled' || imageGenerationStatus === 'canceled';
  const referenceImage = useMemo(() => getMessageReferenceImage(message), [message]);
  const imageModelLabel =
    stringValue(message.messageData?.model_label) ||
    stringValue(message.model?.modelName) ||
    stringValue(message.messageData?.model_name);
  const imageModelName =
    stringValue(message.messageData?.model_name) ||
    stringValue(message.model?.rawModelName) ||
    imageModelLabel ||
    'unknown';
  const imageCreatedAt = numberValue(message.messageData?.created_at);
  const imageCreatedAtText = imageCreatedAt ? formatMessageTime(imageCreatedAt) : '';

  const isClientLoading = useMemo(() => {
    const phase = message.clientState?.phase ?? 'idle';
    return phase === 'requesting' || phase === 'streaming';
  }, [message.clientState?.phase]);
  const shouldShowGenericLoading = isClientLoading && !(isImageGenerating && hasImages);
  const isMessageEnd = useMemo(() => {
    const clientCompleted = message.clientState?.phase === 'completed';
    const wfStatus = message.WorkflowRunInfo?.status;
    const wfEnded =
      wfStatus === 'completed' ||
      wfStatus === 'error' ||
      wfStatus === 'stopped' ||
      wfStatus === 'pending_approval' ||
      wfStatus === 'pending_question' ||
      wfStatus === 'expired';
    return clientCompleted || wfEnded;
  }, [message.clientState?.phase, message.WorkflowRunInfo?.status]);
  const workflowPauseStatus =
    message.WorkflowRunInfo?.status === 'pending_approval' ||
    message.clientState?.status === 'pending_approval'
      ? 'pending_approval'
      : message.WorkflowRunInfo?.status === 'pending_question' ||
          message.clientState?.status === 'pending_question'
        ? 'pending_question'
        : null;
  const isEmptyStoppedWorkflow =
    !hasAi &&
    !hasImages &&
    !hasAddon &&
    !hasQuestionAnswerTranscript &&
    (message.WorkflowRunInfo?.status === 'stopped' || message.clientState?.status === 'stopped');
  const nodeItems = useMemo(() => {
    const nodes = message.WorkflowRunInfo?.runNodeInfo ?? [];
    const mapStatus = (
      s: NodeRunStatus
    ): 'running' | 'succeeded' | 'failed' | 'stopped' | 'paused' => {
      if (s === 'failed') return 'failed';
      if (s === 'stopped') return 'stopped';
      if (s === 'paused') return 'paused';
      if (s === 'success' || s === 'succeeded') return 'succeeded';
      return 'running';
    };
    return nodes.map((n, idx) => {
      const base = {
        title: n.title || `Step ${idx + 1}`,
        nodeId: n.nodeId || `step-${idx}`,
        nodeType: n.nodeType || 'unknown',
        status: mapStatus(n.status),
        nodeInput: n.data?.input ?? undefined,
        nodeOutput: n.data?.output ?? undefined,
        modelInput: n.data?.modelInput ?? undefined,
        elapsedTime: n.elapsedTime,
        error: n.error ?? null,
      };
      if (n.nodeType === 'iteration' || n.nodeType === 'loop') {
        const roundsSource =
          n.nodeType === 'loop' ? (n.loopRounds ?? []) : (n.iterationRounds ?? []);
        const rounds = roundsSource.map(r => ({
          index: r.index,
          elapsedTime: r.elapsedTime,
          nodes: (r.nodes ?? []).map((child, cidx) => ({
            title: child.title || `Step ${cidx + 1}`,
            nodeId: child.nodeId || `step-${cidx}`,
            nodeType: child.nodeType || 'unknown',
            status: mapStatus(child.status),
            nodeInput: child.data?.input ?? undefined,
            nodeOutput: child.data?.output ?? undefined,
            modelInput: child.data?.modelInput ?? undefined,
            elapsedTime: child.elapsedTime,
            error: child.error ?? null,
          })),
        }));
        return {
          ...base,
          iterationInputs: n.nodeType === 'iteration' ? n.iterationInputs : undefined,
          iterationOutputs: n.nodeType === 'iteration' ? n.iterationOutputs : undefined,
          iterationRounds: n.nodeType === 'iteration' ? rounds : undefined,
          loopInputs: n.nodeType === 'loop' ? n.loopInputs : undefined,
          loopOutputs: n.nodeType === 'loop' ? n.loopOutputs : undefined,
          loopRounds: n.nodeType === 'loop' ? rounds : undefined,
          steps: typeof n.steps === 'number' ? n.steps : undefined,
        };
      }
      return base;
    });
  }, [message.WorkflowRunInfo]);

  return (
    <div className="space-y-3">
      {isUser && (
        <div className="flex justify-end">
          <div className="max-w-[80%] rounded-2xl px-3 py-2 bg-muted/80 text-sm whitespace-pre-wrap">
            {referenceImage ? (
              <div className="mb-2 overflow-hidden rounded-lg bg-background/60">
                <img
                  src={referenceImage.url}
                  alt={referenceImage.filename || 'Reference image'}
                  className="max-h-52 max-w-full object-contain"
                />
              </div>
            ) : null}
            {message.query ? <div className="whitespace-pre-wrap">{message.query}</div> : null}
          </div>
        </div>
      )}

      <div className="flex justify-start">
        <div className="w-full prose dark:prose-invert prose-sm">
          <div className="flex justify-between items-center">
            {showAvatar && (
              <div className="h-7 w-7 rounded-full bg-primary flex items-center justify-center">
                <Bot size={20} className="text-primary-foreground" />
              </div>
            )}
            <div>
              {shouldShowGenericLoading && !message.WorkflowRunInfo && (
                <Loader size={16} className="animate-spin" />
              )}
            </div>
          </div>
          {showWorkflowDetail && (nodeItems.length > 0 || message.WorkflowRunInfo) && (
            <div className="mt-3">
              <WorkflowRunMonitor
                status={message.WorkflowRunInfo?.status}
                elapsedTime={message.WorkflowRunInfo?.elapsedTime}
                items={nodeItems}
                error={message.WorkflowRunInfo?.error}
                showDetail={showWorkflowNodeDetail}
                allowExpand={allowWorkflowDetailExpand}
                defaultOpen={defaultWorkflowDetailOpen}
              />
            </div>
          )}
          {messageAddon ? <div className="mt-3">{messageAddon}</div> : null}
          <div className="mt-2 overflow-x-auto">
            {hasQuestionAnswerTranscript ? (
              <QuestionAnswerTranscript items={questionAnswerTranscript} className="mb-3" />
            ) : null}
            {hasAi ? (
              <MarkdownViewer
                className="md-viewer break-words"
                content={displayAnswer}
                isStreaming={isClientLoading}
                renderIdentity={message.messageId}
              />
            ) : isImageGenerating && !hasImages ? (
              <ImageGenerationStatusRow
                icon="running"
                label={tWebapp('chat.imageInput.taskRunning')}
              />
            ) : isImageCancelled ? (
              <ImageGenerationStatusRow
                icon="cancelled"
                label={tWebapp('chat.imageInput.taskCancelled')}
              />
            ) : workflowPauseStatus || isEmptyStoppedWorkflow ? (
              <div
                className={cn(
                  'flex items-start gap-3 rounded-xl border px-4 py-3 not-prose',
                  workflowPauseStatus
                    ? 'border-warning/30 bg-warning/5'
                    : 'border-border/60 bg-muted/35'
                )}
                role="status"
                aria-live="polite"
              >
                <div
                  className={cn(
                    'flex size-9 shrink-0 items-center justify-center rounded-full',
                    workflowPauseStatus
                      ? 'bg-warning/15 text-warning'
                      : 'bg-muted text-muted-foreground'
                  )}
                >
                  {workflowPauseStatus ? (
                    <CirclePause className="size-4" aria-hidden="true" />
                  ) : (
                    <CircleStop className="size-4" aria-hidden="true" />
                  )}
                </div>
                <div className="min-w-0 space-y-0.5">
                  <div className="text-sm font-medium text-foreground">
                    {workflowPauseStatus === 'pending_approval'
                      ? tAgents('workflowRunMonitor.pendingApprovalTitle')
                      : workflowPauseStatus === 'pending_question'
                        ? tAgents('workflowRunMonitor.pendingQuestionTitle')
                        : tAgents('workflowRunMonitor.stoppedTitle')}
                  </div>
                  <div className="text-xs leading-5 text-muted-foreground">
                    {workflowPauseStatus === 'pending_approval'
                      ? tAgents('workflowRunMonitor.pendingApprovalDescription')
                      : workflowPauseStatus === 'pending_question'
                        ? tAgents('workflowRunMonitor.pendingQuestionDescription')
                        : tAgents('workflowRunMonitor.stoppedDescription')}
                  </div>
                </div>
              </div>
            ) : shouldShowGenericLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-4 w-2/3" />
                <Skeleton className="h-4 w-1/2" />
                <Skeleton className="h-4 w-3/4" />
              </div>
            ) : isMessageEnd && !hasImages && !hasAddon && !hasQuestionAnswerTranscript ? (
              <span className="text-muted-foreground break-words">--</span>
            ) : null}

            {hasAi && isImageGenerating && !hasImages ? (
              <ImageGenerationStatusRow
                icon="running"
                label={tWebapp('chat.imageInput.taskRunning')}
                className="mt-3"
              />
            ) : null}
            {hasImages && (
              <div className="mt-4">
                <div className="mb-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <span className="inline-flex size-6 items-center justify-center rounded-full border bg-background">
                    <ModelIcon model={imageModelName} size={24} />
                  </span>
                  {imageModelLabel ? <span>{imageModelLabel}</span> : null}
                  {imageCreatedAtText ? <span>{imageCreatedAtText}</span> : null}
                </div>
                <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4">
                  {generatedImages.map((img, idx) => (
                    <div
                      key={idx}
                      className="relative aspect-square overflow-hidden rounded-lg border bg-muted group"
                    >
                      {img.isLoading ? (
                        <div className="flex h-full w-full items-center justify-center bg-muted/70">
                          <div className="flex flex-col items-center gap-2 text-xs text-muted-foreground">
                            <Loader className="size-5 animate-spin" aria-hidden="true" />
                            <span>{tWebapp('chat.imageInput.generatingShort')}</span>
                          </div>
                        </div>
                      ) : (
                        <MarkdownImage
                          src={img.url}
                          alt={img.alt || `Generated image ${idx + 1}`}
                          className="w-full h-full flex [&>div]:w-full [&>div]:h-full"
                          imageClassName="w-full h-full object-cover transition-all group-hover:scale-105 max-h-none"
                        />
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {hasAi && showCopyButton && (
              <div className="mt-2">
                <Button
                  variant="ghost"
                  isIcon
                  className="h-6 w-6"
                  onClick={() => navigator.clipboard?.writeText(displayAnswer)}
                >
                  <Copy size={12} />
                </Button>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

// Memoize to avoid rerenders when unrelated fields change frequently during streaming
const MessageItem = memo(MessageItemComponent, (prev, next) => {
  const prevRun = prev.message.WorkflowRunInfo;
  const nextRun = next.message.WorkflowRunInfo;
  const sameQuery = prev.message.query === next.message.query;
  const sameAnswer = prev.message.answer === next.message.answer;
  const sameStatus = (prevRun?.status ?? null) === (nextRun?.status ?? null);
  const prevNodes = prevRun?.runNodeInfo ?? [];
  const nextNodes = nextRun?.runNodeInfo ?? [];
  const sameNodeLen = prevNodes.length === nextNodes.length;
  const sameClientPhase =
    (prev.message.clientState?.phase ?? null) === (next.message.clientState?.phase ?? null);
  const sameSensitiveBlocked =
    (prev.message.messageData?.sensitiveOutputBlocked ?? false) ===
    (next.message.messageData?.sensitiveOutputBlocked ?? false);
  const sameQuestionAnswerTranscript =
    prev.message.messageData?.questionAnswerTranscript ===
      next.message.messageData?.questionAnswerTranscript &&
    prev.message.messageData?.metadata === next.message.messageData?.metadata;
  const sameImages = prev.message.generatedImages === next.message.generatedImages;
  const sameReferenceInput =
    prev.message.inputs?.image_reference === next.message.inputs?.image_reference;
  const sameImageHeader =
    prev.message.model === next.message.model &&
    prev.message.messageData?.model_label === next.message.messageData?.model_label &&
    prev.message.messageData?.model_name === next.message.messageData?.model_name &&
    prev.message.messageData?.created_at === next.message.messageData?.created_at &&
    prev.message.messageData?.image_task_status === next.message.messageData?.image_task_status &&
    prev.message.messageData?.image_generation === next.message.messageData?.image_generation;
  // If node counts are equal, shallow-compare the tail where updates are most frequent
  let sameNodesTail = true;
  if (sameNodeLen && nextNodes.length > 0) {
    const a = prevNodes[nextNodes.length - 1];
    const b = nextNodes[nextNodes.length - 1];
    sameNodesTail =
      a?.nodeId === b?.nodeId && a?.status === b?.status && a?.elapsedTime === b?.elapsedTime;
  }
  return (
    sameQuery &&
    sameAnswer &&
    sameStatus &&
    sameClientPhase &&
    sameSensitiveBlocked &&
    sameQuestionAnswerTranscript &&
    sameImageHeader &&
    sameReferenceInput &&
    sameNodeLen &&
    sameNodesTail &&
    sameImages &&
    prev.showWorkflowDetail === next.showWorkflowDetail &&
    prev.showWorkflowNodeDetail === next.showWorkflowNodeDetail &&
    prev.allowWorkflowDetailExpand === next.allowWorkflowDetailExpand &&
    prev.defaultWorkflowDetailOpen === next.defaultWorkflowDetailOpen &&
    prev.showAvatar === next.showAvatar &&
    prev.showCopyButton === next.showCopyButton &&
    prev.messageAddon === next.messageAddon
  );
});

function ImageGenerationStatusRow({
  icon,
  label,
  className,
}: {
  icon: 'running' | 'cancelled';
  label: string;
  className?: string;
}) {
  return (
    <div
      className={cn(
        'flex w-fit items-center gap-2 rounded-lg border bg-muted/35 px-3 py-2 text-sm text-muted-foreground not-prose',
        className
      )}
      role="status"
      aria-live="polite"
    >
      {icon === 'running' ? (
        <Loader className="size-4 animate-spin" aria-hidden="true" />
      ) : (
        <CircleStop className="size-4" aria-hidden="true" />
      )}
      <span>{label}</span>
    </div>
  );
}

function getImageGenerationStatus(message: Message): string {
  const metadata = objectValue(message.messageData?.metadata);
  const imageGeneration =
    objectValue(message.messageData?.image_generation) || objectValue(metadata?.image_generation);
  const status = (
    stringValue(message.messageData?.image_task_status) ||
    stringValue(metadata?.image_task_status) ||
    stringValue(imageGeneration?.status) ||
    stringValue(message.clientState?.status) ||
    stringValue(message.clientState?.phase)
  ).toLowerCase();
  const messageStatus = stringValue(message.messageData?.status).toLowerCase();
  const messageError =
    stringValue(message.messageData?.error) || stringValue(metadata?.image_task_error) || message.answer;
  if (isActiveImageGenerationStatus(status) && isImageMessageOlderThanTimeout(message)) {
    return 'failed';
  }
  if (
    messageStatus === 'error' &&
    (isActiveImageGenerationStatus(status) ||
      status === 'runtime_lease_expired' ||
      messageError.toLowerCase() === 'runtime_lease_expired')
  ) {
    return 'failed';
  }
  if (status === 'runtime_lease_expired' || status === 'image_task_timeout' || status === 'timeout') {
    return 'failed';
  }
  return status;
}

function isActiveImageGenerationStatus(status: string): boolean {
  if (
    status === 'succeeded' ||
    status === 'failed' ||
    status === 'cancelled' ||
    status === 'canceled' ||
    status === 'runtime_lease_expired' ||
    status === 'image_task_timeout' ||
    status === 'timeout'
  ) {
    return false;
  }
  return (
    status === 'pending' ||
    status === 'running' ||
    status === 'processing' ||
    status === 'in_progress' ||
    status === 'streaming' ||
    status === 'requesting'
  );
}

function isImageMessageOlderThanTimeout(message: Message): boolean {
  const createdAt = timestampMs(message.messageData?.created_at);
  return createdAt > 0 && Date.now() - createdAt >= IMAGE_GENERATION_CLIENT_MAX_MS;
}

function timestampMs(value: unknown): number {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value < 1_000_000_000_000 ? value * 1000 : value;
  }
  if (typeof value !== 'string') {
    return 0;
  }
  const trimmed = value.trim();
  if (!trimmed) {
    return 0;
  }
  const numeric = Number(trimmed);
  if (Number.isFinite(numeric)) {
    return numeric < 1_000_000_000_000 ? numeric * 1000 : numeric;
  }
  const parsed = Date.parse(trimmed);
  return Number.isFinite(parsed) ? parsed : 0;
}

function getImageGenerationErrorText(
  message: Message,
  status: string,
  t: ReturnType<typeof useT<'webapp'>>
): string {
  if (status !== 'failed') {
    return '';
  }
  const metadata = objectValue(message.messageData?.metadata);
  const rawError =
    stringValue(message.messageData?.error) ||
    stringValue(metadata?.image_task_error) ||
    stringValue(message.answer);
  if (isImageTimeoutError(rawError) || (!rawError && isImageMessageOlderThanTimeout(message))) {
    return t('chat.imageInput.taskTimeout');
  }
  if (rawError.toLowerCase() === 'prompt_too_long') {
    return t('chat.imageInput.promptTooLong', { max: 4000 });
  }
  if (rawError.toLowerCase() === 'upstream_failed') {
    return t('chat.imageInput.taskProviderFailed');
  }
  return isInternalImageGenerationError(rawError) ? t('chat.imageInput.taskFailed') : '';
}

function isImageTimeoutError(value: unknown): boolean {
  const normalized = stringValue(value).toLowerCase();
  return (
    normalized === 'runtime_lease_expired' ||
    normalized === 'image_task_timeout' ||
    normalized === 'timeout'
  );
}

function isInternalImageGenerationError(value: unknown): boolean {
  const normalized = stringValue(value).toLowerCase();
  return isImageTimeoutError(normalized) || normalized === 'upstream_failed';
}

function getMessageReferenceImage(message: Message): { url: string; filename: string } | null {
  const inputReference = objectValue(message.inputs?.image_reference);
  const messageDataReference = objectValue(message.messageData?.image_reference);
  const metadata = objectValue(message.messageData?.metadata);
  const imageGeneration =
    objectValue(message.messageData?.image_generation) || objectValue(metadata?.image_generation);
  const persistedReference = objectValue(imageGeneration?.reference_image);
  const reference = inputReference || messageDataReference || persistedReference;
  const url = stringValue(reference?.url);
  if (!url) return null;
  return {
    url,
    filename: stringValue(reference?.filename),
  };
}

export default MessageItem;
