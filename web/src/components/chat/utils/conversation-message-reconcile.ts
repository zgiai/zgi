import type { Message, RunStatus } from '@/components/chat/types';

type TerminalMessageStatus = Extract<RunStatus, 'completed' | 'error' | 'stopped' | 'expired'>;

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function messageIdentityValues(message: Message): Set<string> {
  const values = [
    message.messageId,
    message.WorkflowRunInfo?.id,
    message.messageData?.message_id,
    message.messageData?.workflow_run_id,
  ]
    .map(stringValue)
    .filter(Boolean);
  return new Set(values);
}

function messagesShareIdentity(left: Message, right: Message): boolean {
  const leftIdentities = messageIdentityValues(left);
  if (leftIdentities.size > 0) {
    for (const identity of messageIdentityValues(right)) {
      if (leftIdentities.has(identity)) return true;
    }
  }

  const leftTempKey = stringValue(left.messageData?.tempKey);
  const rightTempKey = stringValue(right.messageData?.tempKey);
  return Boolean(leftTempKey && rightTempKey && leftTempKey === rightTempKey);
}

function isLiveMessage(message: Message): boolean {
  const status = message.WorkflowRunInfo?.status ?? message.clientState?.status;
  return (
    message.clientState?.phase === 'requesting' ||
    message.clientState?.phase === 'streaming' ||
    status === 'running' ||
    status === 'pending_approval' ||
    status === 'pending_question'
  );
}

function terminalMessageStatus(message: Message): TerminalMessageStatus | null {
  const statuses = [message.WorkflowRunInfo?.status, message.clientState?.status];
  for (const status of statuses) {
    if (
      status === 'completed' ||
      status === 'error' ||
      status === 'stopped' ||
      status === 'expired'
    ) {
      return status;
    }
  }
  return null;
}

function completedGeneratedImages(
  primary: Message['generatedImages'],
  fallback: Message['generatedImages']
): Message['generatedImages'] {
  const primaryCompleted = primary?.filter(image => !image.isLoading) ?? [];
  if (primaryCompleted.length > 0) return primaryCompleted;

  const fallbackCompleted = fallback?.filter(image => !image.isLoading) ?? [];
  return fallbackCompleted.length > 0 ? fallbackCompleted : undefined;
}

function mergeTerminalMessage(
  persisted: Message,
  local: Message,
  terminalStatus: TerminalMessageStatus,
  terminalOwner: 'persisted' | 'local'
): Message {
  const persistedRun = persisted.WorkflowRunInfo;
  const localRun = local.WorkflowRunInfo;
  const localTempKey = stringValue(local.messageData?.tempKey);
  const localOwnsTerminal = terminalOwner === 'local';
  const primary = localOwnsTerminal ? local : persisted;
  const fallback = localOwnsTerminal ? persisted : local;
  const primaryRun = primary.WorkflowRunInfo;
  const fallbackRun = fallback.WorkflowRunInfo;
  const primaryNodes = primaryRun?.runNodeInfo ?? [];
  const fallbackNodes = fallbackRun?.runNodeInfo ?? [];

  return {
    ...fallback,
    ...primary,
    messageId: persisted.messageId || local.messageId,
    query: primary.query || fallback.query,
    WorkflowRunInfo:
      persistedRun || localRun
        ? {
            ...(fallbackRun ?? { id: '', status: terminalStatus, runNodeInfo: [] }),
            ...primaryRun,
            id: persistedRun?.id || localRun?.id || '',
            status: terminalStatus,
            runNodeInfo: primaryNodes.length > 0 ? primaryNodes : fallbackNodes,
          }
        : undefined,
    clientState: {
      ...(fallback.clientState ?? { phase: 'completed' }),
      ...primary.clientState,
      phase: 'completed',
      status: terminalStatus,
    },
    messageData: {
      ...fallback.messageData,
      ...primary.messageData,
      ...(localTempKey ? { tempKey: localTempKey } : {}),
    },
    generatedImages: completedGeneratedImages(primary.generatedImages, fallback.generatedImages),
  };
}

function mergeMessages(persisted: Message, local: Message): Message {
  const persistedTerminalStatus = terminalMessageStatus(persisted);
  if (persistedTerminalStatus) {
    return mergeTerminalMessage(persisted, local, persistedTerminalStatus, 'persisted');
  }

  const localTerminalStatus = terminalMessageStatus(local);
  if (localTerminalStatus) {
    return mergeTerminalMessage(persisted, local, localTerminalStatus, 'local');
  }

  if (!isLiveMessage(local)) {
    return {
      ...local,
      ...persisted,
      messageData: { ...local.messageData, ...persisted.messageData },
    };
  }

  const persistedRun = persisted.WorkflowRunInfo;
  const localRun = local.WorkflowRunInfo;
  return {
    ...persisted,
    ...local,
    messageId: persisted.messageId || local.messageId,
    query: local.query || persisted.query,
    WorkflowRunInfo:
      localRun || persistedRun
        ? {
            ...(persistedRun ?? { id: '', status: 'running' as RunStatus, runNodeInfo: [] }),
            ...localRun,
            id: persistedRun?.id || localRun?.id || '',
            runNodeInfo: localRun?.runNodeInfo ?? persistedRun?.runNodeInfo ?? [],
          }
        : undefined,
    messageData: { ...persisted.messageData, ...local.messageData },
  };
}

/**
 * Reconcile a freshly fetched server snapshot with messages already received by the live stream.
 * The server owns persisted history, while in-flight and paused local messages must survive until
 * their projection is visible in a later snapshot.
 */
export function reconcileConversationMessages(
  persistedMessages: Message[],
  localMessages: Message[]
): Message[] {
  if (localMessages.length === 0) return persistedMessages;
  if (persistedMessages.length === 0) return localMessages;

  const matchedLocalIndexes = new Set<number>();
  const persistedMatchIndexes = persistedMessages.map(persisted => {
    const localIndex = localMessages.findIndex(
      (local, index) => !matchedLocalIndexes.has(index) && messagesShareIdentity(persisted, local)
    );
    if (localIndex >= 0) matchedLocalIndexes.add(localIndex);
    return localIndex;
  });

  if (matchedLocalIndexes.size === 0) {
    return [...persistedMessages, ...localMessages];
  }

  const persistedByLocalIndex = new Map<number, Message>();
  const persistedBeforeLocalIndex = new Map<number, Message[]>();
  const persistedTail: Message[] = [];

  persistedMessages.forEach((persisted, persistedIndex) => {
    const localIndex = persistedMatchIndexes[persistedIndex];
    if (localIndex >= 0) {
      persistedByLocalIndex.set(localIndex, persisted);
      return;
    }

    const nextMatchedLocalIndex = persistedMatchIndexes
      .slice(persistedIndex + 1)
      .find(index => index >= 0);
    if (nextMatchedLocalIndex === undefined) {
      persistedTail.push(persisted);
      return;
    }

    const bucket = persistedBeforeLocalIndex.get(nextMatchedLocalIndex) ?? [];
    bucket.push(persisted);
    persistedBeforeLocalIndex.set(nextMatchedLocalIndex, bucket);
  });

  const reconciled: Message[] = [];
  localMessages.forEach((local, localIndex) => {
    reconciled.push(...(persistedBeforeLocalIndex.get(localIndex) ?? []));
    const persisted = persistedByLocalIndex.get(localIndex);
    reconciled.push(persisted ? mergeMessages(persisted, local) : local);
  });
  reconciled.push(...persistedTail);
  return reconciled;
}
