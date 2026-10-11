import type { AIChatMessage, AIChatRuntimeInput } from '@/services/types/aichat';

const RUNTIME_INPUT_QUEUE_STATUSES = new Set<AIChatRuntimeInput['status']>([
  'pending',
  'queued_follow_up',
]);

const RUNTIME_INPUT_TERMINAL_STATUSES = new Set<AIChatRuntimeInput['status']>([
  'applied_current_run',
  'applied_follow_up',
  'canceled',
  'failed',
]);

export function selectRuntimeInputQueue(
  messages: AIChatMessage[],
  activeMessageId?: string | null
): AIChatRuntimeInput[] {
  const byClientInputID = new Map<string, AIChatRuntimeInput>();

  messages.forEach(message => {
    (message.runtime_inputs ?? []).forEach(input => {
      if (!RUNTIME_INPUT_QUEUE_STATUSES.has(input.status)) return;
      if (activeMessageId && input.target_message_id !== activeMessageId) return;

      const existing = byClientInputID.get(input.client_input_id);
      if (!existing || (existing.optimistic && !input.optimistic)) {
        byClientInputID.set(input.client_input_id, input);
      }
    });
  });

  return Array.from(byClientInputID.values()).sort((left, right) => {
    if (left.sequence !== right.sequence) return left.sequence - right.sequence;
    if (left.created_at !== right.created_at) return left.created_at - right.created_at;
    return left.client_input_id.localeCompare(right.client_input_id);
  });
}

export function applyRuntimeInputApplication(
  messages: AIChatMessage[],
  input: Pick<AIChatRuntimeInput, 'id' | 'status' | 'consumed_message_id' | 'target_message_id'> & {
    client_input_id?: string;
  }
): AIChatMessage[] {
  let changed = false;
  const next = messages.map(message => {
    const runtimeInputs = message.runtime_inputs ?? [];
    const matchesInput = (candidate: AIChatRuntimeInput) =>
      candidate.id === input.id ||
      Boolean(input.client_input_id && candidate.client_input_id === input.client_input_id);
    if (!runtimeInputs.some(matchesInput)) return message;
    changed = true;
    return {
      ...message,
      runtime_inputs: runtimeInputs.map(candidate =>
        matchesInput(candidate)
          ? {
              ...candidate,
              id: input.id || candidate.id,
              status: input.status,
              consumed_message_id: input.consumed_message_id ?? candidate.consumed_message_id,
              target_message_id: input.target_message_id || candidate.target_message_id,
              optimistic: false,
            }
          : candidate
      ),
    };
  });
  return changed ? next : messages;
}

export function reconcileAIChatRuntimeInput(
  messages: AIChatMessage[],
  clientInputId: string,
  nextInput: AIChatRuntimeInput | null
): AIChatMessage[] {
  const existingInput = messages
    .flatMap(message => message.runtime_inputs ?? [])
    .find(input => input.client_input_id === clientInputId);
  const withoutInput = messages.map(message => removeRuntimeInput(message, clientInputId));
  if (!nextInput) return withoutInput;
  const resolvedInput =
    existingInput &&
    RUNTIME_INPUT_TERMINAL_STATUSES.has(existingInput.status) &&
    !RUNTIME_INPUT_TERMINAL_STATUSES.has(nextInput.status)
      ? {
          ...nextInput,
          status: existingInput.status,
          consumed_message_id: existingInput.consumed_message_id ?? nextInput.consumed_message_id,
          optimistic: false,
        }
      : nextInput;
  return attachRuntimeInput(withoutInput, resolvedInput);
}

export function applyAuthoritativeRuntimeInputSnapshot(
  messages: AIChatMessage[],
  incomingMessages: AIChatMessage[]
): AIChatMessage[] {
  const snapshots = incomingMessages.filter(message => Array.isArray(message.runtime_inputs));
  if (snapshots.length === 0) return messages;

  const snapshotMessageIds = new Set(snapshots.map(message => message.id));
  const authoritativeInputs = new Map<string, AIChatRuntimeInput>();
  snapshots.forEach(message => {
    (message.runtime_inputs ?? []).forEach(input => {
      authoritativeInputs.set(input.client_input_id, input);
    });
  });

  let reconciled = messages.map(message => {
    if (snapshotMessageIds.has(message.id)) {
      return message.runtime_inputs?.length ? { ...message, runtime_inputs: [] } : message;
    }
    const runtimeInputs = (message.runtime_inputs ?? []).filter(
      input => !authoritativeInputs.has(input.client_input_id)
    );
    return runtimeInputs.length === (message.runtime_inputs ?? []).length
      ? message
      : { ...message, runtime_inputs: runtimeInputs };
  });

  Array.from(authoritativeInputs.values())
    .sort((left, right) => left.sequence - right.sequence)
    .forEach(input => {
      reconciled = attachRuntimeInput(reconciled, input);
    });
  return reconciled;
}

function removeRuntimeInput(message: AIChatMessage, clientInputId: string): AIChatMessage {
  const runtimeInputs = message.runtime_inputs ?? [];
  if (!runtimeInputs.some(input => input.client_input_id === clientInputId)) return message;
  return {
    ...message,
    runtime_inputs: runtimeInputs.filter(input => input.client_input_id !== clientInputId),
  };
}

function attachRuntimeInput(messages: AIChatMessage[], input: AIChatRuntimeInput): AIChatMessage[] {
  return messages.map(message => {
    if (message.id !== input.target_message_id) return message;
    return {
      ...message,
      runtime_inputs: [...(message.runtime_inputs ?? []), input].sort(
        (left, right) => left.sequence - right.sequence
      ),
    };
  });
}
