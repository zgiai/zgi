import { useCallback } from 'react';
import type { MutableRefObject } from 'react';

import type { Message } from '@/components/chat/types';
import { resolveAnswerMergeMode } from '@/components/chat/utils/answer-merge';
import { useChatStore } from '@/components/chat/store';
import type {
  UseWorkflowRunEventsStreamReturn,
  WorkflowRunEventsStreamParams,
} from '@/hooks/workflow/use-workflow-run-events-stream';
import { unwrap, mapNode } from '@/utils/webapp/run-mappers';
import {
  isQuestionAnswerPromptMessage,
  parseQuestionAnswerPausedEvent,
  parseQuestionAnswerRequestedEvent,
  type QuestionAnswerTranscriptItem,
} from '@/components/workflow/question-answer/runtime-events';
import {
  parseApprovalPausedEvent,
  parseApprovalRequestedEvent,
} from '@/components/workflow/approval/runtime-events';

import {
  normalizeFinalRunStatus,
  stripQuestionAnswerPromptText,
} from './mappers';

interface UseWebappWorkflowRunEventsArgs {
  startWorkflowRunEvents: UseWorkflowRunEventsStreamReturn['start'];
  cancelWorkflowRunEvents: UseWorkflowRunEventsStreamReturn['cancel'];
  approvalCursor: number;
  restoredRunRef: MutableRefObject<string | null>;
  workflowFinishedRef: MutableRefObject<boolean>;
  questionAnswerTranscriptRef: MutableRefObject<QuestionAnswerTranscriptItem[]>;
  setLatestTaskId: (taskId: string | null) => void;
  getWorkflowRunErrorText: (error: unknown) => string | undefined;
  handleApprovalExpired: (payload: unknown) => void;
  handleApprovalRequested: (payload: unknown) => void;
  handleApprovalResultFilled: (payload: unknown) => void;
  handleQuestionAnswerRequested: (payload: unknown) => void;
  handleQuestionAnswerSubmitted: (payload: unknown) => void;
  hasUnresolvedApprovals: () => boolean;
  resetApprovalRuntime: () => void;
  workflowRunFailedText: string;
}

export function useWebappWorkflowRunEvents({
  startWorkflowRunEvents,
  cancelWorkflowRunEvents,
  approvalCursor,
  restoredRunRef,
  workflowFinishedRef,
  questionAnswerTranscriptRef,
  setLatestTaskId,
  getWorkflowRunErrorText,
  handleApprovalExpired,
  handleApprovalRequested,
  handleApprovalResultFilled,
  handleQuestionAnswerRequested,
  handleQuestionAnswerSubmitted,
  hasUnresolvedApprovals,
  resetApprovalRuntime,
  workflowRunFailedText,
}: UseWebappWorkflowRunEventsArgs) {
  const startWorkflowRunEventStream = useCallback(
    (conversationId: string, message: Message, params?: WorkflowRunEventsStreamParams) => {
      const workflowRunId =
        (typeof message.WorkflowRunInfo?.id === 'string' ? message.WorkflowRunInfo.id : '') ||
        (typeof message.messageData?.workflow_run_id === 'string'
          ? (message.messageData.workflow_run_id as string)
          : '');
      if (!conversationId || !workflowRunId) return;
      if (restoredRunRef.current === workflowRunId) return;

      const tempKey =
        typeof message.messageData?.tempKey === 'string'
          ? (message.messageData.tempKey as string)
          : `restore:${workflowRunId}`;
      const messageId =
        message.messageId ||
        (typeof message.messageData?.message_id === 'string'
          ? (message.messageData.message_id as string)
          : undefined);

      restoredRunRef.current = workflowRunId;
      workflowFinishedRef.current = false;
      setLatestTaskId(workflowRunId);
      useChatStore.getState().mergeAiMessage(conversationId, tempKey, {
        messageId,
        workflowRunId,
        conversationId,
      });

      const effectiveParams =
        params ??
        (approvalCursor > 0
          ? { after: approvalCursor, continue_on_pause: true }
          : { include_snapshot: true, continue_on_pause: true });

      void startWorkflowRunEvents(
        workflowRunId,
        {
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
          onApprovalRequested: payload => {
            handleApprovalRequested(payload);
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
          onApprovalResultFilled: handleApprovalResultFilled,
          onApprovalExpired: payload => {
            handleApprovalExpired(payload);
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
            restoredRunRef.current = null;
            cancelWorkflowRunEvents();
          },
          onQuestionAnswerRequested: payload => {
            handleQuestionAnswerRequested(payload);
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
            handleQuestionAnswerSubmitted(payload);
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
              handleApprovalRequested(payload);
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
              handleQuestionAnswerRequested(qaPaused.prompt);
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
                  qaPaused.prompt?.nodeId === nodeId
                    ? qaPaused.prompt.nodeTitle || nodeId
                    : nodeId,
              });
            });
          },
          onNodeStarted: payload => {
            useChatStore.getState().resumeAiMessage(conversationId, tempKey, { workflowRunId });
            useChatStore.getState().updateRunNode(conversationId, tempKey, mapNode(payload, false));
          },
          onNodeFinished: payload => {
            useChatStore.getState().updateRunNode(conversationId, tempKey, mapNode(payload, true));
          },
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
            const rawStatus = typeof data.status === 'string' ? data.status.toLowerCase() : '';
            const isSuccessfulTerminalStatus = !['failed', 'error', 'stopped', 'expired'].includes(
              rawStatus
            );
            if (isSuccessfulTerminalStatus && hasUnresolvedApprovals()) {
              useChatStore.getState().pauseAiMessage(conversationId, tempKey, {
                workflowRunId,
              });
              return;
            }
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
            workflowFinishedRef.current = true;
            resetApprovalRuntime();
            restoredRunRef.current = null;
            cancelWorkflowRunEvents();
          },
          onError: payload => {
            const errorText =
              getWorkflowRunErrorText(payload) ?? workflowRunFailedText;
            useChatStore.getState().finalizeAiMessage(conversationId, tempKey, {
              status: 'error',
              error: errorText,
              messageId,
              workflowRunId,
            });
            setLatestTaskId(null);
            restoredRunRef.current = null;
          },
        },
        effectiveParams,
        {
          onClose: () => {
            restoredRunRef.current = null;
            if (!workflowFinishedRef.current) {
              window.setTimeout(() => {
                if (!workflowFinishedRef.current) {
                  startWorkflowRunEventStream(conversationId, message);
                }
              }, 1000);
            }
          },
        }
      );
    },
    [
      approvalCursor,
      cancelWorkflowRunEvents,
      getWorkflowRunErrorText,
      handleApprovalExpired,
      handleApprovalRequested,
      handleApprovalResultFilled,
      handleQuestionAnswerRequested,
      handleQuestionAnswerSubmitted,
      hasUnresolvedApprovals,
      questionAnswerTranscriptRef,
      resetApprovalRuntime,
      startWorkflowRunEvents,
      restoredRunRef,
      setLatestTaskId,
      workflowFinishedRef,
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
