export interface ConversationHistorySummary {
  conversationId: string;
  latestRunId: string;
  latestMessageId?: string;
  status: string;
  createdAt?: number;
  finishedAt?: number | null;
  elapsedTime?: number;
  totalSteps?: number;
  totalTokens?: number;
  runCount: number;
}

export interface ConversationHistoryMessageItem {
  id: string;
  conversationId: string;
  query: string;
  answer: string;
  createdAt: number;
  workflowRunId?: string;
  invokeFrom?: string;
  parentMessageId?: string;
  status?: string;
  error?: string | null;
  isVirtual?: boolean;
}

export interface SelectedMessageRunState {
  messageId: string | null;
  runId: string | null;
}
