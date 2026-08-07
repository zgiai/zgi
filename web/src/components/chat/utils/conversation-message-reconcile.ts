import type { Message, RunStatus } from '@/components/chat/types';

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

function mergeMessages(persisted: Message, local: Message): Message {
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
  const reconciled = persistedMessages.map(persisted => {
    const localIndex = localMessages.findIndex(
      (local, index) => !matchedLocalIndexes.has(index) && messagesShareIdentity(persisted, local)
    );
    if (localIndex < 0) return persisted;

    matchedLocalIndexes.add(localIndex);
    return mergeMessages(persisted, localMessages[localIndex]);
  });

  localMessages.forEach((local, index) => {
    if (!matchedLocalIndexes.has(index)) reconciled.push(local);
  });
  return reconciled;
}
