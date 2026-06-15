import type { ConversationSummary, ConversationDetail } from '@/components/chat/controllers/types';
import type { Message, TerminalRunStatus } from '@/components/chat/types';
import { normalizeMessageRunStatus } from '@/components/chat/types';
import { useChatStore } from '@/components/chat/store';
import type {
  WebAppConversation,
  WebAppConversationDetail,
  WebAppConversationMessageItem,
} from '@/services/types/webapp';
import type { QuestionAnswerChoice } from '@/services/types/workflow';
import {
  parseQuestionAnswerRequestedEvent,
  type QuestionAnswerTranscriptItem,
} from '@/components/workflow/question-answer/runtime-events';
import { normalizeQuestionAnswerTranscript } from '@/components/workflow/question-answer/question-answer-transcript';
import { normalizeWorkflowBillingCode } from '@/utils/workflow/billing';

export interface ParsedSseRunError {
  code?: string | number;
  message?: string;
}

function getQuestionAnswerTranscriptFromMetadata(
  metadata: unknown
): QuestionAnswerTranscriptItem[] {
  if (!metadata || typeof metadata !== 'object') return [];
  const record = metadata as Record<string, unknown>;
  return normalizeQuestionAnswerTranscript(record.questionAnswerTranscript);
}

function getPendingQuestionAnswerPromptFromMessage(item: WebAppConversationMessageItem):
  | {
      question: string;
      choices: QuestionAnswerChoice[];
      round?: number;
    }
  | null {
  if (normalizeMessageRunStatus(item.status) !== 'pending_question') return null;
  const metadataPrompt = parseQuestionAnswerRequestedEvent(item.message_metadata?.questionAnswerPrompt);
  if (metadataPrompt) {
    return {
      question: metadataPrompt.question,
      choices: metadataPrompt.choices,
      round: metadataPrompt.round,
    };
  }
  const transcript = getQuestionAnswerTranscriptFromMetadata(item.message_metadata);
  for (let i = transcript.length - 1; i >= 0; i -= 1) {
    const entry = transcript[i];
    if (!entry.question || entry.answer) continue;
    return {
      question: entry.question,
      choices: [],
      round: entry.round,
    };
  }
  return null;
}

export function parseSseRunError(error: unknown): ParsedSseRunError {
  const parsed =
    error && typeof error === 'object'
      ? (error as Record<string, unknown>)
      : typeof error === 'string'
        ? { message: error }
        : error instanceof Error
          ? { message: error.message }
          : {};

  return {
    code: parsed['code'] as string | number | undefined,
    message: parsed['message'] as string | undefined,
  };
}

export function isWorkspaceNotFoundError(error: ParsedSseRunError): boolean {
  if (normalizeWorkflowBillingCode(error.code) === '205004') return true;
  return error.message?.toLowerCase() === 'workspace not found';
}

export function stripQuestionAnswerPromptText(data: Record<string, unknown>): Record<string, unknown> {
  const next = { ...data };
  delete next.answer;
  delete next.text;
  delete next.content;
  delete next.delta;
  if (next.outputs && typeof next.outputs === 'object') {
    const outputs = { ...(next.outputs as Record<string, unknown>) };
    delete outputs.answer;
    delete outputs.text;
    next.outputs = outputs;
  }
  return next;
}

export function hasPendingQuestionAnswerMessage(conversationId?: string): boolean {
  if (!conversationId) return false;
  const messages = useChatStore.getState().conversations[conversationId]?.messages ?? [];
  const latestMessage = messages[messages.length - 1];
  return (
    latestMessage?.WorkflowRunInfo?.status === 'pending_question' ||
    latestMessage?.clientState?.status === 'pending_question'
  );
}

export function getPendingQuestionAnswerPromptFromRuntimeMessage(message?: Message):
  | {
      question: string;
      choices: QuestionAnswerChoice[];
      round?: number;
    }
  | null {
  const runStatus = message?.WorkflowRunInfo?.status ?? message?.clientState?.status;
  if (runStatus !== 'pending_question') return null;

  const metadata =
    message?.messageData?.metadata && typeof message.messageData.metadata === 'object'
      ? (message.messageData.metadata as Record<string, unknown>)
      : undefined;
  const metadataPrompt = parseQuestionAnswerRequestedEvent(
    message?.messageData?.questionAnswerPrompt ?? metadata?.questionAnswerPrompt
  );
  if (metadataPrompt) {
    return {
      question: metadataPrompt.question,
      choices: metadataPrompt.choices,
      round: metadataPrompt.round,
    };
  }

  const transcript = normalizeQuestionAnswerTranscript(
    message?.messageData?.questionAnswerTranscript ?? metadata?.questionAnswerTranscript
  );
  for (let i = transcript.length - 1; i >= 0; i -= 1) {
    const entry = transcript[i];
    if (!entry.question || entry.answer) continue;
    return {
      question: entry.question,
      choices: [],
      round: entry.round,
    };
  }
  return null;
}

// Map WebAppConversation to ConversationSummary
export function mapWebAppConversationToSummary(item: WebAppConversation): ConversationSummary {
  return {
    id: item.id,
    conversationId: item.id,
    title: item.name,
    dialogueCount: item.dialogue_count,
    updatedAt: item.updated_at * 1000,
    status: item.status,
    metadata: {
      workflow_version_uuid: item.workflow_version_uuid,
      invoke_from: item.invoke_from,
      created_at: item.created_at,
    },
  };
}

// Map WebAppConversationMessageItem to Message
export function mapWebAppMessageToMessage(item: WebAppConversationMessageItem): Message {
  const runStatus = normalizeMessageRunStatus(item.status);
  const questionAnswerTranscript = getQuestionAnswerTranscriptFromMetadata(item.message_metadata);

  return {
    messageId: item.id,
    query: item.query,
    answer: item.answer,
    parentId: '',
    model: null,
    clientState: {
      phase: runStatus === 'running' ? 'streaming' : 'completed',
      status: runStatus && runStatus !== 'running' ? runStatus : undefined,
      finishedAt: item.created_at * 1000,
    },
    WorkflowRunInfo:
      item.workflow_run_id && runStatus
        ? {
            id: item.workflow_run_id,
            status: runStatus,
            runNodeInfo: [],
          }
        : undefined,
    messageData: {
      ...(item.workflow_run_id ? { tempKey: `restore:${item.workflow_run_id}` } : {}),
      workflow_run_id: item.workflow_run_id,
      message_id: item.id,
      created_at: item.created_at,
      status: item.status,
      inputs: item.inputs,
      ...(item.message_metadata ? { metadata: item.message_metadata } : {}),
      ...(questionAnswerTranscript.length > 0 ? { questionAnswerTranscript } : {}),
    },
  };
}

export function normalizeFinalRunStatus(status: unknown): TerminalRunStatus {
  const normalized = normalizeMessageRunStatus(status);
  if (normalized === 'completed' || normalized === 'stopped' || normalized === 'expired') {
    return normalized;
  }
  return 'error';
}

// Map WebAppConversationDetail to ConversationDetail
export function mapWebAppConversationDetailToDetail(data: WebAppConversationDetail): ConversationDetail {
  const latestPendingQuestion = [...data.messages]
    .reverse()
    .map(getPendingQuestionAnswerPromptFromMessage)
    .find(Boolean);

  return {
    summary: {
      id: data.id,
      conversationId: data.id,
      title: data.name,
      dialogueCount: data.dialogue_count,
      updatedAt: data.updated_at * 1000,
      status: data.status,
      metadata: {
        agent_id: data.agent_id,
        mode: data.mode,
        workflow_version_uuid: data.workflow_version_uuid,
        invoke_from: data.invoke_from,
        created_at: data.created_at,
        inputs: data.inputs,
        ...(latestPendingQuestion ? { questionAnswerPrompt: latestPendingQuestion } : {}),
      },
    },
    messages: data.messages.map(mapWebAppMessageToMessage),
    loaded: true,
    loading: false,
  };
}
