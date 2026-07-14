// HTTP client type definitions

import type { AxiosRequestConfig } from 'axios';
import type { ApiResponseData } from '@/services/types/common';

// Extended request config with essential options
export interface ExtendedRequestConfig extends AxiosRequestConfig {
  skipAuth?: boolean;
  skipErrorHandling?: boolean;
  endpoint?: string;
  isRetryRequest?: boolean;
  isRefreshingToken?: boolean;
  // Per-request override for retry attempts. If set to 0, disables internal retries
  retryAttemptsOverride?: number;
}

// Internal interface for retry logic
export interface RetryableConfig extends ExtendedRequestConfig {
  _retryCount?: number;
}

// Response wrapper for consistent API responses
export type ApiResponse<T = unknown> = ApiResponseData<T> & {
  status: number;
  timestamp: number;
};

// Typed SSE message
export interface SseMessage<T> {
  event: string | null;
  data: T;
  id?: string | null;
  /** Raw data chunk (before JSON parsing) */
  raw: string;
  /** Retry suggestion from server, if provided */
  retry?: number | null;
}

// SSE connection options
export interface SseOptions<TBody = unknown, TOut = unknown>
  extends Omit<ExtendedRequestConfig, 'data' | 'params' | 'onUploadProgress' | 'signal'> {
  /** HTTP method, default GET */
  method?: 'GET' | 'POST';
  /** Optional request body (for POST) */
  body?: TBody;
  /** Query string parameters */
  query?: Record<string, string | number | boolean | null | undefined>;
  /** Extra headers to send */
  headers?: Record<string, string>;
  /** Open callback, called when the stream is established */
  onOpen?: () => void;
  /** Message callback, invoked for each SSE event message */
  onMessage: (msg: SseMessage<TOut>) => void;
  /** Optional terminal detector used to suppress transport errors after a completed stream */
  isTerminalMessage?: (msg: SseMessage<unknown>) => boolean;
  /** Optional maximum time to wait for the next response byte; disabled when omitted */
  idleTimeoutMs?: number;
  /** Error callback for network/parse issues */
  onError?: (error: Error) => void;
  /** Transport-level error callback with parser state metadata */
  onTransportError?: (
    error: Error,
    meta: { terminalReceived: boolean; incompleteLastEvent: boolean }
  ) => void;
  /** External abort signal to cancel the stream */
  abortSignal?: AbortSignal;
  /** Closed callback, called when the stream is closed */
  onClose?: () => void;
}

// Unified application-level SSE event callbacks
export interface SseEventCallbacks {
  onWorkflowStarted?: (payload: unknown) => void;
  onWorkflowPaused?: (payload: unknown) => void;
  onApprovalRequested?: (payload: unknown) => void;
  onApprovalResultFilled?: (payload: unknown) => void;
  onApprovalExpired?: (payload: unknown) => void;
  onQuestionAnswerRequested?: (payload: unknown) => void;
  onQuestionAnswerSubmitted?: (payload: unknown) => void;
  onWorkflowFinished?: (payload: unknown) => void;
  onError?: (payload: unknown) => void;
  onNodeStarted?: (payload: unknown) => void;
  onNodeFinished?: (payload: unknown) => void;
  onNodeError?: (payload: unknown) => void;
  onNodeRetry?: (payload: unknown) => void;
  onAgentLog?: (payload: unknown) => void;
  onTextChunk?: (payload: unknown) => void;
  onTextReplace?: (payload: unknown) => void;
  onMessage?: (payload: unknown) => void;
  onMessageEnd?: (payload: unknown) => void;
  onIterationStarted?: (payload: unknown) => void;
  onIterationNext?: (payload: unknown) => void;
  onIterationCompleted?: (payload: unknown) => void;
  onLoopStarted?: (payload: unknown) => void;
  onLoopNext?: (payload: unknown) => void;
  onLoopCompleted?: (payload: unknown) => void;
}

// Options for ssePost wrapper
export interface SsePostOptions<TBody = unknown>
  extends Omit<SseOptions<TBody, unknown>, 'onMessage' | 'method'> {
  callbacks: SseEventCallbacks;
  onClose?: () => void;
}
