// Server-Sent Events (SSE) client implementation

import { ErrorNotificationService } from '@/utils/error-notifications';
import { captureException } from '@/lib/sentry/client';
import { getEndpointConfig, type ApiEndpoint } from './config';
import type { SseOptions, SseMessage, SsePostOptions, SseEventCallbacks } from './types';
import { parseSseRawEvent, SseParser, type SseRawEvent } from './sse-parser';

interface SseRequestError extends Error {
  code?: string;
  status?: number;
  details?: unknown;
}

export const SSE_IDLE_TIMEOUT_MS = 45_000;

function withTerminalStatus(
  payload: Record<string, unknown>,
  status: string
): Record<string, unknown> {
  const nested = payload.data;
  if (nested && typeof nested === 'object') {
    return {
      ...payload,
      data: {
        status,
        ...(nested as Record<string, unknown>),
      },
    };
  }

  return { status, ...payload };
}

/**
 * SSE client using Fetch + ReadableStream.
 * Provides auth header, baseURL handling, and typed callbacks.
 */
export class SseClient {
  constructor(
    private endpoint: ApiEndpoint,
    private ensureValidToken: () => Promise<string | null>
  ) {}

  /**
   * Opens an SSE connection with full spec compliance.
   */
  async sse<TOut = unknown, TBody = unknown>(
    path: string,
    options: SseOptions<TBody, TOut>
  ): Promise<{ close: () => void }> {
    const controller = new AbortController();
    const signal = this.bridgeAbortSignal(options.abortSignal, controller);
    const endpointCfg = options.endpoint ? getEndpointConfig(options.endpoint) : this.endpoint;

    // Build URL with query params
    const urlObj = this.buildUrl(path, endpointCfg.baseURL, options.query);

    const method = options.method || 'GET';

    // Prepare auth header
    let authHeader: Record<string, string>;
    try {
      authHeader = await this.buildAuthHeader(options.skipAuth);
    } catch (error) {
      return this.handleAuthError(error, options, urlObj, method, controller, endpointCfg);
    }

    // Prepare headers
    const headers = this.buildHeaders(authHeader, options.headers);

    // Prepare fetch init
    const init = this.buildFetchInit(method, headers, signal, options.body);

    // Execute fetch
    let response: Response;
    try {
      response = await fetch(urlObj.toString(), init);
    } catch (error) {
      return this.handleFetchError(error, options, urlObj, method, controller, endpointCfg);
    }

    // Validate response
    if (!response.ok) {
      return await this.handleResponseError(
        response,
        options,
        urlObj,
        method,
        controller,
        endpointCfg
      );
    }

    if (!this.isEventStream(response)) {
      return this.handleInvalidContentType(
        response,
        options,
        urlObj,
        method,
        controller,
        endpointCfg
      );
    }

    options.onOpen?.();

    // Start stream reading
    return this.startStreamReading(response, options, urlObj, method, controller, endpointCfg);
  }

  /**
   * SSE POST with unified event callbacks for workflow/node events.
   */
  async ssePost<TBody = unknown>(
    path: string,
    options: SsePostOptions<TBody>
  ): Promise<{ close: () => void }> {
    const { callbacks, ...rest } = options;

    return this.sse<unknown, TBody>(path, {
      ...rest,
      method: 'POST',
      onMessage: msg => this.dispatchSseEvent(msg.data, msg.event, callbacks),
      onError: err => {
        rest.onError?.(err);
        const requestError = err as SseRequestError;
        callbacks.onError?.({
          code: requestError.code,
          status: requestError.status,
          details: requestError.details,
          message: requestError.message,
          originalError: err,
        });
      },
    });
  }

  // ---- Private helpers ----

  private bridgeAbortSignal(
    external: AbortSignal | undefined,
    controller: AbortController
  ): AbortSignal {
    if (!external) return controller.signal;
    if (external.aborted) controller.abort();
    external.addEventListener('abort', () => controller.abort());
    return controller.signal;
  }

  private async buildAuthHeader(skipAuth?: boolean): Promise<Record<string, string>> {
    if (skipAuth) return {};
    const token = await this.ensureValidToken();
    if (!token) {
      const err = new Error('Authentication session is not available');
      (err as SseRequestError).code = 'ERR_AUTH_SESSION_MISSING';
      throw err;
    }
    return { Authorization: `Bearer ${token}` };
  }

  private buildUrl(
    path: string,
    baseURL: string,
    query?: Record<string, string | number | boolean | null | undefined>
  ): URL {
    let urlObj: URL;

    // Check if path is already an absolute URL
    if (path.startsWith('http://') || path.startsWith('https://')) {
      urlObj = new URL(path);
    } else {
      // It's a relative path, join with baseURL while preserving baseURL's own path
      const cleanBaseURL = baseURL.endsWith('/') ? baseURL.slice(0, -1) : baseURL;
      const cleanPath = path.startsWith('/') ? path : `/${path}`;
      urlObj = new URL(`${cleanBaseURL}${cleanPath}`);
    }
    if (query) {
      Object.entries(query).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          urlObj.searchParams.set(key, String(value));
        }
      });
    }
    return urlObj;
  }

  private buildHeaders(
    authHeader: Record<string, string>,
    customHeaders?: Record<string, string>
  ): Record<string, string> {
    return {
      Accept: 'text/event-stream',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive',
      ...authHeader,
      ...(customHeaders || {}),
    };
  }

  private buildFetchInit<TBody>(
    method: string,
    headers: Record<string, string>,
    signal: AbortSignal,
    body?: TBody
  ): RequestInit {
    const init: RequestInit = {
      method,
      headers,
      signal,
      credentials: 'omit',
    };

    if (method === 'POST' && body !== undefined) {
      const isJson = !headers['Content-Type'] || headers['Content-Type'].includes('json');
      init.body = isJson ? JSON.stringify(body) : (body as unknown as BodyInit);
      if (isJson && !headers['Content-Type']) {
        headers['Content-Type'] = 'application/json';
      }
    }

    return init;
  }

  private handleFetchError<TOut>(
    error: unknown,
    options: SseOptions<unknown, TOut>,
    urlObj: URL,
    method: string,
    controller: AbortController,
    endpointCfg: ApiEndpoint
  ): { close: () => void } {
    const err = error instanceof Error ? error : new Error('Network error when opening SSE');
    if (!options.skipErrorHandling) {
      ErrorNotificationService.showNetworkError();
    }
    captureException(err, scope => {
      scope.setContext('http', { url: urlObj.toString(), method });
      scope.setTag('endpoint', endpointCfg.name);
    });
    options.onError?.(err);
    return { close: () => controller.abort() };
  }

  private handleAuthError<TOut>(
    error: unknown,
    options: SseOptions<unknown, TOut>,
    urlObj: URL,
    method: string,
    controller: AbortController,
    endpointCfg: ApiEndpoint
  ): { close: () => void } {
    const err: SseRequestError =
      error instanceof Error ? error : new Error('Authentication session is not available');
    err.code = err.code || 'ERR_AUTH_SESSION_MISSING';
    err.status = err.status || 401;
    captureException(err, scope => {
      scope.setContext('http', {
        url: urlObj.toString(),
        method,
        status: err.status,
        code: err.code,
      });
      scope.setTag('endpoint', endpointCfg.name);
    });
    options.onError?.(err);
    return { close: () => controller.abort() };
  }

  private async handleResponseError<TOut>(
    response: Response,
    options: SseOptions<unknown, TOut>,
    urlObj: URL,
    method: string,
    controller: AbortController,
    endpointCfg: ApiEndpoint
  ): Promise<{ close: () => void }> {
    const responseText = await response.text().catch(() => '');
    let code: string | undefined;
    let message: string | undefined;

    if (responseText) {
      try {
        const parsed = JSON.parse(responseText) as {
          code?: unknown;
          message?: unknown;
          error?: unknown;
          details?: unknown;
        };
        code = typeof parsed.code === 'string' ? parsed.code : undefined;
        if (typeof parsed.message === 'string') message = parsed.message;
        else if (typeof parsed.error === 'string') message = parsed.error;
      } catch {
        message = responseText;
      }
    }

    const err: SseRequestError = new Error(
      message || `SSE connection failed with status ${response.status}`
    );
    err.status = response.status;
    err.code = code;
    err.details = responseText || undefined;
    captureException(err, scope => {
      scope.setContext('http', {
        url: urlObj.toString(),
        method,
        status: response.status,
        code,
        responseText,
      });
      scope.setTag('endpoint', endpointCfg.name);
    });
    options.onError?.(err);
    return { close: () => controller.abort() };
  }

  private isEventStream(response: Response): boolean {
    const contentType = response.headers.get('content-type') || '';
    return contentType.includes('text/event-stream');
  }

  private handleInvalidContentType<TOut>(
    response: Response,
    options: SseOptions<unknown, TOut>,
    urlObj: URL,
    method: string,
    controller: AbortController,
    endpointCfg: ApiEndpoint
  ): { close: () => void } {
    const err: SseRequestError = new Error(
      'Invalid SSE response: missing text/event-stream content-type'
    );
    err.status = response.status;
    captureException(err, scope => {
      scope.setContext('http', { url: urlObj.toString(), method, status: response.status });
      scope.setTag('endpoint', endpointCfg.name);
    });
    options.onError?.(err);
    return { close: () => controller.abort() };
  }

  private startStreamReading<TOut>(
    response: Response,
    options: SseOptions<unknown, TOut>,
    urlObj: URL,
    method: string,
    controller: AbortController,
    endpointCfg: ApiEndpoint
  ): { close: () => void } {
    const reader = response.body?.getReader() || null;
    if (!reader) {
      const err = new Error('SSE response has no readable body');
      if (!options.skipErrorHandling) {
        ErrorNotificationService.showNetworkError();
      }
      options.onError?.(err);
      return { close: () => controller.abort() };
    }

    const decoder = new TextDecoder('utf-8');
    const parser = new SseParser();
    let terminalEventReceived = false;
    let incompleteLastEvent = false;
    let decoderFlushed = false;
    const idleTimeoutMs =
      typeof options.idleTimeoutMs === 'number' &&
      Number.isFinite(options.idleTimeoutMs) &&
      options.idleTimeoutMs > 0
        ? options.idleTimeoutMs
        : null;

    const readWithIdleWatchdog = (): Promise<ReadableStreamReadResult<Uint8Array>> => {
      if (idleTimeoutMs === null) return reader.read();

      return new Promise((resolve, reject) => {
        const timer = window.setTimeout(() => {
          const error = new Error('SSE stream received no bytes before the idle timeout');
          (error as SseRequestError).code = 'ERR_SSE_IDLE_TIMEOUT';
          reject(error);
        }, idleTimeoutMs);
        reader.read().then(
          result => {
            window.clearTimeout(timer);
            resolve(result);
          },
          error => {
            window.clearTimeout(timer);
            reject(error);
          }
        );
      });
    };

    const isTerminalMessage = (msg: SseMessage<TOut>): boolean => {
      if (options.isTerminalMessage?.(msg as SseMessage<unknown>)) {
        return true;
      }
      return isTerminalSseEvent(msg.event, msg.data);
    };

    const dispatchRawEvent = (rawEvent: SseRawEvent): void => {
      const msg = parseSseRawEvent<TOut>(rawEvent);
      if (msg.incompleteJson) {
        incompleteLastEvent = true;
        return;
      }
      incompleteLastEvent = false;
      if (isTerminalMessage(msg)) {
        terminalEventReceived = true;
      }
      options.onMessage(msg);
    };

    const dispatchRawEvents = (rawEvents: SseRawEvent[]): void => {
      rawEvents.forEach(dispatchRawEvent);
    };

    const flushPendingEvents = (): void => {
      if (!decoderFlushed) {
        decoderFlushed = true;
        dispatchRawEvents(parser.push(decoder.decode()));
      }
      const result = parser.finish();
      dispatchRawEvents(result.events);
    };

    // Fire-and-forget reading loop
    (async () => {
      try {
        for (;;) {
          const { value, done } = await readWithIdleWatchdog();
          if (done) {
            flushPendingEvents();
            if (!terminalEventReceived && !controller.signal.aborted) {
              this.handleStreamTransportError(
                new Error(
                  incompleteLastEvent
                    ? 'SSE stream ended with an incomplete JSON event'
                    : 'SSE stream ended before a terminal event'
                ),
                options,
                urlObj,
                method,
                endpointCfg,
                {
                  terminalReceived: terminalEventReceived,
                  incompleteLastEvent,
                }
              );
            }
            break;
          }
          dispatchRawEvents(parser.push(decoder.decode(value, { stream: true })));
        }
      } catch (error) {
        flushPendingEvents();
        if (!controller.signal.aborted && !terminalEventReceived) {
          const err = error instanceof Error ? error : new Error('SSE stream error');
          this.handleStreamTransportError(err, options, urlObj, method, endpointCfg, {
            terminalReceived: terminalEventReceived,
            incompleteLastEvent,
          });
          controller.abort();
        }
      } finally {
        try {
          reader.releaseLock();
        } catch {
          // noop
        }
        options.onClose?.();
      }
    })();

    return { close: () => controller.abort() };
  }

  private handleStreamTransportError<TOut>(
    err: Error,
    options: SseOptions<unknown, TOut>,
    urlObj: URL,
    method: string,
    endpointCfg: ApiEndpoint,
    meta: { terminalReceived: boolean; incompleteLastEvent: boolean }
  ): void {
    if (!options.skipErrorHandling) {
      ErrorNotificationService.showNetworkError();
    }
    captureException(err, scope => {
      scope.setContext('http', {
        url: urlObj.toString(),
        method,
        incompleteLastEvent: meta.incompleteLastEvent,
        terminalReceived: meta.terminalReceived,
      });
      scope.setTag('endpoint', endpointCfg.name);
    });
    options.onTransportError?.(err, meta);
    options.onError?.(err);
  }

  private dispatchSseEvent(
    envelope: unknown,
    fallbackEvent: string | null | undefined,
    callbacks: SseEventCallbacks
  ): void {
    const obj = typeof envelope === 'string' ? safeParseJson(envelope) : envelope;
    const dataObj = obj && typeof obj === 'object' ? (obj as Record<string, unknown>) : {};
    const dataEvent = (dataObj as { event?: unknown }).event;
    const evt = (typeof dataEvent === 'string' && dataEvent) || fallbackEvent || '';

    const terminalStatusByEvent: Record<string, string | undefined> = {
      workflow_stopped: 'stopped',
      workflow_failed: 'failed',
      workflow_succeeded: 'succeeded',
      workflow_completed: 'succeeded',
    };
    const terminalStatus = terminalStatusByEvent[evt];
    const payload = terminalStatus ? withTerminalStatus(dataObj, terminalStatus) : dataObj;

    const eventHandlers: Record<string, ((p: unknown) => void) | undefined> = {
      workflow_started: callbacks.onWorkflowStarted,
      workflow_paused: callbacks.onWorkflowPaused,
      approval_requested: callbacks.onApprovalRequested,
      approval_result_filled: callbacks.onApprovalResultFilled,
      approval_expired: callbacks.onApprovalExpired,
      question_answer_requested: callbacks.onQuestionAnswerRequested,
      question_answer_submitted: callbacks.onQuestionAnswerSubmitted,
      workflow_finished: callbacks.onWorkflowFinished,
      workflow_stopped: callbacks.onWorkflowFinished,
      workflow_failed: callbacks.onWorkflowFinished,
      workflow_succeeded: callbacks.onWorkflowFinished,
      workflow_completed: callbacks.onWorkflowFinished,
      error: callbacks.onError,
      node_started: callbacks.onNodeStarted,
      node_finished: callbacks.onNodeFinished,
      node_error: callbacks.onNodeError,
      node_retry: callbacks.onNodeRetry,
      agent_log: callbacks.onAgentLog,
      text_chunk: callbacks.onTextChunk,
      text_replace: callbacks.onTextReplace,
      message: callbacks.onMessage,
      data: callbacks.onMessage,
      message_end: callbacks.onMessageEnd,
      iteration_started: callbacks.onIterationStarted,
      iteration_next: callbacks.onIterationNext,
      iteration_completed: callbacks.onIterationCompleted,
      loop_started: callbacks.onLoopStarted,
      loop_next: callbacks.onLoopNext,
      loop_completed: callbacks.onLoopCompleted,
    };

    eventHandlers[evt]?.(payload);
  }
}

function safeParseJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return { raw: text };
  }
}

function isTerminalSseEvent(eventName: string | null, payload: unknown): boolean {
  const record = payload && typeof payload === 'object' ? (payload as Record<string, unknown>) : {};
  const payloadEvent = typeof record.event === 'string' ? record.event : '';
  const event = payloadEvent || eventName || '';
  return (
    event === 'workflow_finished' ||
    event === 'workflow_stopped' ||
    event === 'workflow_failed' ||
    event === 'workflow_succeeded' ||
    event === 'workflow_completed' ||
    event === 'workflow_paused'
  );
}
