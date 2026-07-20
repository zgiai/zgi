import { useCallback } from 'react';
import type { MutableRefObject } from 'react';

import type { Message } from '@/components/chat/types';
import { resolveAnswerMergeMode } from '@/components/chat/utils/answer-merge';
import { useChatStore } from '@/components/chat/store';
import type {
  UseWorkflowRunEventsStreamReturn,
  WorkflowRunEventsStreamParams,
} from '@/hooks/workflow/use-workflow-run-events-stream';
import type { WorkflowConnectionState } from '@/hooks/workflow/workflow-runtime-controller';
import { mapNode, unwrap } from '@/utils/webapp/run-mappers';
import {
  isQuestionAnswerPromptMessage,
  parseQuestionAnswerPausedEvent,
  parseQuestionAnswerRequestedEvent,
  type QuestionAnswerTranscriptItem,
} from '@/components/workflow/question-answer/runtime-events';
import {
  createWorkflowSnapshotPauseEvent,
  parseApprovalPausedEvent,
  parseApprovalRequestedEvent,
} from '@/components/workflow/approval/runtime-events';

import { normalizeFinalRunStatus, stripQuestionAnswerPromptText } from './mappers';
import { createWorkflowRunNodeAccumulator } from '@/utils/webapp/workflow-run-node-accumulator';

interface UseWebappWorkflowRunEventsArgs {
  startWorkflowRunEvents: UseWorkflowRunEventsStreamReturn['start'];
  cancelWorkflowRunEvents: UseWorkflowRunEventsStreamReturn['cancel'];
  restoredRunRef: MutableRefObject<Set<string>>;
  questionAnswerTranscriptRef: MutableRefObject<QuestionAnswerTranscriptItem[]>;
  setLatestTaskId: (taskId: string | null) => void;
  getWorkflowRunErrorText: (error: unknown) => string | undefined;
  handleApprovalExpired: (payload: unknown) => void;
  handleApprovalRequested: (payload: unknown) => void;
  handleApprovalResultFilled: (payload: unknown) => void;
  handleQuestionAnswerRequested: (payload: unknown) => void;
  handleQuestionAnswerSubmitted: (payload: unknown) => void;
  resetApprovalRuntime: () => void;
  clearQuestionAnswerRuntime?: () => void;
  workflowRunFailedText: string;
  setConversationConnectionState?: (conversationId: string, state: WorkflowConnectionState) => void;
  reconcileConversationRun?: (
    conversationId: string,
    workflowRunId: string
  ) => Promise<WorkflowConnectionState>;
}

export function useWebappWorkflowRunEvents({
  startWorkflowRunEvents,
  cancelWorkflowRunEvents,
  restoredRunRef,
  questionAnswerTranscriptRef,
  setLatestTaskId,
  getWorkflowRunErrorText,
  handleApprovalExpired,
  handleApprovalRequested,
  handleApprovalResultFilled,
  handleQuestionAnswerRequested,
  handleQuestionAnswerSubmitted,
  resetApprovalRuntime,
  clearQuestionAnswerRuntime,
  workflowRunFailedText,
  setConversationConnectionState,
  reconcileConversationRun,
}: UseWebappWorkflowRunEventsArgs) {
  const startWorkflowRunEventStream = useCallback(
    (conversationId: string, message: Message, params?: WorkflowRunEventsStreamParams) => {
      const workflowRunId =
        (typeof message.WorkflowRunInfo?.id === 'string' ? message.WorkflowRunInfo.id : '') ||
        (typeof message.messageData?.workflow_run_id === 'string'
          ? (message.messageData.workflow_run_id as string)
          : '');
      if (!conversationId || !workflowRunId) return;
      if (restoredRunRef.current.has(workflowRunId)) return;

      const tempKey =
        typeof message.messageData?.tempKey === 'string'
          ? (message.messageData.tempKey as string)
          : `restore:${workflowRunId}`;
      const messageId =
        message.messageId ||
        (typeof message.messageData?.message_id === 'string'
          ? (message.messageData.message_id as string)
          : undefined);
      const isForegroundConversation = () => useChatStore.getState().currentId === conversationId;

      restoredRunRef.current.add(workflowRunId);
      setLatestTaskId(workflowRunId);
      useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
        messageId,
        workflowRunId,
        conversationId,
      });

      const runNodes = createWorkflowRunNodeAccumulator({
        onNodeStarted: node => {
          useChatStore.getState().resumeAiMessage(conversationId, tempKey, { workflowRunId });
          useChatStore.getState().updateRunNode(conversationId, tempKey, node);
        },
        onNodeFinished: node => {
          useChatStore.getState().updateRunNode(conversationId, tempKey, node);
        },
      });

      const effectiveParams = params ?? { include_snapshot: true, continue_on_pause: true };

      void startWorkflowRunEvents(
        workflowRunId,
        {
          onWorkflowSnapshot: payload => {
            const snapshot = unwrap(payload);
            const message =
              snapshot.message && typeof snapshot.message === 'object'
                ? (snapshot.message as Record<string, unknown>)
                : null;
            if (message && typeof message.answer === 'string') {
              useChatStore.getState().replaceAiAnswer(conversationId, tempKey, message.answer, {
                projection_revision:
                  typeof message.projection_revision === 'number'
                    ? message.projection_revision
                    : undefined,
              });
            }
            const nodes = Array.isArray(snapshot.nodes) ? snapshot.nodes : [];
            useChatStore.getState().replaceRunNodes(
              conversationId,
              tempKey,
              nodes.map(node => {
                const record =
                  node && typeof node === 'object' ? (node as Record<string, unknown>) : {};
                const status = typeof record.status === 'string' ? record.status.toLowerCase() : '';
                return mapNode(node, !['running', 'pending'].includes(status));
              })
            );

            const run =
              snapshot.workflow_run && typeof snapshot.workflow_run === 'object'
                ? (snapshot.workflow_run as Record<string, unknown>)
                : null;
            const runStatus = typeof run?.status === 'string' ? run.status.toLowerCase() : '';
            if (
              [
                'succeeded',
                'success',
                'completed',
                'failed',
                'error',
                'stopped',
                'expired',
              ].includes(runStatus)
            ) {
              useChatStore.getState().finalizeAiMessage(conversationId, tempKey, {
                status: normalizeFinalRunStatus(runStatus),
                error: getWorkflowRunErrorText(run?.error),
                messageId: message && typeof message.id === 'string' ? message.id : messageId,
                workflowRunId,
                model: null,
              });
              if (isForegroundConversation()) {
                resetApprovalRuntime();
                clearQuestionAnswerRuntime?.();
              }
              restoredRunRef.current.delete(workflowRunId);
              setLatestTaskId(null);
              return;
            }

            const pausePayload = createWorkflowSnapshotPauseEvent(payload);
            if (pausePayload) {
              const parsed = parseApprovalPausedEvent(pausePayload);
              if (parsed.isApproval) {
                if (isForegroundConversation()) handleApprovalRequested(pausePayload);
                useChatStore.getState().pauseAiMessage(conversationId, tempKey, {
                  workflowRunId,
                  status: 'pending_approval',
                });
              } else {
                const qaPaused = parseQuestionAnswerPausedEvent(pausePayload);
                if (qaPaused.isQuestionAnswer) {
                  if (qaPaused.prompt && isForegroundConversation()) {
                    handleQuestionAnswerRequested(qaPaused.prompt);
                  }
                  useChatStore.getState().pauseAiMessage(conversationId, tempKey, {
                    workflowRunId,
                    status: 'pending_question',
                  });
                }
              }
              return;
            }

            if (isForegroundConversation()) {
              resetApprovalRuntime();
              clearQuestionAnswerRuntime?.();
            }
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              messageData: { questionAnswerPrompt: null },
            });
            useChatStore.getState().resumeAiMessage(conversationId, tempKey, { workflowRunId });
          },
          onWorkflowStarted: payload => {
            const data = unwrap(payload);
            const serverConversationId =
              typeof data.conversation_id === 'string'
                ? (data.conversation_id as string)
                : conversationId;
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              messageId:
                typeof data.message_id === 'string' ? (data.message_id as string) : messageId,
              workflowRunId,
              conversationId: serverConversationId,
            });
            useChatStore.getState().resumeAiMessage(conversationId, tempKey, { workflowRunId });
          },
          onWorkflowResumed: () => {
            if (isForegroundConversation()) {
              resetApprovalRuntime();
              clearQuestionAnswerRuntime?.();
            }
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              messageData: { questionAnswerPrompt: null },
            });
            useChatStore.getState().resumeAiMessage(conversationId, tempKey, { workflowRunId });
          },
          onApprovalRequested: payload => {
            if (isForegroundConversation()) handleApprovalRequested(payload);
            const parsed = parseApprovalRequestedEvent(payload);
            if (parsed.form?.node_id) {
              useChatStore.getState().updateRunNode(conversationId, tempKey, {
                status: 'paused',
                nodeId: parsed.form.node_id,
                nodeType: 'approval',
                title: parsed.form.node_title || parsed.form.node_id,
              });
            }
          },
          onApprovalResultFilled: payload => {
            if (isForegroundConversation()) handleApprovalResultFilled(payload);
          },
          onApprovalExpired: payload => {
            if (isForegroundConversation()) handleApprovalExpired(payload);
            const data = unwrap(payload);
            useChatStore.getState().finalizeAiMessage(conversationId, tempKey, {
              status: 'expired',
              elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
              messageId:
                typeof data.message_id === 'string' ? (data.message_id as string) : messageId,
              workflowRunId,
              model: null,
            });
            setLatestTaskId(null);
            restoredRunRef.current.delete(workflowRunId);
            cancelWorkflowRunEvents(workflowRunId);
          },
          onQuestionAnswerRequested: payload => {
            if (isForegroundConversation()) handleQuestionAnswerRequested(payload);
            const parsed = parseQuestionAnswerRequestedEvent(payload);
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              messageData: {
                questionAnswerTranscript: questionAnswerTranscriptRef.current,
                ...(parsed
                  ? {
                      questionAnswerPrompt: {
                        question: parsed.question,
                        choices: parsed.choices,
                        round: parsed.round,
                      },
                    }
                  : {}),
              },
            });
            useChatStore.getState().pauseAiMessage(conversationId, tempKey, {
              workflowRunId: parsed?.workflowRunId || workflowRunId,
              status: 'pending_question',
            });
            if (parsed?.nodeId) {
              useChatStore.getState().updateRunNode(conversationId, tempKey, {
                status: 'paused',
                nodeId: parsed.nodeId,
                nodeType: 'question-answer',
                title: parsed.nodeTitle || parsed.nodeId,
              });
            }
          },
          onQuestionAnswerSubmitted: payload => {
            if (isForegroundConversation()) handleQuestionAnswerSubmitted(payload);
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              messageData: {
                questionAnswerTranscript: questionAnswerTranscriptRef.current,
                questionAnswerPrompt: null,
              },
            });
          },
          onWorkflowPaused: payload => {
            const parsed = parseApprovalPausedEvent(payload);
            const data = unwrap(payload);
            if (parsed.isApproval) {
              if (isForegroundConversation()) handleApprovalRequested(payload);
              useChatStore.getState().pauseAiMessage(conversationId, tempKey, {
                elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
                workflowRunId,
                status: 'pending_approval',
              });
              parsed.nodeIds.forEach(nodeId => {
                useChatStore.getState().updateRunNode(conversationId, tempKey, {
                  status: 'paused',
                  nodeId,
                  nodeType: 'approval',
                });
              });
              return;
            }
            const qaPaused = parseQuestionAnswerPausedEvent(payload);
            if (!qaPaused.isQuestionAnswer) return;
            if (qaPaused.prompt) {
              if (isForegroundConversation()) handleQuestionAnswerRequested(qaPaused.prompt);
              useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
                messageData: {
                  questionAnswerTranscript: questionAnswerTranscriptRef.current,
                  questionAnswerPrompt: {
                    question: qaPaused.prompt.question,
                    choices: qaPaused.prompt.choices,
                    round: qaPaused.prompt.round,
                  },
                },
              });
            }
            useChatStore.getState().pauseAiMessage(conversationId, tempKey, {
              elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
              workflowRunId: qaPaused.workflowRunId || workflowRunId,
              status: 'pending_question',
            });
            qaPaused.nodeIds.forEach(nodeId => {
              useChatStore.getState().updateRunNode(conversationId, tempKey, {
                status: 'paused',
                nodeId,
                nodeType: 'question-answer',
                title:
                  qaPaused.prompt?.nodeId === nodeId ? qaPaused.prompt.nodeTitle || nodeId : nodeId,
              });
            });
          },
          onNodeStarted: runNodes.onNodeStarted,
          onNodeFinished: runNodes.onNodeFinished,
          onIterationStarted: runNodes.onIterationStarted,
          onIterationNext: runNodes.onIterationNext,
          onIterationCompleted: runNodes.onIterationCompleted,
          onLoopStarted: runNodes.onLoopStarted,
          onLoopNext: runNodes.onLoopNext,
          onLoopCompleted: runNodes.onLoopCompleted,
          onTextChunk: payload => {
            const data = unwrap(payload);
            const chunk =
              typeof payload === 'string'
                ? payload
                : typeof data.text === 'string'
                  ? (data.text as string)
                  : typeof data.answer === 'string'
                    ? (data.answer as string)
                    : typeof data.delta === 'string'
                      ? (data.delta as string)
                      : '';
            if (!chunk) return;
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              answer: chunk,
              answerMode: 'append',
              messageId,
              workflowRunId,
              conversationId,
            });
          },
          onMessage: payload => {
            const data = unwrap(payload);
            const checkpointDelta =
              typeof data.answer_delta === 'string' ? (data.answer_delta as string) : null;
            const checkpointRevision =
              typeof data.answer_revision === 'number' ? data.answer_revision : 0;
            if (checkpointDelta !== null && checkpointRevision > 0) {
              const checkpointMetadata = { projection_revision: checkpointRevision };
              if (data.replace === true) {
                useChatStore
                  .getState()
                  .replaceAiAnswer(conversationId, tempKey, checkpointDelta, checkpointMetadata);
              } else {
                useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
                  answer: checkpointDelta,
                  answerMode: 'append',
                  messageData: checkpointMetadata,
                  messageId,
                  workflowRunId,
                  conversationId,
                });
              }
              return;
            }
            const messageData = isQuestionAnswerPromptMessage(data)
              ? stripQuestionAnswerPromptText(data)
              : data;
            const answer =
              typeof messageData.answer === 'string'
                ? (messageData.answer as string)
                : typeof messageData.text === 'string'
                  ? (messageData.text as string)
                  : typeof messageData.content === 'string'
                    ? (messageData.content as string)
                    : typeof messageData.delta === 'string'
                      ? (messageData.delta as string)
                      : '';
            const currentMessage = useChatStore
              .getState()
              .conversations[
                conversationId
              ]?.messages.find(item => item.messageData?.tempKey === tempKey);
            const answerMode = resolveAnswerMergeMode(currentMessage?.answer ?? '', answer);
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              ...(answerMode !== 'skip' ? { answer, answerMode } : {}),
              messageId:
                typeof messageData.message_id === 'string'
                  ? (messageData.message_id as string)
                  : messageId,
              workflowRunId:
                (typeof messageData.workflow_run_id === 'string'
                  ? (messageData.workflow_run_id as string)
                  : '') ||
                (typeof messageData.id === 'string' ? (messageData.id as string) : '') ||
                workflowRunId,
              conversationId:
                typeof messageData.conversation_id === 'string'
                  ? (messageData.conversation_id as string)
                  : conversationId,
            });
          },
          onMessageEnd: payload => {
            const data = unwrap(payload);
            useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
              messageId:
                typeof data.message_id === 'string' ? (data.message_id as string) : messageId,
              workflowRunId:
                (typeof data.workflow_run_id === 'string'
                  ? (data.workflow_run_id as string)
                  : '') ||
                (typeof data.id === 'string' ? (data.id as string) : '') ||
                workflowRunId,
              conversationId:
                typeof data.conversation_id === 'string'
                  ? (data.conversation_id as string)
                  : conversationId,
              metadata:
                data.metadata && typeof data.metadata === 'object'
                  ? (data.metadata as Record<string, unknown>)
                  : undefined,
            });
          },
          onWorkflowFinished: payload => {
            const data = unwrap(payload);
            const status = normalizeFinalRunStatus(data.status);
            useChatStore.getState().finalizeAiMessage(conversationId, tempKey, {
              status,
              error: getWorkflowRunErrorText(data.error),
              elapsedTime: typeof data.elapsed_time === 'number' ? data.elapsed_time : undefined,
              messageId:
                typeof data.message_id === 'string' ? (data.message_id as string) : messageId,
              workflowRunId,
              model: null,
            });
            setLatestTaskId(null);
            if (isForegroundConversation()) {
              resetApprovalRuntime();
              clearQuestionAnswerRuntime?.();
            }
            restoredRunRef.current.delete(workflowRunId);
            cancelWorkflowRunEvents(workflowRunId);
          },
          onError: payload => {
            const errorText = getWorkflowRunErrorText(payload) ?? workflowRunFailedText;
            useChatStore.getState().finalizeAiMessage(conversationId, tempKey, {
              status: 'error',
              error: errorText,
              messageId,
              workflowRunId,
            });
            setLatestTaskId(null);
            restoredRunRef.current.delete(workflowRunId);
          },
        },
        effectiveParams,
        {
          onConnectionStateChange: state => setConversationConnectionState?.(conversationId, state),
          onReconnectExhausted: () => {
            restoredRunRef.current.delete(workflowRunId);
            if (reconcileConversationRun) {
              void reconcileConversationRun(conversationId, workflowRunId).then(state => {
                setConversationConnectionState?.(conversationId, state);
              });
            } else {
              setConversationConnectionState?.(conversationId, 'disconnected');
            }
          },
        }
      );
    },
    [
      cancelWorkflowRunEvents,
      clearQuestionAnswerRuntime,
      getWorkflowRunErrorText,
      handleApprovalExpired,
      handleApprovalRequested,
      handleApprovalResultFilled,
      handleQuestionAnswerRequested,
      handleQuestionAnswerSubmitted,
      questionAnswerTranscriptRef,
      reconcileConversationRun,
      resetApprovalRuntime,
      startWorkflowRunEvents,
      restoredRunRef,
      setLatestTaskId,
      setConversationConnectionState,
      workflowRunFailedText,
    ]
  );

  const resumeWorkflowRun = useCallback(
    (conversationId: string, message: Message) => {
      startWorkflowRunEventStream(conversationId, message, {
        include_snapshot: true,
        continue_on_pause: true,
      });
    },
    [startWorkflowRunEventStream]
  );

  const continueWorkflowRun = useCallback(
    (conversationId: string, message: Message) => {
      startWorkflowRunEventStream(conversationId, message);
    },
    [startWorkflowRunEventStream]
  );

  return { resumeWorkflowRun, continueWorkflowRun };
}
