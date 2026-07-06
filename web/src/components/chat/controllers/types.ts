import type {
  Message,
  ChatAttachment,
  ModelInfo,
  NodeInfo,
  TerminalRunStatus,
  GeneratedImage,
} from '@/components/chat/types';

// Chat mode types
export type ChatMode = 'singleTest' | 'singleChat' | 'groupTest' | 'groupChat';

// Conversation summary (list item, lightweight)
export interface ConversationSummary {
  id: string; // Frontend UUID
  conversationId: string; // Backend id (empty for drafts)
  title: string;
  dialogueCount?: number;
  updatedAt?: number;
  status?: string;
  metadata?: Record<string, unknown>;
}

export type ConversationSearchResultType = 'conversation' | 'message';

export interface ConversationSearchResult {
  type: ConversationSearchResultType;
  conversationId: string;
  conversationTitle: string;
  messageId?: string;
  snippet: string;
  updatedAt?: number;
}

export type ConversationSearchFn = (
  query: string,
  limit: number
) => Promise<ConversationSearchResult[]>;

// Conversation detail (messages loaded)
export interface ConversationDetail {
  summary: ConversationSummary;
  messages: Message[];
  loaded: boolean;
  loading: boolean;
}

// Pagination info
export interface Pagination {
  page: number;
  limit: number;
  total: number;
  hasMore: boolean;
}

// Send message payload
export interface SendMessagePayload {
  query: string;
  conversationId: string; // May be empty for new drafts
  historyWindowSize?: number;
  files?: ChatAttachment[];
  inputs?: Record<string, unknown>;
}

// Chat run callbacks for streaming
export interface ChatRunCallbacks {
  onStarted: (ctx: {
    conversationId: string;
    messageId?: string;
    workflowRunId?: string;
    tempKey?: string;
  }) => void;
  onToken: (token: string) => void;
  onTextReplace?: () => void;
  onMessage: (meta?: Record<string, unknown>) => void;
  onMessageEnd?: (meta?: Record<string, unknown>) => void;
  mergeMessageData?: (data: Record<string, unknown>) => void;
  onNodeStarted?: (node: NodeInfo) => void;
  onNodeFinished?: (node: NodeInfo) => void;
  onPaused?: (meta?: {
    elapsedTime?: number;
    workflowRunId?: string;
    nodeIds?: string[];
    status?: 'pending_approval' | 'pending_question';
    nodeType?: string;
  }) => void;
  onFinished: (meta: {
    status: TerminalRunStatus;
    error?: string;
    elapsedTime?: number;
    messageId?: string;
    workflowRunId?: string;
    model?: ModelInfo | null;
    generatedImages?: GeneratedImage[];
  }) => void;
  onError: (err: Error) => void;
}

// Conversation transport interface (adapter to backend)
export interface ConversationTransport {
  // List conversations with pagination
  list(params: {
    page: number;
    limit: number;
  }): Promise<{ items: ConversationSummary[]; pagination: Pagination }>;

  // Get a single conversation detail with messages
  get(conversationId: string): Promise<ConversationDetail>;

  // Create a new conversation (client-only, returns draft with empty conversationId)
  create(payload?: { title?: string }): Promise<ConversationSummary>;

  // Remove a conversation (may be no-op for some backends)
  remove(conversationId: string): Promise<void>;

  // Send a message via streaming
  send(payload: SendMessagePayload, callbacks: ChatRunCallbacks, abortSignal?: AbortSignal): void;

  // Optional: rename conversation
  rename?(conversationId: string, title: string): Promise<void>;

  // Optional: search conversations and messages in the transport's own scope
  search?(query: string, limit: number): Promise<ConversationSearchResult[]>;
}

// Chat controller state (read-only for component)
export interface ChatControllerState {
  mode: ChatMode;
  conversations: ConversationSummary[];
  pagination: Pagination;
  activeId: string | null;
  activeDetail: ConversationDetail | null;
  isLoadingList: boolean;
  isLoadingDetail: boolean;
  isSending: boolean;
  isPaused: boolean;
}

// Chat controller actions
export interface ChatControllerActions {
  /**
   * Initialize controller
   * @param convId - Optional initial conversation ID to select
   */
  init(convId?: string): void;

  // Refresh conversation list
  refreshList(params?: { page?: number; limit?: number; append?: boolean }): void;

  // Select a conversation by id
  select(id: string): void;

  // Optional: load a conversation by backend id and select it, even if it is not in the current list
  loadAndSelect?(conversationId: string): Promise<void>;

  // Create a new draft conversation
  createDraft(title?: string): ConversationSummary;

  // Remove a conversation
  remove(id: string): Promise<void>;

  // Send a message (uses activeId if conversationId not provided)
  send(payload: Omit<SendMessagePayload, 'conversationId'> & { conversationId?: string }): void;

  // Adopt server conversation id after first send
  adoptServerConversationId(clientId: string, serverConversationId: string): void;

  // Optional: rename conversation
  rename?(id: string, title: string): Promise<void>;

  // Optional: search conversations and messages in the active controller scope
  search?(query: string, limit: number): Promise<ConversationSearchResult[]>;
}

import type { StoreApi } from 'zustand';

// Combined controller interface
export interface ChatController extends ChatControllerState, ChatControllerActions {
  store: StoreApi<ChatControllerState>;
}
