import type {
  AgentMemorySlotConfig,
  AgentMemoryTemplate,
  AgentMemoryTemplateBlock,
} from '@/services/types/agent';
import { AGENT_MEMORY_LIMITS, clampAgentMemoryMaxChars } from './utils';

export type AgentMemoryTemplateMode = 'merge' | 'replace';

export type AgentMemoryTemplateResult =
  | { ok: true; slots: AgentMemorySlotConfig[] }
  | { ok: false; reason: 'too_many' };

// Templates come from the backend catalog (ADR 0013). Applying one copies its
// blocks into the draft: replace swaps the whole set, merge keeps existing
// blocks, refreshes those the template also defines, and adds the rest.
export function applyAgentMemoryTemplate(
  currentSlots: AgentMemorySlotConfig[],
  template: AgentMemoryTemplate,
  mode: AgentMemoryTemplateMode
): AgentMemoryTemplateResult {
  if (mode === 'replace') {
    return {
      ok: true,
      slots: template.blocks.map((block, index) => templateBlockToConfig(block, index)),
    };
  }

  const existingKeys = new Set(currentSlots.map(slot => normalizeKey(slot.key)).filter(Boolean));
  const missingBlocks = template.blocks.filter(block => !existingKeys.has(normalizeKey(block.key)));
  if (currentSlots.length + missingBlocks.length > AGENT_MEMORY_LIMITS.maxBlocks) {
    return { ok: false, reason: 'too_many' };
  }

  const templateByKey = new Map(template.blocks.map(block => [normalizeKey(block.key), block]));
  const merged = currentSlots.map(slot => {
    const block = templateByKey.get(normalizeKey(slot.key));
    if (!block) return slot;
    return {
      ...slot,
      name: slot.name?.trim() || block.name,
      description: block.description.slice(0, AGENT_MEMORY_LIMITS.maxDescriptionChars),
      max_chars: clampAgentMemoryMaxChars(block.max_chars),
      read_only: block.read_only,
      auto_write: block.auto_write,
      enabled: true,
    };
  });

  for (const block of missingBlocks) {
    merged.push(templateBlockToConfig(block, merged.length));
  }
  return { ok: true, slots: merged.map((slot, index) => ({ ...slot, sort_order: index })) };
}

export function addAgentMemoryTemplateBlock(
  currentSlots: AgentMemorySlotConfig[],
  block: AgentMemoryTemplateBlock
): AgentMemoryTemplateResult {
  const key = normalizeKey(block.key);
  if (!key || currentSlots.some(item => normalizeKey(item.key) === key)) {
    return { ok: true, slots: currentSlots };
  }
  if (currentSlots.length >= AGENT_MEMORY_LIMITS.maxBlocks) {
    return { ok: false, reason: 'too_many' };
  }
  return {
    ok: true,
    slots: [...currentSlots, templateBlockToConfig(block, currentSlots.length)],
  };
}

function templateBlockToConfig(
  block: AgentMemoryTemplateBlock,
  sortOrder: number
): AgentMemorySlotConfig {
  return {
    key: normalizeKey(block.key),
    name: block.name.slice(0, 80),
    description: block.description.slice(0, AGENT_MEMORY_LIMITS.maxDescriptionChars),
    max_chars: clampAgentMemoryMaxChars(block.max_chars),
    read_only: block.read_only,
    auto_write: block.auto_write,
    enabled: true,
    sort_order: sortOrder,
  };
}

function normalizeKey(key: string): string {
  return key.trim().toLowerCase();
}
