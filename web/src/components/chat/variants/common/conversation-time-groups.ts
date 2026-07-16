export type ConversationTimeGroupKey =
  | 'today'
  | 'yesterday'
  | 'previous7Days'
  | 'previous30Days'
  | 'older'
  | 'undated';

export interface ConversationTimeGroup<T> {
  key: ConversationTimeGroupKey;
  items: T[];
}

const conversationTimeGroupOrder: ConversationTimeGroupKey[] = [
  'today',
  'yesterday',
  'previous7Days',
  'previous30Days',
  'older',
  'undated',
];

function startOfLocalDay(timestamp: number): number {
  const date = new Date(timestamp);
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
}

function normalizeTimestamp(updatedAt?: number): number | null {
  if (!updatedAt || !Number.isFinite(updatedAt)) return null;
  return updatedAt < 1_000_000_000_000 ? updatedAt * 1000 : updatedAt;
}

export function getConversationTimeGroupKey(
  updatedAt?: number,
  now = Date.now()
): ConversationTimeGroupKey {
  const timestamp = normalizeTimestamp(updatedAt);
  if (timestamp === null) return 'undated';

  const todayStart = startOfLocalDay(now);
  const itemDayStart = startOfLocalDay(timestamp);
  const dayDiff = Math.floor((todayStart - itemDayStart) / 86_400_000);

  if (dayDiff <= 0) return 'today';
  if (dayDiff === 1) return 'yesterday';
  if (dayDiff <= 7) return 'previous7Days';
  if (dayDiff <= 30) return 'previous30Days';
  return 'older';
}

export function groupByConversationTime<T>(
  items: T[],
  getUpdatedAt: (item: T) => number | undefined
): Array<ConversationTimeGroup<T>> {
  const grouped = new Map<ConversationTimeGroupKey, T[]>();

  items.forEach(item => {
    const key = getConversationTimeGroupKey(getUpdatedAt(item));
    const groupItems = grouped.get(key) ?? [];
    groupItems.push(item);
    grouped.set(key, groupItems);
  });

  return conversationTimeGroupOrder
    .map(key => ({ key, items: grouped.get(key) ?? [] }))
    .filter(group => group.items.length > 0);
}
