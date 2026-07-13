import type {
  AIChatConversation,
  AIChatAgentProgressEventData,
  AIChatErrorEventData,
  AIChatIntermediateAnswerEventData,
  AIChatUserInputRequestedEventData,
  AIChatFileParseEndEventData,
  AIChatFileParseErrorEventData,
  AIChatFileParseStartEventData,
  AIChatMessageFile,
  AIChatMessage,
  AIChatMessageChunkEventData,
  AIChatMessageEndEventData,
  AIChatMessageRetractEventData,
  AIChatMessageStartEventData,
  AIChatMemoryMutationEventData,
  AIChatClientActionResultRequest,
  AIChatSkillInvocation,
  AIChatRuntimeSurface,
  AIChatToolGovernanceDecisionEventData,
  AIChatToolGovernanceDecisionRequest,
  AIChatUserInputContinuationRequest,
  AIChatWorkflowPausedEventData,
} from '@/services/types/aichat';
import type { ChatBranchNavigation } from '@/components/chat/utils/message-tree';
import type { ConversationSearchResult } from '@/components/chat/controllers/types';
import type { NodeInfo, RunStatus } from '@/components/chat/types';
import type { StoreApi } from 'zustand/vanilla';

export interface AIChatModelSelection {
  provider?: string;
  model: string;
  parameters?: Record<string, number | string | boolean | string[]>;
}

export interface AIChatPagination {
  page: number;
  limit: number;
  total: number;
  hasMore: boolean;
}

export interface AIChatStreamingMessageState {
  conversation_id: string;
  message_id: string;
  answer: string;
  status:
    | 'streaming'
    | 'completed'
    | 'waiting_approval'
    | 'waiting_client_action'
    | 'waiting_question'
    | 'stopped'
    | 'error';
  timeline?: AIChatAgenticTimelineItem[];
  last_event_id?: string;
  replay_base_answer?: string;
  replay_offset?: number;
  replace?: boolean;
  sensitiveOutputBlocked?: boolean;
}

export type AIChatAgenticTimelineItem =
  | {
      id: string;
      type: 'progress_text';
      content: string;
      phase?: AIChatAgentProgressEventData['phase'];
      transient?: boolean;
      meta_tool_name?: string;
      skill_id?: string;
      tool_name?: string;
      action_id?: string;
      action_type?: string;
      continuation_policy?: string;
      blocking?: boolean;
      status?: string;
      effect?: string;
      asset_type?: string;
      assets?: AIChatAgentProgressEventData['assets'];
      correlation_id?: string;
      asset_operation_audit?: AIChatAgentProgressEventData['asset_operation_audit'];
      result?: AIChatAgentProgressEventData['result'];
      arguments_chars?: number;
      created_at?: number;
      event_id?: string | null;
    }
  | {
      id: string;
      type: 'skill_event';
      invocation: AIChatSkillInvocation;
      created_at?: number;
      event_id?: string | null;
    }
  | {
      id: string;
      type: 'intermediate_answer';
      answer_id?: string;
      title?: string;
      content: string;
      status?: 'streaming' | 'success';
      created_at?: number;
      event_id?: string | null;
    }
  | {
      id: string;
      type: 'user_input_request';
      request_id?: string;
      message?: string;
      questions: Array<{
        question_id?: string;
        question: string;
        options: string[];
      }>;
      created_at?: number;
      event_id?: string | null;
    }
  | {
      id: string;
      type: 'user_input_response';
      request_id?: string;
      message?: string;
      answers: Array<{
        question_id?: string;
        question: string;
        value: string;
      }>;
      created_at?: number;
      event_id?: string | null;
    }
  | {
      id: string;
      type: 'memory_event';
      event: AIChatMemoryMutationEventData;
      created_at?: number;
      event_id?: string | null;
    }
  | {
      id: string;
      type: 'tool_governance_decision';
      event: AIChatToolGovernanceDecisionEventData;
      created_at?: number;
      event_id?: string | null;
    }
  | {
      id: string;
      type: 'workflow_run';
      workflowRunId: string;
      status: RunStatus;
      elapsedTime?: number;
      error?: string;
      nodes: NodeInfo[];
      approval?: Partial<AIChatWorkflowPausedEventData>;
      created_at?: number;
      event_id?: string | null;
    };

export type AIChatRecoveryMode = 'active' | 'background';

export interface AIChatMessageStartContext {
  query?: string;
  model?: AIChatModelSelection;
  files?: AIChatMessageFile[];
  previousConversationId?: string | null;
  resetAnswer?: boolean;
  forceAdvanceLeaf?: boolean;
  mode?: AIChatRecoveryMode;
  moveToTop?: boolean;
}

export interface AIChatControllerState {
  conversations: AIChatConversation[];
  pagination: AIChatPagination;
  activeConversationId: string | null;
  messagesByConversation: Record<string, AIChatMessage[]>;
  messagePaginationByConversation: Record<string, AIChatPagination>;
  loadingOlderByConversation: Record<string, boolean>;
  streamingByMessageId: Record<string, AIChatStreamingMessageState>;
  recoveringByConversation: Record<string, boolean>;
  stoppingByConversation: Record<string, boolean>;
  isLoadingList: boolean;
  isLoadingMessages: boolean;
  isSending: boolean;
  error: string | null;
}

export interface AIChatControllerStore extends AIChatControllerState {
  update: (updater: (current: AIChatControllerState) => AIChatControllerState) => void;
  replaceState: (nextState: AIChatControllerState) => void;
  applyMessageStart: (
    payload: AIChatMessageStartEventData,
    context?: AIChatMessageStartContext,
    eventId?: string | null
  ) => void;
  applyMessageChunk: (payload: AIChatMessageChunkEventData, eventId?: string | null) => void;
  applyMessageRetract: (payload: AIChatMessageRetractEventData, eventId?: string | null) => void;
  applyAgentProgress: (payload: AIChatAgentProgressEventData, eventId?: string | null) => void;
  applyIntermediateAnswer: (
    payload: AIChatIntermediateAnswerEventData,
    eventId?: string | null
  ) => void;
  applyUserInputRequested: (
    payload: AIChatUserInputRequestedEventData,
    eventId?: string | null
  ) => void;
  applyFileParseStart: (payload: AIChatFileParseStartEventData, eventId?: string | null) => void;
  applyFileParseEnd: (payload: AIChatFileParseEndEventData, eventId?: string | null) => void;
  applyFileParseError: (payload: AIChatFileParseErrorEventData, eventId?: string | null) => void;
  applyMessageEnd: (payload: AIChatMessageEndEventData, eventId?: string | null) => void;
  applyStreamError: (payload: AIChatErrorEventData, fallbackConversationId: string | null) => void;
  mergeMessages: (conversationId: string, messages: AIChatMessage[]) => void;
  setActiveConversationId: (conversationId: string | null) => void;
  setConversationRunningState: (
    conversationId: string,
    running: boolean,
    activeMessageId?: string
  ) => void;
}

export interface AIChatController {
  store: StoreApi<AIChatControllerStore>;
  conversations: AIChatConversation[];
  pagination: AIChatPagination;
  activeConversationId: string | null;
  activeConversation: AIChatConversation | null;
  messages: AIChatMessage[];
  streamingByMessageId: Record<string, AIChatStreamingMessageState>;
  displayMessageIds: string[];
  displayMessages: AIChatMessage[];
  branchNavigationByMessageId: Map<string, ChatBranchNavigation>;
  activeMessagePagination: AIChatPagination;
  isLoadingList: boolean;
  isLoadingMessages: boolean;
  isLoadingOlderMessages: boolean;
  isRecoveringMessages: boolean;
  isStopping: boolean;
  isSending: boolean;
  error: string | null;
  init: (conversationId?: string | null) => void;
  refreshList: (params?: { page?: number; append?: boolean }) => Promise<void>;
  select: (conversationId: string) => Promise<void>;
  startNew: () => void;
  remove: (conversationId: string) => Promise<void>;
  rename: (conversationId: string, title: string) => Promise<void>;
  loadOlderMessages: (conversationId?: string) => Promise<void>;
  recoverStreamingConversation: (
    conversationId: string,
    options?: { conversation?: AIChatConversation; mode?: AIChatRecoveryMode }
  ) => Promise<void>;
  send: (payload: {
    query: string;
    model: AIChatModelSelection;
    files?: AIChatMessageFile[];
    parentId?: string | null;
    useMemory?: boolean;
    forceAdvanceLeaf?: boolean;
    runtimeSurface?: AIChatRuntimeSurface;
    operationContext?: unknown;
  }) => Promise<void>;
  stop: () => Promise<void>;
  regenerate: (
    messageId: string,
    model: AIChatModelSelection,
    options?: { operationContext?: unknown; runtimeSurface?: AIChatRuntimeSurface }
  ) => Promise<void>;
  replaceRootMessage: (payload: {
    messageId: string;
    query?: string;
    model?: AIChatModelSelection;
    runtimeSurface?: AIChatRuntimeSurface;
    operationContext?: unknown;
  }) => Promise<void>;
  continueWorkflowApproval?: (
    conversationId: string,
    messageId: string,
    payload?: { approvalToken: string; inputs: Record<string, unknown>; action: string }
  ) => Promise<void>;
  continueWorkflowQuestion?: (
    conversationId: string,
    messageId: string,
    inputs: { query: string; question_answer_option_id?: string }
  ) => Promise<void>;
  continueToolGovernanceDecision?: (
    conversationId: string,
    messageId: string,
    correlationId: string,
    payload: AIChatToolGovernanceDecisionRequest
  ) => Promise<void>;
  continueClientAction?: (
    conversationId: string,
    messageId: string,
    actionId: string,
    payload: AIChatClientActionResultRequest
  ) => Promise<void>;
  continueUserInput?: (
    conversationId: string,
    messageId: string,
    requestId: string,
    payload: AIChatUserInputContinuationRequest
  ) => Promise<void>;
  switchBranch: (messageId: string) => void;
  search?: (
    query: string,
    limit: number,
    options?: { surface?: AIChatRuntimeSurface }
  ) => Promise<ConversationSearchResult[]>;
}

export type AIChatSetControllerState = (
  updater: (current: AIChatControllerState) => AIChatControllerState
) => void;

export const DEFAULT_AICHAT_PAGINATION: AIChatPagination = {
  page: 1,
  limit: 20,
  total: 0,
  hasMore: false,
};

export const DEFAULT_AICHAT_MESSAGE_PAGINATION: AIChatPagination = {
  page: 1,
  limit: 100,
  total: 0,
  hasMore: false,
};
