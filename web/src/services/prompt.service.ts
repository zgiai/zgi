import { http, type SseMessage } from '@/lib/http';
import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  AdoptPromptOptimizationRunRequest,
  CreatePromptRequest,
  PromptDetail,
  PromptPlaygroundMessage,
  PromptPlaygroundRequest,
  PromptList,
  PromptListParams,
  PromptOptimizeRequest,
  PromptOptimizeResult,
  PromptOptimizationRunList,
  PromptOptimizationRunListParams,
  PromptUsageSummary,
  PromptVersionPayload,
  SetPromptLabelsRequest,
  UpdatePromptRequest,
} from './types/prompt';

export interface PromptOptimizeStreamCallbacks {
  onProgress?: (payload: { step?: string; index?: number }) => void;
  onMeta?: (payload: {
    goal?: string;
    preserve_variables?: boolean;
    detected_variables?: string[];
    provider?: string;
    model?: string;
  }) => void;
  onChunk?: (payload: { text?: string }) => void;
  onDone?: (payload: {
    goal?: string;
    preserve_variables?: boolean;
    detected_variables?: string[];
    run_id?: string;
    output?: string;
    provider?: string;
    model?: string;
  }) => void;
  onError?: (error: Error) => void;
  onClose?: () => void;
}

export interface PromptPlaygroundStreamCallbacks {
  onProgress?: (payload: { step?: string; index?: number }) => void;
  onMeta?: (payload: {
    provider?: string;
    model?: string;
    detected_variables?: string[];
    rendered_prompt?: string;
    rendered_messages?: PromptPlaygroundMessage[];
  }) => void;
  onChunk?: (payload: { text?: string }) => void;
  onDone?: (payload: {
    output?: string;
    provider?: string;
    model?: string;
    detected_variables?: string[];
    rendered_prompt?: string;
    rendered_messages?: PromptPlaygroundMessage[];
  }) => void;
  onError?: (error: Error) => void;
  onClose?: () => void;
}

function safeJsonParse(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch {
    return {};
  }
}

function dispatchPromptOptimizerStreamMessage(
  message: SseMessage<Record<string, unknown> | string>,
  callbacks: PromptOptimizeStreamCallbacks
): void {
  const envelope =
    typeof message.data === 'string'
      ? (safeJsonParse(message.data) as Record<string, unknown>)
      : message.data;

  if (!envelope || typeof envelope !== 'object') {
    return;
  }

  const record = envelope as Record<string, unknown>;
  const event = typeof record.event === 'string' ? record.event : message.event || '';
  const data = record.data && typeof record.data === 'object' ? record.data : {};

  switch (event) {
    case 'progress':
      callbacks.onProgress?.(data as { step?: string; index?: number });
      return;
    case 'meta':
      callbacks.onMeta?.(
        data as {
          goal?: string;
          preserve_variables?: boolean;
          detected_variables?: string[];
          provider?: string;
          model?: string;
        }
      );
      return;
    case 'chunk':
      callbacks.onChunk?.(data as { text?: string });
      return;
    case 'done':
      callbacks.onDone?.(
        data as {
          goal?: string;
          preserve_variables?: boolean;
          detected_variables?: string[];
          run_id?: string;
          output?: string;
          provider?: string;
          model?: string;
        }
      );
      return;
    case 'error':
      callbacks.onError?.(
        new Error(
          typeof (data as { message?: unknown }).message === 'string'
            ? (data as { message: string }).message
            : 'Prompt optimization failed'
        )
      );
      return;
    default:
      return;
  }
}

function isPromptOptimizeTerminalMessage(message: SseMessage<unknown>): boolean {
  const envelope =
    typeof message.data === 'string'
      ? (safeJsonParse(message.data) as Record<string, unknown>)
      : message.data;
  const event =
    envelope && typeof envelope === 'object' && typeof (envelope as Record<string, unknown>).event === 'string'
      ? ((envelope as Record<string, unknown>).event as string)
      : message.event || '';
  return event === 'done' || event === 'error';
}

function dispatchPromptPlaygroundStreamMessage(
  message: SseMessage<Record<string, unknown> | string>,
  callbacks: PromptPlaygroundStreamCallbacks
): void {
  const envelope =
    typeof message.data === 'string'
      ? (safeJsonParse(message.data) as Record<string, unknown>)
      : message.data;

  if (!envelope || typeof envelope !== 'object') {
    return;
  }

  const record = envelope as Record<string, unknown>;
  const event = typeof record.event === 'string' ? record.event : message.event || '';
  const data = record.data && typeof record.data === 'object' ? record.data : {};

  switch (event) {
    case 'progress':
      callbacks.onProgress?.(data as { step?: string; index?: number });
      return;
    case 'meta':
      callbacks.onMeta?.(
        data as {
          provider?: string;
          model?: string;
          detected_variables?: string[];
          rendered_prompt?: string;
          rendered_messages?: PromptPlaygroundMessage[];
        }
      );
      return;
    case 'chunk':
      callbacks.onChunk?.(data as { text?: string });
      return;
    case 'done':
      callbacks.onDone?.(
        data as {
          output?: string;
          provider?: string;
          model?: string;
          detected_variables?: string[];
          rendered_prompt?: string;
          rendered_messages?: PromptPlaygroundMessage[];
        }
      );
      return;
    case 'error':
      callbacks.onError?.(
        new Error(
          typeof (data as { message?: unknown }).message === 'string'
            ? (data as { message: string }).message
            : 'Prompt playground failed'
        )
      );
      return;
    default:
      return;
  }
}

class PromptService extends BaseService {
  constructor() {
    super({
      basePath: '/console/api',
      endpoint: 'main',
    });
  }

  listPrompts(params?: PromptListParams): Promise<ApiResponseData<PromptList>> {
    return this.request('get', '/prompts', undefined, {
      params,
      headers: { 'Content-Type': 'application/json' },
    });
  }

  getPrompt(promptId: string): Promise<ApiResponseData<PromptDetail>> {
    return this.request('get', `/prompts/${promptId}`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  getPromptUsage(promptId: string): Promise<ApiResponseData<PromptUsageSummary>> {
    return this.request('get', `/prompts/${promptId}/usage`, undefined, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  createPrompt(data: CreatePromptRequest): Promise<ApiResponseData<PromptDetail>> {
    return this.request('post', '/prompts', data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  updatePrompt(promptId: string, data: UpdatePromptRequest): Promise<ApiResponseData<PromptDetail>> {
    return this.request('patch', `/prompts/${promptId}`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  createPromptVersion(
    promptId: string,
    data: PromptVersionPayload
  ): Promise<ApiResponseData<PromptDetail>> {
    return this.request('post', `/prompts/${promptId}/versions`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  setPromptLabels(
    promptId: string,
    data: SetPromptLabelsRequest
  ): Promise<ApiResponseData<PromptDetail>> {
    return this.request('post', `/prompts/${promptId}/labels`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  optimizePrompt(data: PromptOptimizeRequest): Promise<ApiResponseData<PromptOptimizeResult>> {
    return this.request('post', '/prompts/optimize', data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }

  streamOptimizePrompt(
    data: PromptOptimizeRequest,
    callbacks: PromptOptimizeStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return http.sse<Record<string, unknown>, PromptOptimizeRequest>('/console/api/prompts/optimize/stream', {
      method: 'POST',
      body: data,
      abortSignal,
      isTerminalMessage: isPromptOptimizeTerminalMessage,
      onMessage: message => dispatchPromptOptimizerStreamMessage(message, callbacks),
      onError: error => callbacks.onError?.(error),
      onClose: callbacks.onClose,
    });
  }

  streamPlaygroundPrompt(
    data: PromptPlaygroundRequest,
    callbacks: PromptPlaygroundStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return http.sse<Record<string, unknown>, PromptPlaygroundRequest>('/console/api/prompts/playground/stream', {
      method: 'POST',
      body: data,
      abortSignal,
      isTerminalMessage: isPromptOptimizeTerminalMessage,
      onMessage: message => dispatchPromptPlaygroundStreamMessage(message, callbacks),
      onError: error => callbacks.onError?.(error),
      onClose: callbacks.onClose,
    });
  }

  listOptimizationRuns(
    promptId: string,
    params?: PromptOptimizationRunListParams
  ): Promise<ApiResponseData<PromptOptimizationRunList>> {
    return this.request('get', `/prompts/${promptId}/optimization-runs`, undefined, { params });
  }

  adoptOptimizationRun(
    promptId: string,
    runId: string,
    data: AdoptPromptOptimizationRunRequest
  ): Promise<ApiResponseData<PromptDetail>> {
    return this.request('post', `/prompts/${promptId}/optimization-runs/${runId}/adopt`, data, {
      headers: { 'Content-Type': 'application/json' },
    });
  }
}

export const promptService = new PromptService();
export default promptService;
