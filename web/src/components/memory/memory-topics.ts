// A memory topic is one memory file (an Agent) or one memory item (a
// DigitalHuman). People think of memory as a list of things the assistant
// knows about them, so a topic's content is shown and edited as facts, one
// per line.

const bulletPattern = /^\s*(?:[-*•·]|\d+[.)、])\s+/;

/** Splits a topic's content into facts: one per non-empty line, bullets removed. */
export function splitFacts(content: string): string[] {
  return content
    .split('\n')
    .map(line => line.replace(bulletPattern, '').trim())
    .filter(Boolean);
}

/** Joins facts back into a topic's content: a single fact stays plain, more become a list. */
export function joinFacts(facts: string[]): string {
  const kept = facts.map(fact => fact.trim()).filter(Boolean);
  if (kept.length <= 1) return kept[0] ?? '';
  return kept.map(fact => `- ${fact}`).join('\n');
}

/** Topics the assistant commonly keeps, with readable titles in every locale. */
export const knownMemoryTopics = [
  'profile',
  'preferences',
  'standing_instructions',
  'project_context',
  'projects',
  'work',
  'health',
  'fitness',
  'family',
  'learning',
  'travel',
  'notes',
] as const;

export type KnownMemoryTopic = (typeof knownMemoryTopics)[number];

const topicAliases: Record<string, KnownMemoryTopic> = {
  preference: 'preferences',
  project: 'projects',
  instructions: 'standing_instructions',
  note: 'notes',
};

/** Resolves a file key to a known topic, or undefined when the model named it freely. */
export function knownTopicOf(key: string): KnownMemoryTopic | undefined {
  const normalized = key.trim().toLowerCase();
  if ((knownMemoryTopics as readonly string[]).includes(normalized)) {
    return normalized as KnownMemoryTopic;
  }
  return topicAliases[normalized];
}

/**
 * The title shown for a topic: a display name that says more than the key,
 * a known topic's title, or the key made readable.
 */
export function memoryTopicTitle(
  key: string,
  displayName: string | undefined,
  knownTitle: (topic: KnownMemoryTopic) => string
): string {
  const name = displayName?.trim() ?? '';
  if (name && name.toLowerCase() !== key.trim().toLowerCase()) return name;
  const known = knownTopicOf(key);
  if (known) return knownTitle(known);
  const readable = key.trim().replace(/[_-]+/g, ' ');
  return readable ? readable.charAt(0).toUpperCase() + readable.slice(1) : key;
}
