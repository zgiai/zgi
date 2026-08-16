const DEFAULT_CONVERSATION_TIMESTAMP_PATTERN =
  /^(?:Conversation|\u4f1a\u8bdd) \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/;

export function conversationTitleNeedsRefresh(
  title?: string,
  metadata?: Record<string, unknown>
): boolean {
  const generationStatus = metadata?.title_generation_status;
  if (generationStatus === 'pending' || generationStatus === 'failed') return true;

  const normalized = (title ?? '').trim();
  if (!normalized) return true;
  if (normalized === 'New Conversation' || normalized === '\u65b0\u5efa\u4f1a\u8bdd') return true;
  if (normalized.startsWith('New conversation ')) return true;
  return DEFAULT_CONVERSATION_TIMESTAMP_PATTERN.test(normalized);
}
