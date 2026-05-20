import { useMemo } from 'react';
import { useChatStore } from '@/components/chat/store';
import type {
  ChatRunCallbacks,
  ChatRunFinishedContext,
  ChatRunPausedContext,
  NodeInfo,
} from '@/components/chat/types';
import type { MessageEndEvent } from '@/hooks/workflow/use-run-workflow-chat-draft-stream';
import type { WorkflowStartedEvent } from '@/hooks/workflow/use-run-workflow-chat-draft-stream';
import { eventBus } from '@/lib/event-bus';
import { createTextStreamThrottler } from '@/utils/throttle-text-stream';
import { STREAM_RENDER_THROTTLE_MS } from '@/lib/config';
import { SENSITIVE_OUTPUT_BLOCKED_TOKEN } from '@/utils/model-output-filter';
import { resolveAnswerMergeMode } from '@/components/chat/utils/answer-merge';
import { isQuestionAnswerPromptMessage } from '@/components/workflow/question-answer/runtime-events';

export function useChatApi() {
  const getConversations = useChatStore.use.getConversations();
  const getConversation = useChatStore.use.getConversation();
  const updateConversation = useChatStore.use.updateConversation();
  const deleteConversation = useChatStore.use.deleteConversation();
  const appendUserMessage = useChatStore.use.appendUserMessage();
  const ensureAiMessage = useChatStore.use.ensureAiMessage();
  const appendAiChunk = useChatStore.use.appendAiChunk();
  const mergeAiMessage = useChatStore.use.mergeAiMessage();
  const replaceAiAnswer = useChatStore.use.replaceAiAnswer();
  const finalizeAiMessage = useChatStore.use.finalizeAiMessage();
  const pauseAiMessage = useChatStore.use.pauseAiMessage();
  const resumeAiMessage = useChatStore.use.resumeAiMessage();
  const updateRunNode = useChatStore.use.updateRunNode();
  const updateMessageInputs = useChatStore.use.updateMessageInputs();

  const onAgentRun = useMemo(
    () => (conversationId: string, callbacks: Partial<ChatRunCallbacks>) => {
      let tempKey: string | null = null;
      const throttler = createTextStreamThrottler(STREAM_RENDER_THROTTLE_MS, (text: string) => {
        if (!tempKey) return;
        appendAiChunk(conversationId, tempKey, text);
      });
      const safe = {
        onWorkflowStarted: callbacks.onWorkflowStarted ?? (() => {}),
        onTextChunk: callbacks.onTextChunk ?? (() => {}),
        onTextReplace: callbacks.onTextReplace ?? (() => {}),
        onNodeStarted: callbacks.onNodeStarted ?? (() => {}),
        onNodeFinished: callbacks.onNodeFinished ?? (() => {}),
        onError: callbacks.onError ?? (() => {}),
        onWorkflowFinished: callbacks.onWorkflowFinished ?? (() => {}),
        onMessageEnd: callbacks.onMessageEnd ?? (() => {}),
        onMessage: callbacks.onMessage ?? (() => {}),
        onWorkflowPaused: callbacks.onWorkflowPaused ?? (() => {}),
      } satisfies ChatRunCallbacks;

      return {
        // Accept either local kickoff context or SSE workflow_started event
        onWorkflowStarted: (
          ctx:
            | { query: string; tempKey?: string; inputs?: Record<string, unknown> }
            | (WorkflowStartedEvent & { inputs?: Record<string, unknown> })
        ) => {
          // If already started (tempKey exists), treat as metadata enrichment only to avoid duplicates
          if (tempKey) {
            let query = '';
            let runId: string | undefined;
            let workflowId: string | undefined;
            let createdAt: number | undefined;
            let inputs: Record<string, unknown> | undefined;

            if (typeof (ctx as { query?: string }).query === 'string') {
              const local = ctx as { query: string; inputs?: Record<string, unknown> };
              query = local.query || '';
              inputs = local.inputs;
            } else {
              const ev = ctx as WorkflowStartedEvent & { inputs?: Record<string, unknown> };
              runId = ev.id;
              workflowId = ev.workflow_id;
              createdAt = ev.created_at;
              inputs = ev.inputs;
              const sys = (inputs?.['sys'] as Record<string, unknown> | undefined) ?? undefined;
              const inferred = typeof sys?.['query'] === 'string' ? (sys['query'] as string) : '';
              query = inferred;
            }

            if (inputs) {
              updateMessageInputs(conversationId, tempKey, inputs);
            }
            mergeAiMessage(conversationId, tempKey, {
              workflowRunId: runId,
            });
            resumeAiMessage(conversationId, tempKey, { workflowRunId: runId });

            safe.onWorkflowStarted({
              query,
              tempKey,
              workflowRunId: runId,
              workflowId,
              createdAt,
              inputs,
            });
            // Publish scroll signal for this conversation
            eventBus.publish('chat:scroll', {
              conversationId,
              type: 'workflow_started',
              tempKey,
            });
            return;
          }

          // Extract query from local or SSE payload
          let query = '';
          let runId: string | undefined;
          let workflowId: string | undefined;
          let createdAt: number | undefined;
          let providedTempKey: string | undefined;
          let inputs: Record<string, unknown> | undefined;

          if (typeof (ctx as { query?: string }).query === 'string') {
            const local = ctx as {
              query: string;
              tempKey?: string;
              inputs?: Record<string, unknown>;
            };
            query = local.query || '';
            providedTempKey = local.tempKey;
            inputs = local.inputs;
          } else {
            const ev = ctx as WorkflowStartedEvent & { inputs?: Record<string, unknown> };
            runId = ev.id;
            workflowId = ev.workflow_id;
            createdAt = ev.created_at;
            inputs = ev.inputs;
            // try to infer query from inputs if available
            const sys = (inputs?.['sys'] as Record<string, unknown> | undefined) ?? undefined;
            const inferred = typeof sys?.['query'] === 'string' ? (sys['query'] as string) : '';
            query = inferred;
          }

          const result = appendUserMessage(conversationId, {
            query,
            tempKey: providedTempKey,
            inputs,
          });
          tempKey = result.tempKey;
          ensureAiMessage(conversationId, tempKey);
          safe.onWorkflowStarted({
            query,
            tempKey,
            workflowRunId: runId,
            workflowId,
            createdAt,
            inputs,
          });
          eventBus.publish('chat:scroll', {
            conversationId,
            type: 'workflow_started',
            tempKey,
          });
        },
        onTextChunk: (text: string) => {
          if (!tempKey) return;
          throttler.append(text);
          safe.onTextChunk(text);
          eventBus.publish('chat:scroll', {
            conversationId,
            type: 'message',
            tempKey,
          });
        },
        onTextReplace: () => {
          if (!tempKey) return;
          throttler.cancel();
          replaceAiAnswer(conversationId, tempKey, SENSITIVE_OUTPUT_BLOCKED_TOKEN, {
            sensitiveOutputBlocked: true,
          });
          safe.onTextReplace?.();
          eventBus.publish('chat:scroll', {
            conversationId,
            type: 'message',
            tempKey,
          });
        },
        onMessage: (meta?: Record<string, unknown>) => {
          if (!tempKey) return;

          if (isQuestionAnswerPromptMessage(meta)) {
            safe.onMessage?.(meta);
            return;
          }

          const m = (meta ?? {}) as Record<string, unknown>;
          let chunk = '';
          if (typeof m['answer'] === 'string') chunk = m['answer'] as string;
          else if (typeof m['text'] === 'string') chunk = m['text'] as string;
          else if (typeof m['content'] === 'string') chunk = m['content'] as string;
          else if (typeof m['delta'] === 'string') chunk = m['delta'] as string;
          else if (m['outputs'] && typeof m['outputs'] === 'object') {
            const out = m['outputs'] as Record<string, unknown>;
            if (typeof out['answer'] === 'string') chunk = out['answer'] as string;
            else if (typeof out['text'] === 'string') chunk = out['text'] as string;
          }

          if (chunk.length > 0) {
            const messageId =
              typeof m['message_id'] === 'string' ? (m['message_id'] as string) : '';
            const workflowRunId =
              (typeof m['workflow_run_id'] === 'string' ? (m['workflow_run_id'] as string) : '') ||
              (typeof m['id'] === 'string' ? (m['id'] as string) : '');
            const currentMessage = getConversation(conversationId)?.messages.find(
              item => item.messageData?.tempKey === tempKey
            );
            const shouldReplacePersistentAnswer =
              Boolean(messageId || workflowRunId) &&
              (currentMessage?.WorkflowRunInfo?.status === 'pending_approval' ||
                currentMessage?.WorkflowRunInfo?.status === 'pending_question' ||
                currentMessage?.clientState?.status === 'pending_approval' ||
                currentMessage?.clientState?.status === 'pending_question');

            if (shouldReplacePersistentAnswer) {
              const answerMode = resolveAnswerMergeMode(currentMessage?.answer ?? '', chunk);
              if (answerMode === 'replace') {
                throttler.cancel();
              } else if (answerMode === 'append') {
                throttler.append(chunk);
              }
              mergeAiMessage(conversationId, tempKey, {
                ...(answerMode !== 'skip' && answerMode === 'replace'
                  ? { answer: chunk, answerMode }
                  : {}),
                messageId: messageId || undefined,
                workflowRunId: workflowRunId || undefined,
                conversationId:
                  typeof m['conversation_id'] === 'string'
                    ? (m['conversation_id'] as string)
                    : undefined,
              });
            } else {
              throttler.append(chunk);
              if (messageId || workflowRunId) {
                mergeAiMessage(conversationId, tempKey, {
                  messageId: messageId || undefined,
                  workflowRunId: workflowRunId || undefined,
                });
              }
            }
          }

          safe.onMessage?.(meta);
        },
        onNodeStarted: (n: NodeInfo) => {
          if (!tempKey) return;
          const message = getConversation(conversationId)?.messages.find(
            item => item.messageData?.tempKey === tempKey
          );
          if (message?.clientState?.status === 'stopped') return;
          resumeAiMessage(conversationId, tempKey);
          updateRunNode(conversationId, tempKey, n);
          safe.onNodeStarted(n);
          eventBus.publish('chat:scroll', {
            conversationId,
            type: 'node_started',
            tempKey,
          });
        },
        onNodeFinished: (n: NodeInfo) => {
          if (!tempKey) return;
          updateRunNode(conversationId, tempKey, n);
          safe.onNodeFinished(n);
          eventBus.publish('chat:scroll', {
            conversationId,
            type: 'node_finished',
            tempKey,
          });
        },
        onError: (error: unknown) => {
          if (tempKey) {
            // Ensure message loading state is cleared on error
            finalizeAiMessage(conversationId, tempKey, {
              status: 'error',
              error: typeof error === 'string' ? error : 'Stream error',
            });
            eventBus.publish('chat:scroll', {
              conversationId,
              type: 'error',
              tempKey,
            });
          }
          safe.onError(typeof error === 'string' ? error : 'Unknown error');
        },
        onWorkflowFinished: (ctx: ChatRunFinishedContext) => {
          if (!tempKey) return;
          throttler.flush();
          finalizeAiMessage(conversationId, tempKey, ctx);
          safe.onWorkflowFinished(ctx);
          eventBus.publish('chat:scroll', {
            conversationId,
            type: 'workflow_finished',
            tempKey,
          });
        },
        onMessageEnd: (ctx: MessageEndEvent) => {
          if (!tempKey) return;
          mergeAiMessage(conversationId, tempKey, {
            messageId: typeof ctx.message_id === 'string' ? ctx.message_id : undefined,
            workflowRunId:
              (typeof ctx.workflow_run_id === 'string' ? ctx.workflow_run_id : '') ||
              (typeof ctx.id === 'string' ? ctx.id : '') ||
              undefined,
            conversationId:
              typeof ctx.conversation_id === 'string' ? ctx.conversation_id : undefined,
            metadata:
              ctx.metadata && typeof ctx.metadata === 'object'
                ? (ctx.metadata as Record<string, unknown>)
                : undefined,
          });
          safe.onMessageEnd(ctx);
          eventBus.publish('chat:scroll', {
            conversationId,
            type: 'message_end',
            tempKey,
          });
        },
        onWorkflowPaused: (ctx: ChatRunPausedContext = {}) => {
          if (!tempKey) return;
          const currentTempKey = tempKey;
          throttler.flush();
          pauseAiMessage(conversationId, currentTempKey, ctx);
          ctx.nodeIds?.forEach(nodeId => {
            updateRunNode(conversationId, currentTempKey, {
              status: 'paused',
              nodeId,
              nodeType: ctx.nodeType ?? 'approval',
            });
          });
          safe.onWorkflowPaused?.(ctx);
          eventBus.publish('chat:scroll', {
            conversationId,
            type: 'workflow_paused',
            tempKey: currentTempKey,
          });
        },
        mergeMessageData: (data: Record<string, unknown>) => {
          if (!tempKey) return;
          mergeAiMessage(conversationId, tempKey, { messageData: data });
        },
        // cleanup hook for future bus subscription
        dispose: () => {
          throttler.cancel();
        },
      };
    },
    [
      appendUserMessage,
      ensureAiMessage,
      appendAiChunk,
      mergeAiMessage,
      getConversation,
      replaceAiAnswer,
      finalizeAiMessage,
      pauseAiMessage,
      resumeAiMessage,
      updateMessageInputs,
      updateRunNode,
    ]
  );

  return {
    getConversations,
    getConversation,
    updateConversation,
    deleteConversation,
    onAgentRun,
  } as const;
}

export type UseChatApi = ReturnType<typeof useChatApi>;
