import { AgentRuntimeTransport } from '@/components/chat/transports/agent-runtime-transport';
import type {
  AIChatRuntimeTransport,
  AIChatStreamCallbacks,
} from '@/components/chat/transports/aichat-transport';
import { digitalHumanService } from '@/services/digital-human.service';
import type { AIChatChatRequest, AIChatRegenerateMessageRequest } from '@/services/types/aichat';
import type { DigitalHumanRun } from '@/services/types/digital-human';

const RUN_LINK_POLL_INTERVAL_MS = 400;
const RUN_LINK_TIMEOUT_MS = 120_000;

const activeRunStatuses = new Set(['queued', 'running', 'canceling']);

/**
 * Adapts the durable DigitalHuman invocation path to the shared ChatRuntime UI.
 * Conversation history and event recovery come from ChatRuntime; new turns still
 * enter through the DigitalHuman run/outbox boundary.
 */
export class DigitalHumanRuntimeTransport
  extends AgentRuntimeTransport
  implements AIChatRuntimeTransport
{
  constructor(private readonly digitalHumanId: string) {
    super({
      runtimeBasePath: `/console/api/digital-humans/${digitalHumanId}/runtime`,
      chatPath: `/console/api/digital-humans/${digitalHumanId}/invocations`,
      supportsRuntimeInput: true,
    });
  }

  override async streamChat(
    payload: AIChatChatRequest,
    callbacks: AIChatStreamCallbacks,
    abortSignal?: AbortSignal
  ): Promise<{ close: () => void }> {
    let handedOffToEventStream = false;
    let submittedRun: DigitalHumanRun | null = null;
    try {
      const response = await digitalHumanService.submitInvocation(this.digitalHumanId, {
        request_id: crypto.randomUUID(),
        query: payload.query,
        conversation_id: payload.conversation_id,
      });
      submittedRun = response.data.run;
      if (abortSignal?.aborted) throw abortError();
      const run = await this.waitForMessageLink(submittedRun, abortSignal);
      if (!run.conversation_id || !run.message_id) {
        throw new Error('The digital human run did not create a conversation message.');
      }

      callbacks.onOpen?.({
        conversation_id: run.conversation_id,
        message_id: run.message_id,
      });

      handedOffToEventStream = true;
      return await super.recoverConversationStream(
        run.conversation_id,
        { messageId: run.message_id },
        callbacks,
        abortSignal
      );
    } catch (error) {
      if (abortSignal?.aborted && submittedRun && activeRunStatuses.has(submittedRun.status)) {
        try {
          await digitalHumanService.cancelRun(this.digitalHumanId, submittedRun.id);
        } catch {
          // The durable run may already have completed or been canceled by the
          // conversation stop endpoint. Either state is safe to reconcile later.
        }
      }
      if (!abortSignal?.aborted) {
        callbacks.onRequestError(toError(error));
      }
      if (!handedOffToEventStream) callbacks.onClose();
      throw error;
    }
  }

  override async regenerateMessage(
    _messageId: string,
    _payload: AIChatRegenerateMessageRequest,
    callbacks: AIChatStreamCallbacks
  ): Promise<{ close: () => void }> {
    const error = new Error('Digital human message replay is not available yet.');
    callbacks.onRequestError(error);
    callbacks.onClose();
    throw error;
  }

  private async waitForMessageLink(
    initialRun: DigitalHumanRun,
    abortSignal?: AbortSignal
  ): Promise<DigitalHumanRun> {
    const startedAt = Date.now();
    let run = initialRun;
    while (!run.conversation_id || !run.message_id) {
      if (abortSignal?.aborted) throw abortError();
      if (!activeRunStatuses.has(run.status)) {
        throw new Error(run.error_message || 'The digital human run ended before chat started.');
      }
      if (Date.now() - startedAt >= RUN_LINK_TIMEOUT_MS) {
        throw new Error('Timed out while waiting for the digital human conversation to start.');
      }
      await waitForRunPoll(abortSignal);
      const response = await digitalHumanService.getRun(this.digitalHumanId, run.id);
      run = response.data;
    }
    return run;
  }
}

export function createDigitalHumanRuntimeTransport(
  digitalHumanId: string
): DigitalHumanRuntimeTransport {
  return new DigitalHumanRuntimeTransport(digitalHumanId);
}

function waitForRunPoll(abortSignal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (abortSignal?.aborted) {
      reject(abortError());
      return;
    }
    const timeout = window.setTimeout(() => {
      abortSignal?.removeEventListener('abort', cancel);
      resolve();
    }, RUN_LINK_POLL_INTERVAL_MS);
    const cancel = () => {
      window.clearTimeout(timeout);
      reject(abortError());
    };
    abortSignal?.addEventListener('abort', cancel, { once: true });
  });
}

function abortError(): Error {
  return new DOMException('The digital human request was stopped.', 'AbortError');
}

function toError(error: unknown): Error {
  return error instanceof Error ? error : new Error('The digital human request failed.');
}
