import React, { useMemo, useDeferredValue, memo } from 'react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { MarkdownImage } from '@/components/common/markdown-image';
import type { Message, NodeRunStatus } from '@/components/chat/types';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import WorkflowRunMonitor from '@/components/chat/ui/workflow-run-monitor';
import { Bot, Copy, Loader } from 'lucide-react';
import { useT } from '@/i18n';
import { isSensitiveOutputBlockedValue } from '@/utils/model-output-filter';
import {
  normalizeQuestionAnswerTranscript,
  QuestionAnswerTranscript,
} from '@/components/workflow/question-answer/question-answer-transcript';

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
  // Defer heavy markdown parsing under streaming to reduce render pressure
  const deferredAnswer = useDeferredValue(message.answer);

  const isSensitiveBlocked = useMemo(
    () =>
      message.messageData?.sensitiveOutputBlocked === true ||
      isSensitiveOutputBlockedValue(deferredAnswer),
    [deferredAnswer, message.messageData]
  );
  const displayAnswer = isSensitiveBlocked ? t('sensitiveOutput.blocked') : deferredAnswer;
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
  const hasAi = useMemo(() => displayAnswer && displayAnswer.length > 0, [displayAnswer]);
  const hasAddon = Boolean(messageAddon);
  const generatedImages = useMemo(() => message.generatedImages || [], [message.generatedImages]);
  const hasImages = generatedImages.length > 0;

  const isClientLoading = useMemo(() => {
    const phase = message.clientState?.phase ?? 'idle';
    return phase === 'requesting' || phase === 'streaming';
  }, [message.clientState?.phase]);
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
            <div className="whitespace-pre-wrap">{message.query}</div>
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
              {isClientLoading && !message.WorkflowRunInfo && (
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
              <MarkdownViewer className="md-viewer break-words" content={displayAnswer} />
            ) : isClientLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-4 w-2/3" />
                <Skeleton className="h-4 w-1/2" />
                <Skeleton className="h-4 w-3/4" />
              </div>
            ) : isMessageEnd && !hasImages && !hasAddon && !hasQuestionAnswerTranscript ? (
              <span className="text-muted-foreground break-words">--</span>
            ) : null}

            {hasImages && (
              <div className="mt-4 grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4">
                {generatedImages.map((img, idx) => (
                  <div
                    key={idx}
                    className="relative aspect-square overflow-hidden rounded-lg border bg-muted group"
                  >
                    {img.isLoading ? (
                      <Skeleton className="h-full w-full" />
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

export default MessageItem;
