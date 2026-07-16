import type { AIChatMessage } from '@/services/types/aichat';
import type { AIChatControllerState } from '@/components/chat/controllers/aichat';
import { generateClientId } from '@/utils/client-id';

export function getErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }

  if (error && typeof error === 'object') {
    const record = error as Record<string, unknown>;
    const response = record.response;
    if (response && typeof response === 'object') {
      const responseData = (response as Record<string, unknown>).data;
      if (responseData && typeof responseData === 'object') {
        const data = responseData as Record<string, unknown>;
        const message = data.message ?? data.error ?? data.errorMessage;
        if (typeof message === 'string' && message.trim()) {
          return message;
        }
      }
    }

    for (const key of ['message', 'error', 'errorMessage', 'details'] as const) {
      const value = record[key];
      if (typeof value === 'string' && value.trim()) {
        return value;
      }
    }
  }

  return String(error || 'Unknown error');
}

export function isAbortError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  return error.name === 'AbortError' || error.message.toLowerCase().includes('abort');
}

export function isRecoverableStreamTransportError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return true;
  const status = (error as { status?: unknown }).status;
  return typeof status !== 'number' || !Number.isFinite(status);
}

export function isContinuationLikelyStartedError(error: unknown): boolean {
  const message = getErrorMessage(error).toLowerCase();
  return (
    message.includes('continuation is already running') ||
    message.includes('continuation has already resolved') ||
    message.includes('conversation is already streaming') ||
    message.includes('invalid current leaf message status')
  );
}

export const AICHAT_RECOVERY_RETRY_DELAYS = [800, 1600, 3200] as const;
export const AICHAT_STREAM_EVENTS_EXPIRED = 'stream events expired';

export function createClientDraftId(prefix: string): string {
  return generateClientId(prefix);
}

export function removeRunningStreamingStateByConversation(
  streamingByMessageId: AIChatControllerState['streamingByMessageId'],
  conversationId: string
): AIChatControllerState['streamingByMessageId'] {
  const nextStreamingByMessageId = { ...streamingByMessageId };
  Object.values(streamingByMessageId).forEach(streaming => {
    if (streaming.conversation_id !== conversationId) return;
    if (streaming.status === 'streaming' || !streaming.timeline?.length) {
      delete nextStreamingByMessageId[streaming.message_id];
    }
  });
  return nextStreamingByMessageId;
}

export function clearStreamingRuntimeMessageMetadata(message: AIChatMessage): AIChatMessage {
  if (!message.metadata) {
    return message;
  }

  const metadata = { ...message.metadata };
  delete metadata.has_trace;
  delete metadata.skill_invocations;
  delete metadata.selected_skill_ids;
  delete metadata.loaded_skill_ids;
  delete metadata.skill_step_count;
  delete metadata.skill_call_count;
  delete metadata.skill_names;
  delete metadata.tool_call_count;
  delete metadata.tool_names;
  delete metadata.generated_file_count;
  delete metadata.generated_files;
  delete metadata.user_input_request;

  return {
    ...message,
    metadata: Object.keys(metadata).length > 0 ? metadata : undefined,
  };
}
