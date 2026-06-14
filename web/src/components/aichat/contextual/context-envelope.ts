import type {
  AIChatRuntimeTransport,
  AIChatStreamCallbacks,
} from '@/components/chat/transports/aichat-transport';
import { aichatTransport } from '@/components/chat/transports/aichat-transport';
import type { AIChatChatRequest, AIChatRegenerateMessageRequest } from '@/services/types/aichat';
import type { AIChatContextItem } from './types';

const MAX_CONTEXT_ITEMS = 8;
const MAX_METADATA_KEYS = 8;
const MAX_FIELD_LENGTH = 260;

function compactText(value: string | undefined, limit = MAX_FIELD_LENGTH): string {
  const text = (value ?? '').replace(/\s+/g, ' ').trim();
  if (text.length <= limit) return text;
  return `${text.slice(0, limit).trim()}...`;
}

function formatMetadata(item: AIChatContextItem): string {
  const entries = Object.entries(item.metadata ?? {})
    .filter(([, value]) => value !== undefined && value !== null && `${value}`.trim() !== '')
    .slice(0, MAX_METADATA_KEYS);
  if (entries.length === 0) return '';
  return entries.map(([key, value]) => `${key}=${compactText(`${value}`, 120)}`).join(', ');
}

export function buildAIChatContextEnvelope(items: AIChatContextItem[]): string {
  const visibleItems = items.slice(0, MAX_CONTEXT_ITEMS);
  if (visibleItems.length === 0) return '';

  const lines = [
    'Current ZGI page context. Use it only to interpret this turn; do not save it as memory unless the user explicitly asks.',
    'Important: AIChat account memory and Agent memory are separate. Do not claim they are shared.',
  ];

  visibleItems.forEach((item, index) => {
    const details = [
      `type=${item.type}`,
      `id=${item.id}`,
      `title=${compactText(item.title)}`,
      item.subtitle ? `subtitle=${compactText(item.subtitle)}` : '',
      item.description ? `description=${compactText(item.description)}` : '',
      item.href ? `href=${item.href}` : '',
      item.permissions?.length ? `permissions=${item.permissions.join(',')}` : '',
      item.risk ? `risk=${item.risk}` : '',
      formatMetadata(item),
    ].filter(Boolean);
    lines.push(`${index + 1}. ${details.join('; ')}`);
  });

  if (items.length > visibleItems.length) {
    lines.push(`Additional context items omitted: ${items.length - visibleItems.length}.`);
  }

  return lines.join('\n');
}

export function createContextualAIChatTransport(
  getContextItems: () => AIChatContextItem[]
): AIChatRuntimeTransport {
  const base = aichatTransport;
  return {
    listConversations: base.listConversations.bind(base),
    getConversation: base.getConversation.bind(base),
    listMessages: base.listMessages.bind(base),
    refreshConversation: base.refreshConversation.bind(base),
    updateConversation: base.updateConversation.bind(base),
    removeConversation: base.removeConversation.bind(base),
    stopConversation: base.stopConversation.bind(base),
    streamChat(
      payload: AIChatChatRequest,
      callbacks: AIChatStreamCallbacks,
      abortSignal?: AbortSignal
    ) {
      const envelope = buildAIChatContextEnvelope(getContextItems());
      return aichatTransport.streamChat(
        {
          ...payload,
          runtime_context: envelope || undefined,
        },
        callbacks,
        abortSignal
      );
    },
    regenerateMessage(
      messageId: string,
      payload: AIChatRegenerateMessageRequest,
      callbacks: AIChatStreamCallbacks,
      abortSignal?: AbortSignal
    ) {
      return aichatTransport.regenerateMessage(messageId, payload, callbacks, abortSignal);
    },
    recoverConversationStream: base.recoverConversationStream.bind(base),
  };
}
