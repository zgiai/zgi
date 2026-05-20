import type {
  AIChatConversation,
  AIChatMessage,
  AIChatMessageMetadata,
  AIChatMessageStatus,
} from '@/services/types/aichat';

export function isDraftAIChatConversationId(id: string | null | undefined): boolean {
  return Boolean(id?.startsWith('draft-aichat-'));
}

export function normalizeAIChatStatus(status: unknown): AIChatMessageStatus {
  if (typeof status !== 'string') return 'error';
  switch (status.trim().toLowerCase()) {
    case 'pending':
      return 'pending';
    case 'streaming':
      return 'streaming';
    case 'completed':
      return 'completed';
    case 'stopped':
      return 'stopped';
    case 'error':
    default:
      return 'error';
  }
}

export function createDraftAIChatConversation(id: string, title: string): AIChatConversation {
  const now = Math.floor(Date.now() / 1000);
  return {
    id,
    organization_id: '',
    account_id: '',
    title,
    status: 'normal',
    runtime_status: 'idle',
    dialogue_count: 0,
    source: 'console',
    created_at: now,
    updated_at: now,
  };
}

export function createStreamingAIChatMessage(payload: {
  id: string;
  conversationId: string;
  parentId?: string | null;
  query: string;
  modelName: string;
  modelProvider?: string;
  createdAt?: number;
  metadata?: AIChatMessageMetadata;
}): AIChatMessage {
  const now = Math.floor(Date.now() / 1000);
  const createdAt = payload.createdAt ?? now;

  return {
    id: payload.id,
    conversation_id: payload.conversationId,
    parent_id: payload.parentId || undefined,
    query: payload.query,
    answer: '',
    status: 'streaming',
    model_provider: payload.modelProvider,
    model_name: payload.modelName,
    metadata: payload.metadata,
    created_at: createdAt,
    updated_at: createdAt,
  };
}

export function upsertAIChatMessage(
  messages: AIChatMessage[],
  incoming: AIChatMessage
): AIChatMessage[] {
  const index = messages.findIndex(message => message.id === incoming.id);
  if (index < 0) {
    return [...messages, incoming].sort((a, b) => a.created_at - b.created_at);
  }

  const next = messages.slice();
  next[index] = incoming;
  return next.sort((a, b) => a.created_at - b.created_at);
}

export function replaceAIChatConversation(
  conversations: AIChatConversation[],
  incoming: AIChatConversation,
  options: { moveToTop?: boolean } = {}
): AIChatConversation[] {
  const index = conversations.findIndex(item => item.id === incoming.id);
  if (index < 0) return [incoming, ...conversations];

  const next = conversations.map(item => (item.id === incoming.id ? incoming : item));
  if (!options.moveToTop || index === 0) return next;

  return [incoming, ...next.filter(item => item.id !== incoming.id)];
}
