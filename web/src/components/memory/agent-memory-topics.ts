import type { ScopedTranslations } from '@/i18n/translations';
import { formatDate } from '@/utils/format';
import { knownMemoryTopics, memoryTopicTitle, type KnownMemoryTopic } from './memory-topics';

// The memory API shape shared by Agent memory surfaces (WebApp and the
// builder's own debug memory).
export interface AgentMemoryValueLike {
  key: string;
  name?: string;
  content: string;
  revision: number;
  source_kind?: string;
  last_operation_id?: string;
  undoable_until?: number | null;
  updated_at?: number;
}

type Translate = ScopedTranslations<'common.memoryItems'>;

const memorySources = ['legacy', 'explicit', 'automatic', 'manager'] as const;
type MemorySource = (typeof memorySources)[number];

/** A localized "updated … · source" line for a memory value. */
export function agentMemoryMeta(value: AgentMemoryValueLike, t: Translate): string | undefined {
  if (!value.updated_at) return undefined;
  const kind = (memorySources as readonly string[]).includes(value.source_kind ?? '')
    ? (value.source_kind as MemorySource)
    : 'manager';
  const source = t(`sources.${kind}`);
  return t('meta', { time: formatDate(value.updated_at), source });
}

export function agentMemoryTitle(value: { key: string; name?: string }, t: Translate): string {
  return memoryTopicTitle(value.key, value.name, topic => t(`topics.${topic}`));
}

/** Whether the value's last automatic change can still be undone. */
export function agentMemoryUndoable(value: AgentMemoryValueLike): boolean {
  return (
    value.source_kind === 'automatic' &&
    Boolean(value.last_operation_id) &&
    Boolean(value.undoable_until && value.undoable_until > Date.now() / 1000)
  );
}

/**
 * Where a new fact may go: the user's existing topics first, then common
 * topics they do not have yet.
 */
export function agentMemoryAddTargets(values: AgentMemoryValueLike[], t: Translate) {
  const existing = values.filter(value => value.content.trim());
  const keys = new Set(existing.map(value => value.key));
  const suggested: KnownMemoryTopic[] = ['profile', 'preferences', 'work', 'notes'];
  return [
    ...existing.map(value => ({ value: value.key, label: agentMemoryTitle(value, t) })),
    ...suggested
      .filter(topic => !keys.has(topic) && (knownMemoryTopics as readonly string[]).includes(topic))
      .map(topic => ({ value: topic, label: t(`topics.${topic}`) })),
  ];
}
