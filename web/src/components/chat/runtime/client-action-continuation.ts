export interface ClientActionCompletionGate<T> {
  continuationReady: boolean;
  deferredCompletion?: T;
}

export interface ClientActionContinuationSnapshot {
  messageStatus?: string;
  streamingStatus?: string;
  isSending: boolean;
  conversationRuntimeStatus?: string;
  activeMessageMatches: boolean;
}

export function takeClientActionCompletion<T>(
  gate: ClientActionCompletionGate<T>,
  payload: T
): T | null {
  if (!gate.continuationReady) {
    gate.deferredCompletion = payload;
    return null;
  }

  gate.deferredCompletion = undefined;
  return payload;
}

export function openClientActionCompletionGate<T>(
  gate: ClientActionCompletionGate<T>
): T | null {
  gate.continuationReady = true;
  const deferred = gate.deferredCompletion ?? null;
  gate.deferredCompletion = undefined;
  return deferred;
}

export function canStartClientActionContinuation(
  snapshot: ClientActionContinuationSnapshot
): boolean {
  const waitingForClientAction =
    snapshot.messageStatus === 'waiting_client_action' ||
    snapshot.streamingStatus === 'waiting_client_action';
  if (!waitingForClientAction) return false;

  const executionStillOwnsMessage =
    (snapshot.isSending || snapshot.conversationRuntimeStatus === 'streaming') &&
    (snapshot.streamingStatus === 'streaming' || snapshot.activeMessageMatches);
  return !executionStillOwnsMessage;
}
