import { http, type SseMessage } from '@/lib/http';
import type { ApiResponseData } from '@/services/types/common';
import type {
  ActionCapabilityResponse,
  ActionPlanRequest,
  ActionRunResponse,
  AIChatActionRuntimeEventName,
  AIChatActionRuntimeEventPayload,
  AIChatActionRuntimeSseEnvelope,
  ConfirmActionRequest,
  ExecuteActionRequest,
} from '@/types/aichat-action-runtime';

const ACTION_RUNTIME_BASE_PATH = '/console/api/aichat';

export interface AIChatActionRuntimeStreamCallbacks {
  onEvent: (
    event: AIChatActionRuntimeEventName,
    data: AIChatActionRuntimeEventPayload,
    eventId?: string | null
  ) => void;
  onError?: (error: Error) => void;
  onClose?: () => void;
}

function safeJsonParse(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch {
    return { message: value };
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function normalizeActionRuntimeSseMessage(
  message: SseMessage<AIChatActionRuntimeSseEnvelope | string>
): { event: AIChatActionRuntimeEventName; data: AIChatActionRuntimeEventPayload } | null {
  const envelope = typeof message.data === 'string' ? safeJsonParse(message.data) : message.data;
  if (!isRecord(envelope)) {
    return message.event
      ? { event: message.event as AIChatActionRuntimeEventName, data: { message: String(envelope) } }
      : null;
  }

  const event =
    (typeof envelope.event === 'string' && envelope.event) ||
    (message.event as AIChatActionRuntimeEventName | null) ||
    '';
  if (!event) return null;

  const data = isRecord(envelope.data) ? envelope.data : envelope;
  return {
    event: event as AIChatActionRuntimeEventName,
    data: data as AIChatActionRuntimeEventPayload,
  };
}

function isActionRuntimeTerminalMessage(message: SseMessage<unknown>): boolean {
  const normalized = normalizeActionRuntimeSseMessage(
    message as SseMessage<AIChatActionRuntimeSseEnvelope | string>
  );
  return normalized?.event === 'action_run_end' || normalized?.event === 'error';
}

function encodePathSegment(value: string): string {
  return encodeURIComponent(value);
}

export const aichatActionRuntimeClient = {
  listCapabilities() {
    return http.get<ApiResponseData<ActionCapabilityResponse[]>>(
      `${ACTION_RUNTIME_BASE_PATH}/action-capabilities`
    );
  },

  createActionPlan(payload: ActionPlanRequest) {
    return http.post<ApiResponseData<ActionRunResponse>>(
      `${ACTION_RUNTIME_BASE_PATH}/action-plans`,
      payload
    );
  },

  getActionRun(actionId: string) {
    return http.get<ApiResponseData<ActionRunResponse>>(
      `${ACTION_RUNTIME_BASE_PATH}/actions/${encodePathSegment(actionId)}`
    );
  },

  confirmAction(actionId: string, payload: ConfirmActionRequest) {
    return http.post<ApiResponseData<ActionRunResponse>>(
      `${ACTION_RUNTIME_BASE_PATH}/actions/${encodePathSegment(actionId)}/confirm`,
      payload
    );
  },

  executeAction(actionId: string, payload: ExecuteActionRequest = {}) {
    return http.post<ApiResponseData<ActionRunResponse>>(
      `${ACTION_RUNTIME_BASE_PATH}/actions/${encodePathSegment(actionId)}/execute`,
      payload
    );
  },

  streamActionEvents(
    actionId: string,
    callbacks: AIChatActionRuntimeStreamCallbacks,
    abortSignal?: AbortSignal
  ) {
    return http.sse<AIChatActionRuntimeSseEnvelope | string, never>(
      `${ACTION_RUNTIME_BASE_PATH}/actions/${encodePathSegment(actionId)}/events`,
      {
        method: 'GET',
        abortSignal,
        isTerminalMessage: isActionRuntimeTerminalMessage,
        onMessage: message => {
          const normalized = normalizeActionRuntimeSseMessage(message);
          if (!normalized) return;
          callbacks.onEvent(normalized.event, normalized.data, message.id);
        },
        onError: error => callbacks.onError?.(error),
        onClose: callbacks.onClose,
      }
    );
  },
};

export type {
  ActionCapabilityResponse,
  ActionPlanRequest,
  ActionRunResponse,
  ConfirmActionRequest,
  ExecuteActionRequest,
};
