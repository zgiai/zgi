import type {
  AgentMemorySlotConfig,
  UpdateAgentRuntimeConfigRequest,
} from '@/services/types/agent';

export type AgentMemorySlotValidationError =
  | 'required'
  | 'pattern'
  | 'duplicate'
  | 'too_many'
  | null;

const AGENT_MEMORY_SLOT_KEY_PATTERN = /^[a-z][a-z0-9_]*$/;

export function toModelParams(
  params: Record<string, unknown> | undefined
): Record<string, number | string | boolean> {
  const next: Record<string, number | string | boolean> = {};
  for (const [key, value] of Object.entries(params ?? {})) {
    if (typeof value === 'number' || typeof value === 'string' || typeof value === 'boolean') {
      next[key] = value;
    }
  }
  return next;
}

export function buildAgentRuntimeSignature(payload: UpdateAgentRuntimeConfigRequest): string {
  const editableMemorySlots = (payload.agent_memory_slots ?? []).map(slot => ({
    key: slot.key,
    description: slot.description,
    max_chars: slot.max_chars,
    enabled: slot.enabled,
    sort_order: slot.sort_order,
  }));

  return JSON.stringify({
    ...payload,
    enabled_skill_ids: [...payload.enabled_skill_ids].sort(),
    knowledge_dataset_ids: [...(payload.knowledge_dataset_ids ?? [])].sort(),
    agent_memory_slots: editableMemorySlots.sort((left, right) =>
      left.key.localeCompare(right.key)
    ),
  });
}

export function validateAgentMemorySlots(
  slots: AgentMemorySlotConfig[]
): AgentMemorySlotValidationError[] {
  const normalizedKeys = slots.map(slot => slot.key.trim().toLowerCase());
  const keyCounts = normalizedKeys.reduce<Record<string, number>>((acc, key) => {
    if (key) acc[key] = (acc[key] ?? 0) + 1;
    return acc;
  }, {});

  return normalizedKeys.map(key => {
    if (slots.length > 5) return 'too_many';
    if (!key) return 'required';
    if (!AGENT_MEMORY_SLOT_KEY_PATTERN.test(key)) return 'pattern';
    if ((keyCounts[key] ?? 0) > 1) return 'duplicate';
    return null;
  });
}

export function pickAgentInitials(name?: string): string {
  const trimmed = name?.trim();
  if (!trimmed) return 'A';
  return trimmed.slice(0, 2).toUpperCase();
}
